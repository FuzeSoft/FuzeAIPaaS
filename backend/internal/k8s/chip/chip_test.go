package chip

import (
	"testing"

	"fuze-ai-paas/backend/internal/domain/gpu"
)

func TestRecognizeChipKnowledge(t *testing.T) {
	cases := []struct {
		dev     gpu.GPUDevice
		wantNPU bool
		wantMem int
	}{
		{gpu.NewGPUDevice("n", "NVIDIA", "A100 80GB", 1, 0), false, 80},
		{gpu.NewGPUDevice("n", "NVIDIA", "Tesla V100", 1, 0), false, 32},
		{gpu.NewGPUDevice("n", "NVIDIA", "A100 40GB", 1, 0), false, 40},
		{gpu.NewGPUDevice("n", "NVIDIA", "Tesla T4", 1, 0), false, 16},
		{gpu.NewGPUDevice("n", "华为", "Ascend 910", 1, 0), true, 32},
		{gpu.NewGPUDevice("n", "Ascend", "Ascend 910B", 1, 0), true, 32},
		{gpu.NewGPUDevice("n", "Cambricon", "MLU370", 1, 0), true, 80},
		{gpu.NewGPUDevice("n", "Unknown", "Mystery", 1, 0), false, 80},
	}
	for _, c := range cases {
		isNPU, mem := Recognize(c.dev)
		if isNPU != c.wantNPU || mem != c.wantMem {
			t.Fatalf("Recognize(%+v): NPU want %v got %v, mem want %d got %d",
				c.dev, c.wantNPU, isNPU, c.wantMem, mem)
		}
	}
}

func TestAnnotationsByVendor(t *testing.T) {
	
	nvidia := Annotations(Spec{Vendor: gpu.VendorNVIDIA, GPUs: 2, GPUMemory: 40, GPUCores: 80})
	if nvidia["nvidia.com/gpu"] != "2" {
		t.Errorf("NVIDIA 应有 nvidia.com/gpu=2，实际 %v", nvidia)
	}
	if nvidia["nvidia.com/gpumem"] != "40" {
		t.Errorf("NVIDIA 应有 nvidia.com/gpumem=40，实际 %v", nvidia)
	}
	if nvidia["nvidia.com/gpucores"] != "80" {
		t.Errorf("NVIDIA 应有 nvidia.com/gpucores=80，实际 %v", nvidia)
	}

	ascend := Annotations(Spec{Vendor: gpu.VendorAscend, GPUs: 1, GPUMemory: 32, GPUCores: 20})
	if ascend["ascend.com/vnpu"] != "1" {
		t.Errorf("Ascend 应有 ascend.com/vnpu=1，实际 %v", ascend)
	}
	if ascend["ascend.com/virMemory"] != "32" {
		t.Errorf("Ascend 应有 ascend.com/virMemory=32，实际 %v", ascend)
	}
	if ascend["ascend.com/virAICore"] != "20" {
		t.Errorf("Ascend 应有 ascend.com/virAICore=20，实际 %v", ascend)
	}

	cambricon := Annotations(Spec{Vendor: gpu.VendorCambricon, GPUs: 4, GPUMemory: 16})
	if cambricon["cambricon.com/mlu"] != "4" {
		t.Errorf("Cambricon 应有 cambricon.com/mlu=4，实际 %v", cambricon)
	}
	if cambricon["cambricon.com/virMemory"] != "16" {
		t.Errorf("Cambricon 应有 cambricon.com/virMemory=16，实际 %v", cambricon)
	}

	empty := Annotations(Spec{Vendor: gpu.VendorNVIDIA})
	if len(empty) != 0 {
		t.Errorf("无隔离诉求时注解应为空，实际 %v", empty)
	}
}

func TestResourceLimits(t *testing.T) {
	
	nv := Spec{Vendor: gpu.VendorNVIDIA, GPUs: 2}.ResourceLimits()
	if nv["nvidia.com/gpu"] != "2" {
		t.Errorf("NVIDIA limits 应有 nvidia.com/gpu=2，实际 %v", nv)
	}
	
	asc := Spec{Vendor: gpu.VendorAscend, GPUs: 1}.ResourceLimits()
	if asc["ascend.com/vnpu"] != "1" {
		t.Errorf("Ascend limits 应有 ascend.com/vnpu=1，实际 %v", asc)
	}
	
	cam := Spec{Vendor: gpu.VendorCambricon, GPUs: 4}.ResourceLimits()
	if cam["cambricon.com/mlu"] != "4" {
		t.Errorf("Cambricon limits 应有 cambricon.com/mlu=4，实际 %v", cam)
	}
	
	none := Spec{Vendor: gpu.VendorNVIDIA}.ResourceLimits()
	if len(none) != 0 {
		t.Errorf("无卡数时 limits 应为空，实际 %v", none)
	}
}

func TestVendorOf(t *testing.T) {
	if VendorOf("Ascend") != gpu.VendorAscend {
		t.Error("VendorOf(Ascend) 应为 Ascend")
	}
	if VendorOf("Cambricon") != gpu.VendorCambricon {
		t.Error("VendorOf(Cambricon) 应为 Cambricon")
	}
	if VendorOf("") != gpu.VendorNVIDIA {
		t.Error("VendorOf(空) 应默认 NVIDIA")
	}
	if VendorOf("unknown") != gpu.VendorNVIDIA {
		t.Error("VendorOf(未知) 应默认 NVIDIA")
	}
}

func TestNodeDeviceResourceKeys(t *testing.T) {
	keys := NodeDeviceResourceKeys()
	want := map[string]bool{
		"nvidia.com/gpu":  true,
		"ascend.com/npu":  true,
		"hygon.com/dcunum": true,
		"cambricon.com/mlu": true,
	}
	if len(keys) != len(want) {
		t.Fatalf("节点设备键数量应为 %d，实际 %d: %v", len(want), len(keys), keys)
	}
	for _, k := range keys {
		if !want[k] {
			t.Errorf("意外节点设备键 %q", k)
		}
	}
}

func TestVendorFromNodeLabels(t *testing.T) {
	cases := []struct {
		labels   map[string]string
		wantV    gpu.Vendor
		wantM    string
		wantOk   bool
	}{
		{map[string]string{"nvidia.com/gpu.product": "NVIDIA A100"}, gpu.VendorNVIDIA, "NVIDIA A100", true},
		{map[string]string{"ascend.com/chip-name": "Ascend 910"}, gpu.VendorAscend, "Ascend 910", true},
		{map[string]string{"hygon.com/dcuname": "DCU Z100"}, gpu.VendorUnknown, "DCU Z100", true},
		{map[string]string{"cambricon.com/mlu": "MLU370"}, gpu.VendorCambricon, "MLU370", true},
		{map[string]string{"kubernetes.io/hostname": "node-1"}, gpu.VendorUnknown, "", false},
	}
	for _, c := range cases {
		v, m, ok := VendorFromNodeLabels(c.labels)
		if ok != c.wantOk || v != c.wantV || m != c.wantM {
			t.Errorf("VendorFromNodeLabels(%v): got (%v,%q,%v), want (%v,%q,%v)",
				c.labels, v, m, ok, c.wantV, c.wantM, c.wantOk)
		}
	}
}

func TestJobResourceLimits(t *testing.T) {
	
	dedicated := Spec{Vendor: gpu.VendorNVIDIA, GPUs: 2}.JobResourceLimits()
	if dedicated["nvidia.com/gpu"] != "2" {
		t.Errorf("NVIDIA 整卡应写 nvidia.com/gpu=2，实际 %v", dedicated)
	}
	if len(dedicated) != 1 {
		t.Errorf("NVIDIA 整卡不应写细粒度隔离键，实际 %v", dedicated)
	}

	hami := Spec{Vendor: gpu.VendorNVIDIA, GPUs: 1, GPUMemory: 8192, GPUCores: 50}.JobResourceLimits()
	if hami["nvidia.com/gpu"] != "1" || hami["nvidia.com/gpumem"] != "8192" || hami["nvidia.com/gpucores"] != "50" {
		t.Errorf("NVIDIA HAMi 应写 gpu+gpumem+gpucores，实际 %v", hami)
	}

	asc := Spec{Vendor: gpu.VendorAscend, GPUs: 1, GPUMemory: 32, GPUCores: 20}.JobResourceLimits()
	if asc["ascend.com/vnpu"] != "1" || asc["ascend.com/virMemory"] != "32" || asc["ascend.com/virAICore"] != "20" {
		t.Errorf("Ascend 应写 vnpu+virMemory+virAICore，实际 %v", asc)
	}

	if len((Spec{}).JobResourceLimits()) != 0 {
		t.Error("无卡数时 JobResourceLimits 应为空")
	}
}