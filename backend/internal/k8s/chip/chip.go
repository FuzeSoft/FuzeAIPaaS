
package chip

import (
	"fmt"

	"fuze-ai-paas/backend/internal/domain/gpu"
)

type Spec struct {
	Vendor    gpu.Vendor
	GPUs      int 
	GPUMemory int 
	GPUCores  int 
}

func VendorOf(s string) gpu.Vendor {
	switch gpu.Vendor(s) {
	case gpu.VendorAscend:
		return gpu.VendorAscend
	case gpu.VendorCambricon:
		return gpu.VendorCambricon
	case gpu.VendorNVIDIA:
		return gpu.VendorNVIDIA
	default:
		return gpu.VendorNVIDIA 
	}
}

func Annotations(spec Spec) map[string]string {
	ann := map[string]string{}
	switch spec.Vendor {
	case gpu.VendorAscend:
		if spec.GPUs > 0 {
			ann["ascend.com/vnpu"] = fmt.Sprintf("%d", spec.GPUs)
		}
		if spec.GPUMemory > 0 {
			ann["ascend.com/virMemory"] = fmt.Sprintf("%d", spec.GPUMemory)
		}
		if spec.GPUCores > 0 {
			ann["ascend.com/virAICore"] = fmt.Sprintf("%d", spec.GPUCores)
		}
	case gpu.VendorCambricon:
		if spec.GPUs > 0 {
			ann["cambricon.com/mlu"] = fmt.Sprintf("%d", spec.GPUs)
		}
		if spec.GPUMemory > 0 {
			ann["cambricon.com/virMemory"] = fmt.Sprintf("%d", spec.GPUMemory)
		}
		if spec.GPUCores > 0 {
			ann["cambricon.com/virAICore"] = fmt.Sprintf("%d", spec.GPUCores)
		}
	default: 
		if spec.GPUs > 0 {
			ann["nvidia.com/gpu"] = fmt.Sprintf("%d", spec.GPUs)
		}
		if spec.GPUMemory > 0 {
			ann["nvidia.com/gpumem"] = fmt.Sprintf("%d", spec.GPUMemory)
		}
		if spec.GPUCores > 0 {
			ann["nvidia.com/gpucores"] = fmt.Sprintf("%d", spec.GPUCores)
		}
	}
	return ann
}

func (s Spec) ResourceLimits() map[string]string {
	limits := map[string]string{}
	if s.GPUs <= 0 {
		return limits
	}
	switch s.Vendor {
	case gpu.VendorAscend:
		limits["ascend.com/vnpu"] = fmt.Sprintf("%d", s.GPUs)
	case gpu.VendorCambricon:
		limits["cambricon.com/mlu"] = fmt.Sprintf("%d", s.GPUs)
	default: 
		limits["nvidia.com/gpu"] = fmt.Sprintf("%d", s.GPUs)
	}
	return limits
}

func Recognize(dev gpu.GPUDevice) (isNPU bool, perCardMemory int) {
	switch dev.VendorEnum() {
	case gpu.VendorAscend, gpu.VendorCambricon:
		isNPU = true
	}
	perCardMemory = 80 
	switch {
	case contains(dev.Model, "40GB"):
		perCardMemory = 40
	case contains(dev.Model, "80GB"):
		perCardMemory = 80
	case contains(dev.Model, "A800"):
		perCardMemory = 80
	case contains(dev.Model, "H800"):
		perCardMemory = 80
	case contains(dev.Model, "V100"):
		perCardMemory = 32
	case contains(dev.Model, "Ascend 910"):
		perCardMemory = 32
	case contains(dev.Model, "T4"):
		perCardMemory = 16
	}
	return isNPU, perCardMemory
}

func NodeDeviceResourceKeys() []string {
	return []string{
		"nvidia.com/gpu",
		"ascend.com/npu",
		"hygon.com/dcunum",
		"cambricon.com/mlu",
	}
}

func NodeVendorLabelKeys() []NodeVendorLabel {
	return []NodeVendorLabel{
		{Key: "nvidia.com/gpu.product", Vendor: gpu.VendorNVIDIA},
		{Key: "nvidia.com/gpu", Vendor: gpu.VendorNVIDIA},
		{Key: "ascend.com/chip-name", Vendor: gpu.VendorAscend},
		{Key: "hygon.com/dcuname", Vendor: gpu.VendorUnknown}, 
		{Key: "cambricon.com/mlu", Vendor: gpu.VendorCambricon},
	}
}

type NodeVendorLabel struct {
	Key    string
	Vendor gpu.Vendor
}

func VendorFromNodeLabels(labels map[string]string) (gpu.Vendor, string, bool) {
	for _, l := range NodeVendorLabelKeys() {
		if v, ok := labels[l.Key]; ok {
			return l.Vendor, v, true
		}
	}
	return gpu.VendorUnknown, "", false
}

func (s Spec) JobResourceLimits() map[string]string {
	limits := map[string]string{}
	if s.GPUs <= 0 {
		return limits
	}
	switch s.Vendor {
	case gpu.VendorAscend:
		limits["ascend.com/vnpu"] = fmt.Sprintf("%d", s.GPUs)
		if s.GPUMemory > 0 {
			limits["ascend.com/virMemory"] = fmt.Sprintf("%d", s.GPUMemory)
		}
		if s.GPUCores > 0 {
			limits["ascend.com/virAICore"] = fmt.Sprintf("%d", s.GPUCores)
		}
	case gpu.VendorCambricon:
		limits["cambricon.com/mlu"] = fmt.Sprintf("%d", s.GPUs)
		if s.GPUMemory > 0 {
			limits["cambricon.com/virMemory"] = fmt.Sprintf("%d", s.GPUMemory)
		}
		if s.GPUCores > 0 {
			limits["cambricon.com/virAICore"] = fmt.Sprintf("%d", s.GPUCores)
		}
	default: 
		limits["nvidia.com/gpu"] = fmt.Sprintf("%d", s.GPUs)
		if s.GPUMemory > 0 {
			limits["nvidia.com/gpumem"] = fmt.Sprintf("%d", s.GPUMemory)
		}
		if s.GPUCores > 0 {
			limits["nvidia.com/gpucores"] = fmt.Sprintf("%d", s.GPUCores)
		}
	}
	return limits
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}