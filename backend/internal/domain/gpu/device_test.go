package gpu

import "testing"

func TestNewGPUDeviceNormalization(t *testing.T) {
	d := NewGPUDevice("n1", "", "", 8, 10)
	if d.Vendor != "NVIDIA" {
		t.Fatalf("厂商缺省应为 NVIDIA，实际 %s", d.Vendor)
	}
	if d.Model != "Unknown" {
		t.Fatalf("型号缺省应为 Unknown，实际 %s", d.Model)
	}
	if d.Used != 8 {
		t.Fatalf("Used 应收敛到 Total(8)，实际 %d", d.Used)
	}
	if d.Available() != 0 {
		t.Fatalf("满载时可用应为 0，实际 %d", d.Available())
	}

	d2 := NewGPUDevice("n2", "NVIDIA", "A800 80GB", 4, -3)
	if d2.Used != 0 {
		t.Fatalf("负数 Used 应收敛为 0，实际 %d", d2.Used)
	}
	if d2.Available() != 4 {
		t.Fatalf("可用应为 4，实际 %d", d2.Available())
	}
}

func TestIsNPU(t *testing.T) {
	if !NewGPUDevice("n", "华为", "Ascend 910", 1, 0).IsNPU() {
		t.Fatal("华为昇腾应识别为 NPU")
	}
	if !NewGPUDevice("n", "Ascend", "Ascend 910B", 1, 0).IsNPU() {
		t.Fatal("Ascend 型号应识别为 NPU")
	}
	if NewGPUDevice("n", "NVIDIA", "A800 80GB", 1, 0).IsNPU() {
		t.Fatal("NVIDIA A800 不应识别为 NPU")
	}
}

func TestPerCardMemoryMovedToChip(t *testing.T) {
	
	type noPerCardMemory interface {
		IsNPU() bool
		Available() int
	}
	var d GPUDevice
	var _ noPerCardMemory = d
}