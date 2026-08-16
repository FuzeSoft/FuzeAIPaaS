
package model

import "time"

type Framework string

const (
	FrameworkTensorFlow Framework = "tensorflow"
	FrameworkPyTorch    Framework = "pytorch"
	FrameworkONNX       Framework = "onnx"
	FrameworkSklearn    Framework = "sklearn"
	FrameworkXGBoost    Framework = "xgboost"
	FrameworkTriton     Framework = "triton"
	FrameworkVLLM       Framework = "vllm"
	FrameworkCustom     Framework = "custom"
)

type StorageBackend string

const (
	StorageS3    StorageBackend = "s3"
	StorageOSS   StorageBackend = "oss" 
	StoragePVC   StorageBackend = "pvc"
	StorageNFS   StorageBackend = "nfs"
	StorageLocal StorageBackend = "local"
)

type Model struct {
	ID          string
	Name        string
	Description string
	Framework   Framework
	Owner       string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}