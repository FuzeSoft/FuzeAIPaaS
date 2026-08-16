
package optimize

import "errors"

const (
	CompressionTypeQuantize = CompressionType("quantize")
	CompressionTypePrune    = CompressionType("prune")
	CompressionTypeDistill  = CompressionType("distill")
	CompressionTypeConvert  = CompressionType("convert")
)

const (
	StatusPending   = CompressionStatus("pending")
	StatusRunning   = CompressionStatus("running")
	StatusSucceeded = CompressionStatus("succeeded")
	StatusFailed    = CompressionStatus("failed")
	StatusCancelled = CompressionStatus("cancelled")
)

const (
	BackendPyTorch     = CompressionBackend("pytorch")
	BackendONNXRuntime = CompressionBackend("onnxruntime")
	BackendOpenVINO    = CompressionBackend("openvino")
)

var errIllegalTransition = errors.New("illegal compression status transition")

var allowedTransitions = map[CompressionStatus]map[CompressionStatus]struct{}{
	StatusPending: {
		StatusRunning:   {},
		StatusCancelled: {},
	},
	StatusRunning: {
		StatusSucceeded: {},
		StatusFailed:    {},
		StatusCancelled: {},
	},
}

type CompressionType string

type CompressionStatus string

type CompressionBackend string

type CompressionTask struct {
	ID             string  `json:"id"`
	TenantID       string  `json:"tenantId"`
	Name           string  `json:"name"`
	Type           CompressionType `json:"type"`
	Backend        CompressionBackend `json:"backend"`
	ConfigJSON     string  `json:"config"`
	ModelVersionID string  `json:"modelVersionId"`
	Status         CompressionStatus `json:"status"`

	JobID         string  `json:"jobId,omitempty"`        
	GateThreshold float64 `json:"gateThreshold"`          
	OrigAccuracy  float64 `json:"origAccuracy,omitempty"`  
	GatePass      bool    `json:"gatePass,omitempty"`      
	FailReason    string  `json:"failReason,omitempty"`    

	CompressedSizeBytes int64   `json:"compressedSizeBytes,omitempty"`
	LatencyMs           float64 `json:"latencyMs,omitempty"`
	Accuracy            float64 `json:"accuracy,omitempty"`
	ArtifactURI         string  `json:"artifactUri,omitempty"`
	CompressionRatio    float64 `json:"compressionRatio,omitempty"`
	Speedup             float64 `json:"speedup,omitempty"`
}

func NewCompressionTask(id, tenantID, name string, typ CompressionType, backend CompressionBackend, configJSON, modelVersionID string) *CompressionTask {
	return &CompressionTask{
		ID:             id,
		TenantID:       tenantID,
		Name:           name,
		Type:           typ,
		Backend:        backend,
		ConfigJSON:     configJSON,
		ModelVersionID: modelVersionID,
		Status:         StatusPending,
	}
}

func (t *CompressionTask) CanTransitionTo(target CompressionStatus) bool {
	if t.Status == target {
		return false 
	}
	edges, ok := allowedTransitions[t.Status]
	if !ok {
		return false
	}
	_, ok = edges[target]
	return ok
}

func (t *CompressionTask) TransitionTo(target CompressionStatus) error {
	if !t.CanTransitionTo(target) {
		return errIllegalTransition
	}
	t.Status = target
	return nil
}