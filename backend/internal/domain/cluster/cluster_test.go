package cluster

import (
	"testing"

	"fuze-ai-paas/backend/internal/domain/gpu"
)

func TestDiscoverAggregatesInventory(t *testing.T) {
	c := New("c1", "C1")
	devices := []gpu.GPUDevice{
		gpu.NewGPUDevice("n1", "NVIDIA", "A800 80GB", 4, 2),
		gpu.NewGPUDevice("n2", "华为", "Ascend 910", 8, 0),
	}
	ev := c.Discover(devices)

	if c.NodeCount != 2 || c.TotalGPUs != 12 || c.UsedGPUs != 2 {
		t.Fatalf("aggregate: nodes=%d total=%d used=%d", c.NodeCount, c.TotalGPUs, c.UsedGPUs)
	}
	if c.Status != StatusHealthy {
		t.Fatalf("status=%s", c.Status)
	}

	if ev.EventType() != "ClusterDiscovered" || ev.AggregateID() != "c1" {
		t.Fatalf("event meta: %+v", ev)
	}
	if ev.TotalGPUs != 12 || ev.UsedGPUs != 2 || ev.NodeCount != 2 {
		t.Fatalf("event stats: %+v", ev)
	}
}

func TestDiscoverEmptyInventory(t *testing.T) {
	c := New("c1", "C1")
	ev := c.Discover(nil)
	if ev.NodeCount != 0 || ev.TotalGPUs != 0 || ev.UsedGPUs != 0 {
		t.Fatalf("empty discover stats: %+v", ev)
	}
	if c.Status != StatusHealthy {
		t.Fatalf("empty cluster should still be healthy: %s", c.Status)
	}
}