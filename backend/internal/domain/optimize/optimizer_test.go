package optimize

import "testing"

func TestSelectOptimizer(t *testing.T) {
	cases := []struct {
		typ     CompressionType
		backend CompressionBackend
		want    string
		wantErr bool
	}{
		{CompressionTypeQuantize, BackendPyTorch, "torch.quantization", false},
		{CompressionTypeQuantize, BackendONNXRuntime, "onnxruntime.quantization", false},
		{CompressionTypeQuantize, BackendOpenVINO, "openvino.tools.pot", false},
		{CompressionTypePrune, BackendPyTorch, "torch.prune", false},
		{CompressionTypePrune, BackendOpenVINO, "openvino.tools.pot", false},
		{CompressionTypePrune, BackendONNXRuntime, "", true}, 
		{CompressionTypeDistill, BackendPyTorch, "torch.distributed", false},
		{CompressionTypeDistill, BackendOpenVINO, "torch.distributed", false},
		{CompressionTypeConvert, BackendONNXRuntime, "onnxruntime.tools", false},
		{CompressionTypeConvert, BackendOpenVINO, "openvino.tools", false},
		{CompressionTypeConvert, BackendPyTorch, "torch.onnx.export", false},
	}
	for _, c := range cases {
		got, err := SelectOptimizer(c.typ, c.backend)
		if c.wantErr {
			if err == nil {
				t.Errorf("%s+%s: expected error, got %q", c.typ, c.backend, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s+%s: unexpected error %v", c.typ, c.backend, err)
			continue
		}
		if got != c.want {
			t.Errorf("%s+%s: got %q want %q", c.typ, c.backend, got, c.want)
		}
	}
}