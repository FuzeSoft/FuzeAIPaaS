package training

import "testing"

func validSpec() Spec {
	return Spec{Image: "pytorch:2.3", Command: "python train.py", GPUs: 1, Memory: 16}
}

func TestSpecNormalizeTrimsAndDefaults(t *testing.T) {
	s := Spec{Image: "  pytorch:2.3  ", Framework: "  ", Distributed: false, Replicas: 4, MinAvailable: 2}
	s.Normalize()

	if s.Image != "pytorch:2.3" {
		t.Fatalf("image not trimmed: %q", s.Image)
	}
	
	if s.Replicas != 0 || s.MinAvailable != 0 {
		t.Fatalf("non-distributed spec must clear replica fields: %+v", s)
	}
}

func TestSpecNormalizeDemotesDistributedWithoutReplicas(t *testing.T) {
	s := Spec{Image: "i", Distributed: true, Replicas: 0}
	s.Normalize()
	if s.Distributed {
		t.Fatal("distributed without workers must be demoted to single-node")
	}
}

func TestSpecNormalizeDefaultsFrameworkForDistributed(t *testing.T) {
	s := Spec{Image: "i", Distributed: true, Replicas: 2}
	s.Normalize()
	if s.Framework != FrameworkPyTorchDDP {
		t.Fatalf("expected default distributed framework, got %q", s.Framework)
	}
}

func TestSpecValidateAcceptsMinimalSpec(t *testing.T) {
	s := validSpec()
	s.Normalize()
	if err := s.Validate(); err != nil {
		t.Fatalf("valid spec rejected: %v", err)
	}
}

func TestSpecValidateRequiresImage(t *testing.T) {
	s := Spec{Command: "python train.py"}
	if err := s.Validate(); err == nil {
		t.Fatal("image is mandatory")
	}
}

func TestSpecValidateRejectsNegativeAndOversizedResources(t *testing.T) {
	cases := map[string]Spec{
		"negative gpus":      {Image: "i", GPUs: -1},
		"oversized gpus":     {Image: "i", GPUs: maxGPUs + 1},
		"negative memory":    {Image: "i", Memory: -8},
		"oversized memory":   {Image: "i", Memory: maxMemoryGB + 1},
		"negative gpu mem":   {Image: "i", GPUMemory: -1},
		"negative gpu cores": {Image: "i", GPUCores: -1},
		"oversized cores":    {Image: "i", GPUCores: 101},
		"negative runtime":   {Image: "i", MaxRuntime: -1},
		"oversized runtime":  {Image: "i", MaxRuntime: maxRuntimeMinutes + 1},
	}
	for name, s := range cases {
		if err := s.Validate(); err == nil {
			t.Fatalf("%s must be rejected", name)
		}
	}
}

func TestSpecValidateDistributedConstraints(t *testing.T) {
	
	s := Spec{Image: "i", Distributed: true, Framework: FrameworkPyTorchDDP, Replicas: 2, MinAvailable: 4}
	if err := s.Validate(); err == nil {
		t.Fatal("min_available exceeding total replicas must be rejected")
	}

	s = Spec{Image: "i", Distributed: true, Framework: FrameworkPyTorchDDP, Replicas: maxReplicas + 1}
	if err := s.Validate(); err == nil {
		t.Fatal("replica count above the guard rail must be rejected")
	}

	s = Spec{Image: "i", Distributed: true, Framework: FrameworkPyTorchDDP, Replicas: 2, MinAvailable: -1}
	if err := s.Validate(); err == nil {
		t.Fatal("negative min_available must be rejected")
	}

	s = Spec{Image: "i", Distributed: true, Framework: "quantum-mpi", Replicas: 2}
	if err := s.Validate(); err == nil {
		t.Fatal("unknown distributed framework must be rejected")
	}
}

func TestSpecValidateRejectsOverlongCommand(t *testing.T) {
	long := make([]byte, maxCommandLen+1)
	for i := range long {
		long[i] = 'x'
	}
	s := Spec{Image: "i", Command: string(long)}
	if err := s.Validate(); err == nil {
		t.Fatal("overlong command must be rejected")
	}
}

func TestSpecResourceAccounting(t *testing.T) {
	single := Spec{Image: "i", GPUs: 8, Memory: 64}
	if got := single.TotalGPUs(); got != 8 {
		t.Fatalf("single-node TotalGPUs = %d, want 8", got)
	}
	if got := single.TotalMemory(); got != 64 {
		t.Fatalf("single-node TotalMemory = %d, want 64", got)
	}

	dist := Spec{Image: "i", GPUs: 8, Memory: 64, Distributed: true, Replicas: 15}
	if got := dist.EffectiveReplicas(); got != 16 {
		t.Fatalf("EffectiveReplicas = %d, want 16", got)
	}
	if got := dist.TotalGPUs(); got != 128 {
		t.Fatalf("distributed TotalGPUs = %d, want 128", got)
	}
	if got := dist.TotalMemory(); got != 1024 {
		t.Fatalf("distributed TotalMemory = %d, want 1024", got)
	}

	zero := Spec{Image: "i", GPUs: 0, Memory: 0, Distributed: true, Replicas: 3}
	if zero.TotalGPUs() != 0 || zero.TotalMemory() != 0 {
		t.Fatal("zero per-replica resources must total zero")
	}
}