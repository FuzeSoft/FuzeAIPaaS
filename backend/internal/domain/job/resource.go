package job

type Resource struct {
	ID              string
	ClusterID       string
	Name            string
	Type            ResourceType
	Vendor          string
	Model           string
	TotalGPUs       int
	UsedGPUs        int
	TotalMemory     int
	AvailableMemory int
	Status          ResourceStatus
	NodeName        string
}

func (r Resource) IsAvailable() bool {
	return r.Status == ResourceStatusAvailable
}

func (r Resource) GPUCount() int {
	if r.TotalGPUs > 0 {
		return r.TotalGPUs
	}
	return 1
}

func (r Resource) GPUAllocated() int {
	if r.UsedGPUs > 0 {
		return r.UsedGPUs
	}
	if r.Status == ResourceStatusAllocated {
		return r.GPUCount()
	}
	return 0
}

func (r *Resource) Allocate(memory int) int {
	if !r.IsAvailable() || memory <= 0 {
		return 0
	}
	allocate := minInt(r.AvailableMemory, memory)
	r.AvailableMemory -= allocate
	if r.AvailableMemory == 0 {
		r.Status = ResourceStatusAllocated
	}
	return allocate
}