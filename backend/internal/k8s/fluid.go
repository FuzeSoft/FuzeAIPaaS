package k8s

import (
	"context"
	"fmt"
	"log"
	"strings"

	"fuze-ai-paas/backend/internal/models"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	
	fluidGroup   = "data.fluid.io"
	fluidVersion = "v1alpha1"

	fluidDatasetResource = "datasets"
)

func FluidDatasetGVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{
		Group:    fluidGroup,
		Version:  fluidVersion,
		Resource: fluidDatasetResource,
	}
}

func FluidRuntimeGVR(runtime models.RuntimeType) schema.GroupVersionResource {
	resource := "alluxioruntimes"
	switch runtime {
	case models.RuntimeJuiceFS:
		resource = "juicefsruntimes"
	case models.RuntimeGooseFS:
		resource = "goosefsruntimes"
	case models.RuntimeVineyard:
		resource = "vineyardruntimes"
	case models.RuntimeAlluxio:
		resource = "alluxioruntimes"
	}
	return schema.GroupVersionResource{
		Group:    fluidGroup,
		Version:  fluidVersion,
		Resource: resource,
	}
}

func runtimeKind(runtime models.RuntimeType) string {
	switch runtime {
	case models.RuntimeJuiceFS:
		return "JuiceFSRuntime"
	case models.RuntimeGooseFS:
		return "GooseFSRuntime"
	case models.RuntimeVineyard:
		return "VineyardRuntime"
	default:
		return "AlluxioRuntime"
	}
}

func (c *Client) CreateDataset(ctx context.Context, ds *models.Dataset) error {
	if !c.enabled {
		return fmt.Errorf("k8s client not available")
	}

	name := sanitizeName(ds.Name)
	accessMode := "ReadOnly"
	if ds.AccessMode != "" {
		accessMode = ds.AccessMode
	}

	datasetObj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": fmt.Sprintf("%s/%s", fluidGroup, fluidVersion),
			"kind":       "Dataset",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": c.namespace,
				"labels": map[string]interface{}{
					"app":        "fuze-ai-paas",
					"managed-by": "fuze-scheduler",
				},
			},
			"spec": map[string]interface{}{
				
				"mounts": []interface{}{
					map[string]interface{}{
						"mountPoint": ds.MountPoint,
						"name":       name,
						"path":       orDefault(ds.SubPath, "/"),
					},
				},
				"accessModes": []interface{}{accessModeK8s(accessMode)},
			},
		},
	}

	datasetCreated := false
	if _, err := c.dynamicClient.Resource(FluidDatasetGVR()).Namespace(c.namespace).
		Create(ctx, datasetObj, metav1.CreateOptions{}); err != nil {
		if !apierrors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create fluid dataset: %w", err)
		}
		log.Printf("[K8s] Fluid Dataset already exists, reusing: %s", name)
	} else {
		datasetCreated = true
		log.Printf("[K8s] Fluid Dataset created: %s", name)
	}

	replicas := ds.Replicas
	if replicas <= 0 {
		replicas = 1
	}
	medium := string(ds.CacheMedium)
	if medium == "" {
		medium = "MEM"
	}
	quota := ds.CacheCapacity
	if quota == "" {
		quota = "100Gi"
	}

	runtimeObj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": fmt.Sprintf("%s/%s", fluidGroup, fluidVersion),
			"kind":       runtimeKind(ds.Runtime),
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": c.namespace,
				"labels": map[string]interface{}{
					"app":        "fuze-ai-paas",
					"managed-by": "fuze-scheduler",
				},
			},
			"spec": map[string]interface{}{
				"replicas": int64(replicas),
				"tieredstore": map[string]interface{}{
					"levels": []interface{}{
						map[string]interface{}{
							"mediumtype": medium,
							"path":       orDefault(ds.CachePath, "/dev/shm"),
							"quota":      quota,
							"high":       "0.95",
							"low":        "0.7",
						},
					},
				},
			},
		},
	}

	if _, err := c.dynamicClient.Resource(FluidRuntimeGVR(ds.Runtime)).Namespace(c.namespace).
		Create(ctx, runtimeObj, metav1.CreateOptions{}); err != nil {
		if apierrors.IsAlreadyExists(err) {
			log.Printf("[K8s] Fluid %s already exists, reusing: %s", runtimeKind(ds.Runtime), name)
			return nil
		}
		
		if datasetCreated {
			
			if delErr := c.dynamicClient.Resource(FluidDatasetGVR()).Namespace(c.namespace).
				Delete(ctx, name, metav1.DeleteOptions{}); delErr != nil && !apierrors.IsNotFound(delErr) {
				log.Printf("[K8s] CRITICAL: orphaned fluid dataset %s could not be rolled back: %v", name, delErr)
				return fmt.Errorf("failed to create fluid runtime: %w (rollback of dataset %s also failed: %v)",
					err, name, delErr)
			}
		}
		return fmt.Errorf("failed to create fluid runtime: %w", err)
	}
	log.Printf("[K8s] Fluid %s created: %s", runtimeKind(ds.Runtime), name)

	return nil
}

func (c *Client) DeleteDataset(ctx context.Context, ds *models.Dataset) error {
	if !c.enabled {
		return fmt.Errorf("k8s client not available")
	}
	name := sanitizeName(ds.Name)

	_ = c.dynamicClient.Resource(FluidRuntimeGVR(ds.Runtime)).Namespace(c.namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err := c.dynamicClient.Resource(FluidDatasetGVR()).Namespace(c.namespace).Delete(ctx, name, metav1.DeleteOptions{}); err != nil {
		return fmt.Errorf("failed to delete fluid dataset: %w", err)
	}
	log.Printf("[K8s] Fluid Dataset deleted: %s", name)
	return nil
}

func (c *Client) GetDatasetStatus(ctx context.Context, name string) (models.DatasetStatus, float64, error) {
	if !c.enabled {
		return models.DatasetStatusUnknown, 0, fmt.Errorf("k8s client not available")
	}
	name = sanitizeName(name)

	obj, err := c.dynamicClient.Resource(FluidDatasetGVR()).Namespace(c.namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return models.DatasetStatusUnknown, 0, fmt.Errorf("failed to get fluid dataset: %w", err)
	}

	phase, _, _ := unstructured.NestedString(obj.Object, "status", "phase")
	cachedPercent := 0.0
	if pctStr, found, _ := unstructured.NestedString(obj.Object, "status", "cacheStates", "cachedPercentage"); found {
		cachedPercent = parsePercent(pctStr)
	}

	return models.FluidPhaseToStatus(phase), cachedPercent, nil
}

func accessModeK8s(mode string) string {
	if strings.EqualFold(mode, "ReadWrite") {
		return "ReadWriteMany"
	}
	return "ReadOnlyMany"
}

func parsePercent(s string) float64 {
	s = strings.TrimSuffix(strings.TrimSpace(s), "%")
	var v float64
	_, _ = fmt.Sscanf(s, "%f", &v)
	return v
}