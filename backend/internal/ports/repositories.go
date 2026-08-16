
package ports

import "fuze-ai-paas/backend/internal/models"

type ClusterRepository interface {
	GetCluster(id string) (*models.Cluster, error)
	UpsertClusterResources(clusterID string, incoming []models.Resource) error
	UpdateClusterStats(id string, stats models.Cluster) error
}

type JobRepository interface {
	GetJobs() ([]models.Job, error)
	GetPendingJobs() ([]models.Job, error)
	
	GetActiveJobs() ([]models.Job, error)
	
	GetJob(id string) (*models.Job, error)
	
	CreateJob(job *models.Job) error
	GetResourcesByCluster(clusterID string) ([]models.Resource, error)
	
	GetJobsByTenant(tenantID string) ([]models.Job, error)
	
	UpdateJobSpec(job *models.Job) error
	
	DeleteJob(id string) error
	
	UpdateJob(job *models.Job) error
	
	UpdateJobStatus(job *models.Job) error
	UpdateResource(resource *models.Resource) error
}

type InferenceRepository interface {
	GetInferenceServices() ([]models.InferenceService, error)
	GetInferenceService(id string) (*models.InferenceService, error)
	UpdateInferenceService(svc *models.InferenceService) error
	
	UpdateInferenceServiceSpec(svc *models.InferenceService) error
	
	UpdateInferenceRuntimeStatus(svc *models.InferenceService) error
	DeleteInferenceService(id string) error
}

type DatasetRepository interface {
	UpdateDataset(ds *models.Dataset) error
	DeleteDataset(id string) error
}

type MetricsRepository interface {
	GetResourcesByCluster(clusterID string) ([]models.Resource, error)
	GetResources() ([]models.Resource, error)
	GetJobs() ([]models.Job, error)
}

type ModelRepository interface {
	GetModels() ([]models.Model, error)
	GetModelsByTenant(tenantID string) ([]models.Model, error)
	GetModel(id string) (*models.Model, error)
	CreateModel(m *models.Model) error
	UpdateModel(m *models.Model) error
	DeleteModel(id string) error
	GetModelVersions(modelID string) ([]models.ModelVersion, error)
	GetModelVersion(modelID, versionID string) (*models.ModelVersion, error)
	CreateModelVersion(v *models.ModelVersion) error
	DeleteModelVersion(modelID, versionID string) error
}

type UserRepository interface {
	GetUserByUsername(username string) (*models.User, error)
	GetUser(id string) (*models.User, error)
	ListUsers() ([]models.User, error)
	CreateUser(u *models.User) error
	UpdateUser(u *models.User) error
	UpsertSSOUser(u *models.User) (*models.User, error)
}

type TenantRepository interface {
	ListTenants() ([]models.Tenant, error)
	GetTenant(id string) (*models.Tenant, error)
	CreateTenant(t *models.Tenant) error
	UpdateTenant(t *models.Tenant) error
	DeleteTenant(id string) error
}

type QuotaRepository interface {
	GetQuota(tenantID string) (*models.Quota, error)
	ListQuotas() ([]models.Quota, error)
	UpsertQuota(q *models.Quota) error
	CheckAndReserve(tenantID string, gpus, memGB, jobs int) error
	Release(tenantID string, gpus, memGB, jobs int) error
}

type AuditRepository interface {
	Record(entry *models.AuditLog) error
	ListAudit(q AuditQuery) ([]models.AuditLog, error)
}

type AuditQuery struct {
	Actor        string
	Action       string
	TenantID     string
	ResourceType string
	Limit        int
}

type DataRepository interface {
	
	CreatePipeline(p *models.DataPipeline) error
	GetPipeline(id string) (*models.DataPipeline, error)
	ListPipelines(tenantID string) ([]models.DataPipeline, error)
	
	ListActivePipelines() ([]models.DataPipeline, error)
	UpdatePipeline(p *models.DataPipeline) error
	
	CreateStep(s *models.PipelineStep) error
	GetSteps(pipelineID string) ([]models.PipelineStep, error)
	UpdateStep(s *models.PipelineStep) error
	
	CreateStepRun(r *models.PipelineStepRun) error
	UpdateStepRun(r *models.PipelineStepRun) error
	GetStepRuns(stepID string) ([]models.PipelineStepRun, error)
	
	CreateAnnotation(a *models.AnnotationTask) error
	GetAnnotation(id string) (*models.AnnotationTask, error)
	ListAnnotations(tenantID string) ([]models.AnnotationTask, error)
	UpdateAnnotation(a *models.AnnotationTask) error
}