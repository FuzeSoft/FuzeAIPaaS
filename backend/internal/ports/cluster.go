package ports

import (
	"context"
	"errors"

	"fuze-ai-paas/backend/internal/domain/gpu"
	"fuze-ai-paas/backend/internal/models"
)

const DefaultNamespace = "fuze-ai-paas"

var ErrPodNotFound = errors.New("pod does not belong to this job")

var ErrClusterNotRegistered = errors.New("cluster not registered")

type LogQuery struct {
	Pod       string 
	Task      string 
	TailLines int    
}

type PodRef struct {
	Name  string `json:"name"`
	Task  string `json:"task"`
	Phase string `json:"phase"`
}

type JobLogs struct {
	Logs string
	Pods []PodRef
}

type ClusterClientPort interface {
	Enabled() bool
	Namespace() string
	ServerVersion(ctx context.Context) (string, error)
	DiscoverGPUInventory(ctx context.Context) ([]gpu.GPUDevice, error)
	CreateVolcanoJob(ctx context.Context, job *models.Job) (string, error)
	DeleteVolcanoJob(ctx context.Context, name string) error
	GetVolcanoJobStatus(ctx context.Context, name string) (models.JobState, error)
	SyncJobStatuses(ctx context.Context) (map[string]models.JobState, error)
	
	GetJobLogs(ctx context.Context, job *models.Job, query LogQuery) (JobLogs, error)
	CreateInferenceService(ctx context.Context, svc *models.InferenceService) (string, error)
	DeleteInferenceService(ctx context.Context, name string) error
	CreateDataset(ctx context.Context, ds *models.Dataset) error
	DeleteDataset(ctx context.Context, ds *models.Dataset) error
}

type ClusterRegistry interface {
	Register(cluster *models.Cluster) error
	Unregister(clusterID string)
	Get(clusterID string) (ClusterClientPort, error)
	List() []string
	LoadAll(clusters []models.Cluster) []error
	
	K8sClient(clusterID string) (ClusterClientPort, error)
}