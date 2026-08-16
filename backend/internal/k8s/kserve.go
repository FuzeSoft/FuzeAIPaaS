package k8s

import (
	"context"
	"fmt"
	"log"

	"fuze-ai-paas/backend/internal/k8s/chip"
	"fuze-ai-paas/backend/internal/models"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
)

const (
	
	kserveGroup    = "serving.kserve.io"
	kserveVersion  = "v1beta1"
	kserveResource = "inferenceservices"
)

func KServeGVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{
		Group:    kserveGroup,
		Version:  kserveVersion,
		Resource: kserveResource,
	}
}

func (c *Client) BuildInferenceServiceObject(svc *models.InferenceService, vendor string) *unstructured.Unstructured {
	name := svc.Name
	if name == "" {
		name = sanitizeName("isvc-" + svc.ID)
	}

	predictorResources := map[string]interface{}{
		"limits": map[string]interface{}{
			"cpu":    orDefault(svc.CPU, "1"),
			"memory": orDefault(svc.Memory, "2Gi"),
		},
		"requests": map[string]interface{}{
			"cpu":    orDefault(svc.CPU, "1"),
			"memory": orDefault(svc.Memory, "2Gi"),
		},
	}
	
	spec := chip.Spec{
		Vendor:    chip.VendorOf(vendor),
		GPUs:      svc.GPUs,
		GPUMemory: svc.GPUMemory,
		GPUCores:  svc.GPUCores,
	}
	
	limits, ok := predictorResources["limits"].(map[string]interface{})
	if !ok {
		return nil
	}
	requests, ok := predictorResources["requests"].(map[string]interface{})
	if !ok {
		return nil
	}
	for k, v := range spec.ResourceLimits() {
		limits[k] = v
		requests[k] = v
	}

	annotations := chip.Annotations(chip.Spec{
		Vendor:    chip.VendorOf(vendor),
		GPUs:      svc.GPUs,
		GPUMemory: svc.GPUMemory,
		GPUCores:  svc.GPUCores,
	})
	predictorAnnotations := map[string]interface{}{}
	for k, v := range annotations {
		predictorAnnotations[k] = v
	}

	predictor := map[string]interface{}{
		"minReplicas": int64(svc.MinReplicas),
		"maxReplicas": int64(svc.MaxReplicas),
	}
	
	if len(predictorAnnotations) > 0 {
		predictor["metadata"] = map[string]interface{}{
			"annotations": predictorAnnotations,
		}
	}

	switch svc.Framework {
	case models.FrameworkCustom:
		predictor["containers"] = []interface{}{
			map[string]interface{}{
				"image":     svc.Image,
				"resources": predictorResources,
			},
		}
	default:
		predictor["model"] = map[string]interface{}{
			"modelFormat": map[string]interface{}{
				"name": string(svc.Framework),
			},
			"storageUri": svc.StorageURI,
			"resources":  predictorResources,
		}
	}

	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": fmt.Sprintf("%s/%s", kserveGroup, kserveVersion),
			"kind":       "InferenceService",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": c.namespace,
				"labels": map[string]interface{}{
					"app":        "fuze-ai-paas",
					"managed-by": "fuze-scheduler",
				},
			},
			
			"spec": map[string]interface{}{
				"predictor": predictor,
			},
		},
	}
}

func (c *Client) CreateInferenceService(ctx context.Context, svc *models.InferenceService) (string, error) {
	if !c.enabled {
		return "", fmt.Errorf("k8s client not available")
	}

	obj := c.BuildInferenceServiceObject(svc, svc.Chip)
	result, err := c.dynamicClient.Resource(KServeGVR()).Namespace(c.namespace).Create(ctx, obj, metav1.CreateOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to create kserve inferenceservice: %w", err)
	}

	log.Printf("[K8s] KServe InferenceService created: %s", result.GetName())
	return result.GetName(), nil
}

func (c *Client) DeleteInferenceService(ctx context.Context, name string) error {
	if !c.enabled {
		return fmt.Errorf("k8s client not available")
	}

	propagation := metav1.DeletePropagationForeground
	if err := c.dynamicClient.Resource(KServeGVR()).Namespace(c.namespace).Delete(
		ctx, name, metav1.DeleteOptions{PropagationPolicy: &propagation},
	); err != nil {
		return fmt.Errorf("failed to delete kserve inferenceservice: %w", err)
	}
	log.Printf("[K8s] KServe InferenceService deleted: %s", name)
	return nil
}

func (c *Client) GetInferenceServiceStatus(ctx context.Context, name string) (models.InferenceStatus, int, string, error) {
	if !c.enabled {
		return models.InferenceStatusUnknown, 0, "", fmt.Errorf("k8s client not available")
	}

	obj, err := c.dynamicClient.Resource(KServeGVR()).Namespace(c.namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return models.InferenceStatusUnknown, 0, "", fmt.Errorf("failed to get kserve inferenceservice: %w", err)
	}

	ready, found, failed := getConditionReady(obj)
	url, _, _ := unstructured.NestedString(obj.Object, "status", "url")
	replicas, _, _ := unstructured.NestedInt64(obj.Object, "status", "components", "predictor", "latestReadyRevisionReplicas")

	return models.KServeStateToStatus(ready, found, failed), int(replicas), url, nil
}

const predictorPath = "/spec/predictor"

func predictorFieldPatch(field string, value int) (types.PatchType, []byte) {
	
	return types.JSONPatchType, []byte(fmt.Sprintf(
		`[{"op":"add","path":"%s/%s","value":%d}]`, predictorPath, field, value))
}

func replicasPatch(replicas int) (types.PatchType, []byte, error) {
	if replicas < 0 {
		return "", nil, fmt.Errorf("replicas must be non-negative, got %d", replicas)
	}
	patchType, payload := predictorFieldPatch("minReplicas", replicas)
	return patchType, payload, nil
}

func canaryPatch(weight int) (types.PatchType, []byte, error) {
	if weight < 0 || weight > 100 {
		return "", nil, fmt.Errorf("canary weight must be between 0 and 100, got %d", weight)
	}
	patchType, payload := predictorFieldPatch("canaryTrafficPercent", weight)
	return patchType, payload, nil
}

func (c *Client) PatchInferenceServiceReplicas(ctx context.Context, name string, replicas int) error {
	if !c.enabled {
		return fmt.Errorf("k8s client not available")
	}
	patchType, patch, err := replicasPatch(replicas)
	if err != nil {
		return err
	}
	if _, err := c.dynamicClient.Resource(KServeGVR()).Namespace(c.namespace).Patch(
		ctx, name, patchType, patch, metav1.PatchOptions{}); err != nil {
		return fmt.Errorf("failed to patch kserve replicas: %w", err)
	}
	return nil
}

func (c *Client) PatchInferenceServiceCanary(ctx context.Context, name string, weight int) error {
	if !c.enabled {
		return fmt.Errorf("k8s client not available")
	}
	patchType, patch, err := canaryPatch(weight)
	if err != nil {
		return err
	}
	if _, err := c.dynamicClient.Resource(KServeGVR()).Namespace(c.namespace).Patch(
		ctx, name, patchType, patch, metav1.PatchOptions{}); err != nil {
		return fmt.Errorf("failed to patch kserve canary traffic: %w", err)
	}
	return nil
}

func getConditionReady(obj *unstructured.Unstructured) (bool, bool, bool) {
	conditions, found, _ := unstructured.NestedSlice(obj.Object, "status", "conditions")
	if !found {
		return false, false, false
	}
	for _, cond := range conditions {
		condMap, ok := cond.(map[string]interface{})
		if !ok {
			continue
		}
		condType, _ := condMap["type"].(string)
		if condType == "Ready" {
			status, _ := condMap["status"].(string)
			return status == "True", true, status == "False"
		}
	}
	return false, true, false
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}