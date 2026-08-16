
package runtime

import (
	"fmt"

	"fuze-ai-paas/backend/internal/domain/inference"
	"fuze-ai-paas/backend/internal/k8s"
	"fuze-ai-paas/backend/internal/ports"
)

type Registry struct{}

func NewDefaultRegistry() *Registry { return &Registry{} }

func (r *Registry) For(_ string, kind inference.RuntimeKind, client ports.ClusterClientPort) (inference.RuntimeClient, error) {
	k8sClient, ok := client.(*k8s.Client)
	if !ok {
		return nil, fmt.Errorf("runtime registry requires a k8s cluster client, got %T", client)
	}
	switch kind {
	case inference.RuntimeVLLM:
		return &VLLMRuntime{deploymentRuntime{client: k8sClient, port: 8000, defaultImage: "vllm/vllm-openai:latest"}}, nil
	case inference.RuntimeTriton:
		return &TritonRuntime{deploymentRuntime{client: k8sClient, port: 8000, defaultImage: "nvcr.io/nvidia/tritonserver:23.10-py3"}}, nil
	case inference.RuntimeAscend:
		return &AscendRuntime{deploymentRuntime{client: k8sClient, port: 8000, defaultImage: "ascend-mindie:latest"}}, nil
	case inference.RuntimeKServe, inference.RuntimeCustom:
		return &KServeRuntime{client: k8sClient}, nil
	default:
		return nil, fmt.Errorf("unsupported inference runtime: %s", kind)
	}
}