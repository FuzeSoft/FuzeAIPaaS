package optimize

import "errors"

var (
	ErrNonPositiveCompressedSize = errors.New("compressed size must be positive")
	ErrNonPositiveLatency        = errors.New("compressed latency must be positive")
)

func ComputeCompressionRatio(origSize, compSize int64) (float64, error) {
	if compSize <= 0 {
		return 0, ErrNonPositiveCompressedSize
	}
	return float64(origSize) / float64(compSize), nil
}

func ComputeSpeedup(origLatency, compLatency float64) (float64, error) {
	if compLatency <= 0 {
		return 0, ErrNonPositiveLatency
	}
	return origLatency / compLatency, nil
}