package gpu

type Vendor string

const (
	VendorNVIDIA    Vendor = "NVIDIA"
	VendorAscend    Vendor = "Ascend"    
	VendorCambricon Vendor = "Cambricon" 
	VendorUnknown   Vendor = "Unknown"
)

type IsolationPolicy string

const (
	IsolationHAMi       IsolationPolicy = "hami"           
	IsolationAscendVDev IsolationPolicy = "ascend-virtual" 
	IsolationMLUIsolate IsolationPolicy = "mlu-isolate"    
	IsolationNone       IsolationPolicy = "none"
)

func (v Vendor) DefaultIsolation() IsolationPolicy {
	switch v {
	case VendorAscend:
		return IsolationAscendVDev
	case VendorCambricon:
		return IsolationMLUIsolate
	default:
		return IsolationHAMi
	}
}