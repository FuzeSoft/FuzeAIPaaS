
package gpu

type GPUDevice struct {
	NodeName string 
	Vendor   string 
	Model    string 
	Total    int    
	Used     int    
}

func (d GPUDevice) VendorEnum() Vendor {
	switch Vendor(d.Vendor) {
	case VendorAscend, VendorCambricon, VendorNVIDIA:
		return Vendor(d.Vendor)
	case "华为", "Huawei", "HUAWEI":
		return VendorAscend
	default:
		return VendorUnknown
	}
}

func NewGPUDevice(nodeName, vendor, model string, total, used int) GPUDevice {
	if vendor == "" {
		vendor = "NVIDIA"
	}
	if model == "" {
		model = "Unknown"
	}
	if total < 0 {
		total = 0
	}
	if used < 0 {
		used = 0
	}
	if used > total {
		used = total
	}
	return GPUDevice{
		NodeName: nodeName,
		Vendor:   vendor,
		Model:    model,
		Total:    total,
		Used:     used,
	}
}

func (d GPUDevice) IsNPU() bool {
	return d.VendorEnum() == VendorAscend || d.VendorEnum() == VendorCambricon
}

func (d GPUDevice) Available() int {
	if d.Total <= d.Used {
		return 0
	}
	return d.Total - d.Used
}