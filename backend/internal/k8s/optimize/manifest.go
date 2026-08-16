
package optimize

import (
	"encoding/json"
	"fmt"
	"strings"

	"fuze-ai-paas/backend/internal/domain/optimize"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const Namespace = "fuze-ai-paas"

const jobNamePrefix = "opt-"

var defaultBackendImages = map[optimize.CompressionBackend]string{
	optimize.BackendPyTorch:     "registry.fuze.ai/optimize/pytorch:latest",
	optimize.BackendONNXRuntime: "registry.fuze.ai/optimize/onnxruntime:latest",
	optimize.BackendOpenVINO:    "registry.fuze.ai/optimize/openvino:latest",
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
		return "opt"
	}
	return out
}

func objectName(taskID string) string {
	return jobNamePrefix + sanitizeName(taskID)
}

func GetBackendImage(b optimize.CompressionBackend, images map[optimize.CompressionBackend]string) (string, error) {
	if img, ok := images[b]; ok && img != "" {
		return img, nil
	}
	if img, ok := defaultBackendImages[b]; ok {
		return img, nil
	}
	return "", fmt.Errorf("no image whitelisted for backend %q", b)
}

func BuildCompressionJob(task *optimize.CompressionTask, images map[optimize.CompressionBackend]string) (*unstructured.Unstructured, error) {
	name := objectName(task.ID)
	img, err := GetBackendImage(task.Backend, images)
	if err != nil {
		return nil, err
	}

	env := []interface{}{
		map[string]interface{}{"name": "OPT_TASK_TYPE", "value": string(task.Type)},
		map[string]interface{}{"name": "OPT_BACKEND", "value": string(task.Backend)},
		map[string]interface{}{"name": "OPT_CONFIG", "value": task.ConfigJSON},
		map[string]interface{}{"name": "OPT_MODEL_VERSION_ID", "value": task.ModelVersionID},
	}

	container := map[string]interface{}{
		"name":            "compressor",
		"image":           img,
		"securityContext": securityBaseline(),
		"env":             env,
		"volumeMounts": []interface{}{
			map[string]interface{}{"name": "work", "mountPath": "/work"},
		},
		
	}

	podSpec := map[string]interface{}{
		"restartPolicy":                 "Never",
		"automountServiceAccountToken": false,
		"containers":                    []interface{}{container},
		"volumes": []interface{}{
			map[string]interface{}{
				"name": "work",
				"persistentVolumeClaim": map[string]interface{}{
					"claimName": name + "-work",
					"readOnly":  false,
				},
			},
		},
	}

	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "batch/v1",
			"kind":       "Job",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": Namespace,
				"labels": map[string]interface{}{
					"app":                  "fuze-ai-paas",
					"managed-by":           "fuze-optimizer",
					"fuze.ai/task-id":      task.ID,
					"fuze.ai/tenant-id":    task.TenantID,
					"fuze.ai/opt-type":     string(task.Type),
					"fuze.ai/opt-backend":  string(task.Backend),
				},
			},
			"spec": map[string]interface{}{
				"backoffLimit": int64(1),
				"ttlSecondsAfterFinished": int64(3600),
				"template": map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							"app":               "fuze-ai-paas",
							"fuze.ai/task-id":   task.ID,
							"fuze.ai/tenant-id": task.TenantID,
						},
					},
					"spec": podSpec,
				},
			},
		},
	}, nil
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

func Snapshot(obj *unstructured.Unstructured, task *optimize.CompressionTask, images map[optimize.CompressionBackend]string) error {
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
	
	sc, ok := first["securityContext"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("snapshot: container missing securityContext")
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
	
	img, _ := first["image"].(string)
	if _, err := GetBackendImage(task.Backend, images); err != nil {
		return fmt.Errorf("snapshot: %w", err)
	}
	if !imageInWhitelist(img, task.Backend, images) {
		return fmt.Errorf("snapshot: image %q not in whitelist for backend %q", img, task.Backend)
	}
	
	vols, _ := spec["volumes"].([]interface{})
	for _, v := range vols {
		vm, ok := v.(map[string]interface{})
		if !ok {
			continue
		}
		if _, has := vm["hostPath"]; has {
			return fmt.Errorf("snapshot: hostPath volume is forbidden")
		}
	}
	return nil
}

func imageInWhitelist(img string, b optimize.CompressionBackend, images map[optimize.CompressionBackend]string) bool {
	want, _ := GetBackendImage(b, images)
	return img == want
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