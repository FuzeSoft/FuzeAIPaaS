package workspace

import (
	"encoding/json"
	"fmt"
	"strings"

	"fuze-ai-paas/backend/internal/k8s/chip"
	"fuze-ai-paas/backend/internal/models"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const Namespace = "fuze-ai-paas"

const (
	workspaceNamePrefix = "ws-"
)

func vendorOfModel(gpuModel string) string {
	m := strings.ToLower(gpuModel)
	switch {
	case strings.Contains(m, "ascend"):
		return "Ascend"
	case strings.Contains(m, "cambricon") || strings.Contains(m, "mlu"):
		return "Cambricon"
	default:
		return "NVIDIA"
	}
}

func sanitizeName(name string) string {
	var sb strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			sb.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			sb.WriteRune(r + 32)
		case r == '-' || r == '.':
			sb.WriteRune(r)
		default:
			sb.WriteRune('-')
		}
	}
	out := strings.Trim(sb.String(), "-")
	if out == "" {
		return "ws"
	}
	return out
}

func objectName(ws *models.Workspace) string {
	return workspaceNamePrefix + sanitizeName(ws.ID)
}

func BuildWorkspaceManifest(ws *models.Workspace) *unstructured.Unstructured {
	name := objectName(ws)

	cpu := orDefault(ws.CPURequest, "1")
	mem := orDefault(ws.MemoryRequest, "2Gi")

	limits := map[string]interface{}{
		"cpu":    cpu,
		"memory": mem,
	}
	requests := map[string]interface{}{
		"cpu":    cpu,
		"memory": mem,
	}
	
	if ws.GPUCount > 0 {
		spec := chip.Spec{
			Vendor: chip.VendorOf(vendorOfModel(ws.GPUModel)),
			GPUs:   ws.GPUCount,
		}
		for k, v := range spec.ResourceLimits() {
			limits[k] = v
			requests[k] = v
		}
	}

	env := []interface{}{}
	if ws.IdleTimeout > 0 {
		env = append(env, map[string]interface{}{
			"name":  "FUZE_IDLE_TIMEOUT_SECONDS",
			"value": fmt.Sprintf("%d", int64(ws.IdleTimeout.Seconds())),
		})
	}
	
	baseURL := fmt.Sprintf("/api/v1/workspaces/%s/proxy", ws.ID)
	env = append(env,
		map[string]interface{}{"name": "JUPYTER_BASE_URL", "value": baseURL},
		map[string]interface{}{"name": "NB_PREFIX", "value": baseURL},
		map[string]interface{}{"name": "CODE_SERVER_APP_BASE_PATH", "value": baseURL},
	)

	container := map[string]interface{}{
		"name":            "notebook",
		"image":           ws.Image,
		"resources":       map[string]interface{}{"limits": limits, "requests": requests},
		"securityContext": securityBaseline(),
		
		"ports": []interface{}{
			map[string]interface{}{
				"containerPort": int64(notebookPort),
				"protocol":      "TCP",
			},
		},
		"volumeMounts": []interface{}{
			map[string]interface{}{
				"name":      "home",
				"mountPath": "/home/jovyan",
			},
		},
		"env": env,
	}

	podSpec := map[string]interface{}{
		"restartPolicy": "Always",
		"containers":    []interface{}{container},
		"volumes": []interface{}{
			map[string]interface{}{
				"name": "home",
				"persistentVolumeClaim": map[string]interface{}{
					"claimName": name + "-home",
					"readOnly":  false,
				},
			},
		},
		
		"automountServiceAccountToken": false,
	}

	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": Namespace,
				"labels": map[string]interface{}{
					"app":                    "fuze-ai-paas",
					"managed-by":             "fuze-scheduler",
					"fuze.ai/workspace-id":   ws.ID,
					"fuze.ai/tenant-id":      ws.TenantID,
					"fuze.ai/owner-id":       ws.OwnerID,
					"fuze.ai/workspace-kind": string(ws.Kind),
				},
			},
			"spec": map[string]interface{}{
				"replicas": int64(1),
				"selector": map[string]interface{}{
					"matchLabels": map[string]interface{}{
						"fuze.ai/workspace-id": ws.ID,
					},
				},
				"template": map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							"app":                    "fuze-ai-paas",
							"fuze.ai/workspace-id":   ws.ID,
							"fuze.ai/tenant-id":      ws.TenantID,
							"fuze.ai/workspace-kind": string(ws.Kind),
						},
					},
					"spec": podSpec,
				},
			},
		},
	}
}

func securityBaseline() map[string]interface{} {
	return map[string]interface{}{
		"runAsNonRoot":             true,
		"runAsUser":                int64(1000),
		"privileged":               false,
		"allowPrivilegeEscalation": false,
		"readOnlyRootFilesystem":   true,
		"capabilities": map[string]interface{}{
			"drop": []interface{}{"ALL"},
		},
	}
}

func Snapshot(obj *unstructured.Unstructured) error {
	b, err := json.Marshal(obj.Object)
	if err != nil {
		return fmt.Errorf("snapshot: manifest not serializable: %w", err)
	}
	var root map[string]interface{}
	if err := json.Unmarshal(b, &root); err != nil {
		return fmt.Errorf("snapshot: manifest not valid JSON: %w", err)
	}
	spec := asNested(root, "spec", "template", "spec")
	if spec == nil {
		return fmt.Errorf("snapshot: missing pod spec")
	}
	containers, ok := spec["containers"].([]interface{})
	if !ok || len(containers) == 0 {
		return fmt.Errorf("snapshot: no containers")
	}
	first, ok := containers[0].(map[string]interface{})
	if !ok {
		return fmt.Errorf("snapshot: container is not an object")
	}
	scRaw, ok := first["securityContext"]
	if !ok {
		return fmt.Errorf("snapshot: container missing securityContext")
	}
	sc, ok := scRaw.(map[string]interface{})
	if !ok {
		return fmt.Errorf("snapshot: securityContext is not an object")
	}
	if v, _ := sc["runAsNonRoot"].(bool); !v {
		return fmt.Errorf("snapshot: runAsNonRoot must be true")
	}
	if v, _ := sc["privileged"].(bool); v {
		return fmt.Errorf("snapshot: privileged must be false")
	}
	if v, _ := sc["readOnlyRootFilesystem"].(bool); !v {
		return fmt.Errorf("snapshot: readOnlyRootFilesystem must be true")
	}
	if v, _ := sc["allowPrivilegeEscalation"].(bool); v {
		return fmt.Errorf("snapshot: allowPrivilegeEscalation must be false")
	}
	caps, ok := sc["capabilities"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("snapshot: capabilities missing")
	}
	drop, ok := caps["drop"].([]interface{})
	if !ok || len(drop) != 1 || drop[0] != "ALL" {
		return fmt.Errorf("snapshot: capabilities.drop must be [ALL]")
	}
	return nil
}

func asNested(root map[string]interface{}, path ...string) map[string]interface{} {
	cur := root
	for _, p := range path {
		next, ok := cur[p].(map[string]interface{})
		if !ok {
			return nil
		}
		cur = next
	}
	return cur
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}