package runtime

import (
	"context"
	"fmt"
	"k8s.io/apimachinery/pkg/types"

	"fuze-ai-paas/backend/internal/domain/inference"
	"fuze-ai-paas/backend/internal/k8s"
	"fuze-ai-paas/backend/internal/k8s/chip"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func deploymentGVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
}

func serviceGVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{Group: "", Version: "v1", Resource: "services"}
}

type deploymentRuntime struct {
	client       *k8s.Client
	port         int
	defaultImage string
}

func (d deploymentRuntime) namespace() string {
	if d.client == nil {
		return k8s.DefaultNamespace
	}
	return d.client.Namespace()
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

func (d deploymentRuntime) Deploy(ctx context.Context, svc *inference.InferenceService) (string, error) {
	if d.client == nil || !d.client.Enabled() {
		return "", fmt.Errorf("k8s client not available")
	}
	name := svc.RuntimeName
	if name == "" {
		name = k8s.SanitizeName(svc.Name)
	}
	ns := d.namespace()

	dep := d.buildDeploymentObject(svc, name, ns)
	if _, err := d.client.Dynamic().Resource(deploymentGVR()).Namespace(ns).Create(ctx, dep, metav1.CreateOptions{}); err != nil {
		return "", fmt.Errorf("failed to create deployment: %w", err)
	}

	svcObj := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Service",
		"metadata":   map[string]interface{}{"name": name, "namespace": ns, "labels": map[string]interface{}{"app": name, "managed-by": "fuze-scheduler"}},
		"spec": map[string]interface{}{
			"selector": map[string]interface{}{"app": name},
			"ports":    []interface{}{map[string]interface{}{"port": int64(d.port), "targetPort": int64(d.port), "protocol": "TCP"}},
		},
	}}
	if _, err := d.client.Dynamic().Resource(serviceGVR()).Namespace(ns).Create(ctx, svcObj, metav1.CreateOptions{}); err != nil {
		return "", fmt.Errorf("failed to create service: %w", err)
	}
	return name, nil
}

func (d deploymentRuntime) buildDeploymentObject(svc *inference.InferenceService, name, ns string) *unstructured.Unstructured {
	
	annotations := chip.Annotations(chip.Spec{
		Vendor:    chip.VendorOf(svc.Chip),
		GPUs:      svc.GPUs,
		GPUMemory: svc.GPUMemory,
		GPUCores:  svc.GPUCores,
	})
	podAnn := map[string]interface{}{}
	for k, v := range annotations {
		podAnn[k] = v
	}

	container := map[string]interface{}{
		"name":  "predictor",
		"image": orDefaultImage(svc.Image, d.defaultImage),
		"ports": []interface{}{map[string]interface{}{"containerPort": int64(d.port)}},
		"resources": map[string]interface{}{
			"limits": map[string]interface{}{
				"cpu":    orDefault(svc.CPU, "1"),
				"memory": orDefault(svc.Memory, "2Gi"),
			},
		},
	}
	labels := map[string]interface{}{"app": name, "managed-by": "fuze-scheduler"}

	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata":   map[string]interface{}{"name": name, "namespace": ns, "labels": labels},
		"spec": map[string]interface{}{
			"replicas": int64(svc.MinReplicas),
			"selector": map[string]interface{}{"matchLabels": labels},
			"template": map[string]interface{}{
				"metadata": map[string]interface{}{"labels": labels, "annotations": podAnn},
				"spec":     map[string]interface{}{"containers": []interface{}{container}},
			},
		},
	}}
}

func (d deploymentRuntime) Undeploy(ctx context.Context, runtimeName string) error {
	if d.client == nil || !d.client.Enabled() {
		return fmt.Errorf("k8s client not available")
	}
	ns := d.namespace()
	_ = d.client.Dynamic().Resource(deploymentGVR()).Namespace(ns).Delete(ctx, runtimeName, metav1.DeleteOptions{})
	_ = d.client.Dynamic().Resource(serviceGVR()).Namespace(ns).Delete(ctx, runtimeName, metav1.DeleteOptions{})
	return nil
}

func (d deploymentRuntime) Status(ctx context.Context, runtimeName string) (bool, bool, bool, int, string, error) {
	if d.client == nil || !d.client.Enabled() {
		return false, false, false, 0, "", fmt.Errorf("k8s client not available")
	}
	ns := d.namespace()
	obj, err := d.client.Dynamic().Resource(deploymentGVR()).Namespace(ns).Get(ctx, runtimeName, metav1.GetOptions{})
	if err != nil {
		return false, false, false, 0, "", err
	}
	readyReplicas, _, _ := unstructured.NestedInt64(obj.Object, "status", "readyReplicas")
	replicas, _, _ := unstructured.NestedInt64(obj.Object, "status", "replicas")
	url := fmt.Sprintf("http://%s.%s.svc.cluster.local:%d", runtimeName, ns, d.port)

	failed := deploymentFailed(obj)
	
	if replicas == 0 {
		return true, true, false, 0, url, nil
	}
	ready := int(readyReplicas) > 0 && int(readyReplicas) == int(replicas)
	return ready, true, failed, int(readyReplicas), url, nil
}

func deploymentFailed(obj *unstructured.Unstructured) bool {
	conds, found, _ := unstructured.NestedSlice(obj.Object, "status", "conditions")
	if !found {
		return false
	}
	for _, c := range conds {
		cm, ok := c.(map[string]interface{})
		if !ok {
			continue
		}
		if t, _ := cm["type"].(string); t == "Progressing" {
			if s, _ := cm["status"].(string); s == "False" {
				return true
			}
		}
	}
	return false
}

func scalePatch(replicas int) (types.PatchType, []byte) {
	return types.JSONPatchType, []byte(fmt.Sprintf(
		`[{"op":"replace","path":"/spec/replicas","value":%d}]`, replicas))
}

func (d deploymentRuntime) Scale(ctx context.Context, runtimeName string, replicas int) error {
	if d.client == nil || !d.client.Enabled() {
		return fmt.Errorf("k8s client not available")
	}
	ns := d.namespace()
	patchType, patch := scalePatch(replicas)
	_, err := d.client.Dynamic().Resource(deploymentGVR()).Namespace(ns).Patch(ctx, runtimeName, patchType, patch, metav1.PatchOptions{})
	return err
}

func (d deploymentRuntime) RolloutCanary(ctx context.Context, runtimeName string, weight int) error {
	if weight <= 0 || weight >= 100 {
		
		return nil
	}
	return fmt.Errorf("%w: deployment runtime requires a traffic gateway (istio/ingress)", inference.ErrCanaryUnsupported)
}

type VLLMRuntime struct{ deploymentRuntime }

type TritonRuntime struct{ deploymentRuntime }

type AscendRuntime struct{ deploymentRuntime }

func orDefaultImage(img, def string) string {
	if img == "" {
		return def
	}
	return img
}