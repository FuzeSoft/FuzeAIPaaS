package optimize

import "fmt"

func SelectOptimizer(t CompressionType, b CompressionBackend) (string, error) {
	switch t {
	case CompressionTypeQuantize:
		switch b {
		case BackendPyTorch:
			return "torch.quantization", nil
		case BackendONNXRuntime:
			return "onnxruntime.quantization", nil
		case BackendOpenVINO:
			return "openvino.tools.pot", nil
		}
	case CompressionTypePrune:
		switch b {
		case BackendPyTorch:
			return "torch.prune", nil
		case BackendOpenVINO:
			return "openvino.tools.pot", nil
		}
	case CompressionTypeDistill:
		
		return "torch.distributed", nil
	case CompressionTypeConvert:
		switch b {
		case BackendONNXRuntime:
			return "onnxruntime.tools", nil
		case BackendOpenVINO:
			return "openvino.tools", nil
		case BackendPyTorch:
			return "torch.onnx.export", nil
		}
	}
	return "", fmt.Errorf("no optimizer for compression type %q with backend %q", t, b)
}