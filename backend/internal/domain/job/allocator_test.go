package job

import "testing"

func TestCanSchedule(t *testing.T) {
	resources := []Resource{
		{Status: ResourceStatusAvailable, AvailableMemory: 60},
		{Status: ResourceStatusAvailable, AvailableMemory: 60},
	}
	if !CanSchedule(&Job{Memory: 100}, resources) {
		t.Fatalf("120 可用显存应能承载 100 的需求")
	}
	if CanSchedule(&Job{Memory: 200}, resources) {
		t.Fatalf("120 可用显存不应承载 200 的需求")
	}
	mixed := []Resource{
		{Status: ResourceStatusAllocated, AvailableMemory: 80},
		{Status: ResourceStatusAvailable, AvailableMemory: 40},
	}
	if CanSchedule(&Job{Memory: 50}, mixed) {
		t.Fatalf("仅 40 可用显存不应承载 50 的需求")
	}
}

func TestAllocate(t *testing.T) {
	resources := []Resource{
		{ID: "r1", Status: ResourceStatusAvailable, AvailableMemory: 80},
		{ID: "r2", Status: ResourceStatusAvailable, AvailableMemory: 80},
	}
	Allocate(&Job{Memory: 100}, resources)
	if resources[0].AvailableMemory != 0 || resources[0].Status != ResourceStatusAllocated {
		t.Fatalf("r1 期望 0/allocated，实际 %d/%s", resources[0].AvailableMemory, resources[0].Status)
	}
	if resources[1].AvailableMemory != 60 || resources[1].Status != ResourceStatusAvailable {
		t.Fatalf("r2 期望 60/available，实际 %d/%s", resources[1].AvailableMemory, resources[1].Status)
	}
}

func TestAllocateExactFit(t *testing.T) {
	resources := []Resource{
		{ID: "r1", Status: ResourceStatusAvailable, AvailableMemory: 100},
	}
	Allocate(&Job{Memory: 100}, resources)
	if resources[0].AvailableMemory != 0 || resources[0].Status != ResourceStatusAllocated {
		t.Fatalf("r1 期望 0/allocated，实际 %d/%s", resources[0].AvailableMemory, resources[0].Status)
	}
}

func TestAllocateSkipsAllocated(t *testing.T) {
	resources := []Resource{
		{ID: "r1", Status: ResourceStatusAllocated, AvailableMemory: 0},
		{ID: "r2", Status: ResourceStatusAvailable, AvailableMemory: 50},
	}
	Allocate(&Job{Memory: 50}, resources)
	if resources[1].AvailableMemory != 0 || resources[1].Status != ResourceStatusAllocated {
		t.Fatalf("r2 期望 0/allocated，实际 %d/%s", resources[1].AvailableMemory, resources[1].Status)
	}
}