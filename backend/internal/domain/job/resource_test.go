package job

import "testing"

func domainResource() Resource {
	return Resource{
		ID:              "r1",
		Type:            ResourceTypeGPU,
		Status:          ResourceStatusAvailable,
		TotalGPUs:       0,
		TotalMemory:     80,
		AvailableMemory: 80,
	}
}

func TestGPUCount(t *testing.T) {
	if domainResource().GPUCount() != 1 {
		t.Fatal("单卡资源（TotalGPUs=0）应计 1 卡")
	}
	if (Resource{TotalGPUs: 4}).GPUCount() != 4 {
		t.Fatal("多卡节点应计 TotalGPUs 卡")
	}
}

func TestGPUAllocated(t *testing.T) {
	if (Resource{UsedGPUs: 2}).GPUAllocated() != 2 {
		t.Fatal("UsedGPUs>0 时直接返回该值")
	}
	if (Resource{UsedGPUs: 0, Status: ResourceStatusAllocated, TotalGPUs: 4}).GPUAllocated() != 4 {
		t.Fatal("allocated 多卡节点应计满 TotalGPUs")
	}
	if (Resource{UsedGPUs: 0, Status: ResourceStatusAllocated}).GPUAllocated() != 1 {
		t.Fatal("allocated 单卡应计 1")
	}
	if (Resource{UsedGPUs: 0, Status: ResourceStatusAvailable}).GPUAllocated() != 0 {
		t.Fatal("available 且未用时应为 0")
	}
}

func TestResourceAllocate(t *testing.T) {
	r := domainResource()
	if got := r.Allocate(30); got != 30 {
		t.Fatalf("应扣 30，实际扣 %d", got)
	}
	if r.AvailableMemory != 50 || r.Status != ResourceStatusAvailable {
		t.Fatalf("剩余 50/available，实际 %d/%s", r.AvailableMemory, r.Status)
	}
	if got := r.Allocate(50); got != 50 {
		t.Fatalf("应扣 50，实际扣 %d", got)
	}
	if r.AvailableMemory != 0 || r.Status != ResourceStatusAllocated {
		t.Fatalf("占满后应为 0/allocated，实际 %d/%s", r.AvailableMemory, r.Status)
	}
	if got := r.Allocate(50); got != 0 {
		t.Fatalf("已分配资源不应再扣减，实际扣 %d", got)
	}
}