package k8s

import "fuze-ai-paas/backend/internal/domain/gpu"

func toGPUDevices(infos []NodeGPUInfo) []gpu.GPUDevice {
	devices := make([]gpu.GPUDevice, 0, len(infos))
	for _, info := range infos {
		devices = append(devices, gpu.NewGPUDevice(
			info.NodeName,
			info.GPUVendor,
			info.GPUModel,
			info.TotalGPUs,
			info.UsedGPUs,
		))
	}
	return devices
}