package optimize

import "testing"

func TestQuantizeConfigValidate(t *testing.T) {
	if err := (QuantizeConfig{Method: "bad", Bits: 8}).Validate(); err == nil {
		t.Error("invalid method should fail")
	}
	if err := (QuantizeConfig{Method: QuantizeDynamic, Bits: 0}).Validate(); err == nil {
		t.Error("bits<=0 should fail")
	}
	
	if err := (QuantizeConfig{Method: QuantizeStatic, Bits: 8}).Validate(); err == nil {
		t.Error("static without calibration dataset should fail")
	}
	if err := (QuantizeConfig{Method: QuantizeStatic, Bits: 8, CalibrationDataset: "ds"}).Validate(); err != nil {
		t.Errorf("valid static config should pass: %v", err)
	}
	if err := (QuantizeConfig{Method: QuantizeDynamic, Bits: 8}).Validate(); err != nil {
		t.Errorf("valid dynamic config should pass: %v", err)
	}
}

func TestPruneConfigValidate(t *testing.T) {
	if err := (PruneConfig{Strategy: "bad", Sparsity: 0.5}).Validate(); err == nil {
		t.Error("invalid strategy should fail")
	}
	if err := (PruneConfig{Strategy: PruneStructured, Sparsity: 0}).Validate(); err == nil {
		t.Error("sparsity<=0 should fail")
	}
	if err := (PruneConfig{Strategy: PruneStructured, Sparsity: 1}).Validate(); err == nil {
		t.Error("sparsity>=1 should fail")
	}
	if err := (PruneConfig{Strategy: PruneUnstructured, Sparsity: 0.3}).Validate(); err != nil {
		t.Errorf("valid prune config should pass: %v", err)
	}
}

func TestDistillConfigValidate(t *testing.T) {
	if err := (DistillConfig{Temperature: 2, Alpha: 0.5}).Validate(); err == nil {
		t.Error("missing teacher uri should fail")
	}
	if err := (DistillConfig{TeacherModelURI: "t", Temperature: 0, Alpha: 0.5}).Validate(); err == nil {
		t.Error("temperature<=0 should fail")
	}
	if err := (DistillConfig{TeacherModelURI: "t", Temperature: 2, Alpha: 1.5}).Validate(); err == nil {
		t.Error("alpha out of range should fail")
	}
	if err := (DistillConfig{TeacherModelURI: "t", Temperature: 2, Alpha: 0.5}).Validate(); err != nil {
		t.Errorf("valid distill config should pass: %v", err)
	}
}

func TestConvertConfigValidate(t *testing.T) {
	if err := (ConvertConfig{TargetFormat: "bad"}).Validate(); err == nil {
		t.Error("invalid format should fail")
	}
	for _, f := range []string{ConvertONNX, ConvertTensorRT, ConvertOpenVINO} {
		if err := (ConvertConfig{TargetFormat: f}).Validate(); err != nil {
			t.Errorf("format %q should pass: %v", f, err)
		}
	}
}

func TestParseConfig(t *testing.T) {
	if _, err := ParseConfig(CompressionTypeQuantize, `{"method":"dynamic","bits":8}`); err != nil {
		t.Errorf("parse valid quantize: %v", err)
	}
	if _, err := ParseConfig(CompressionTypeQuantize, `{"method":"bad","bits":8}`); err == nil {
		t.Error("parse invalid quantize should fail")
	}
	if _, err := ParseConfig(CompressionType("unknown"), `{}`); err == nil {
		t.Error("unknown type should fail")
	}
}