
package ports

import (
	"context"
	"errors"
	"time"

	"fuze-ai-paas/backend/internal/models"
)

var ErrQuotaExceeded = errors.New("quota exceeded")

var ErrNotFound = errors.New("resource not found")

var ErrConflict = errors.New("resource already exists")

type ClusterReader interface {
	GetClusters() ([]models.Cluster, error)
	GetCluster(id string) (*models.Cluster, error)
	GetQueues() ([]models.Queue, error)
	GetResourcesByCluster(clusterID string) ([]models.Resource, error)
}

type ClusterWriter interface {
	ClusterReader
	CreateCluster(*models.Cluster) error
	UpdateCluster(*models.Cluster) error
	UpdateClusterStats(id string, stats models.Cluster) error
	DeleteCluster(id string) error
}

type JobReader interface {
	GetJobs() ([]models.Job, error)
	
	GetJobsByTenant(tenantID string) ([]models.Job, error)
	GetJob(id string) (*models.Job, error)
}

type JobWriter interface {
	JobReader
	CreateJob(*models.Job) error
	UpdateJob(*models.Job) error
	
	UpdateJobSpec(*models.Job) error
	
	UpdateJobStatus(*models.Job) error
	DeleteJob(id string) error
}

type InferenceReader interface {
	
	GetInferenceServices() ([]models.InferenceService, error)
	
	GetInferenceService(id string) (*models.InferenceService, error)
	
	GetInferenceServicesByTenant(tenantID string) ([]models.InferenceService, error)
	
	GetInferenceServiceForTenant(tenantID, id string) (*models.InferenceService, error)
}

type InferenceWriter interface {
	InferenceReader
	CreateInferenceService(*models.InferenceService) error
	UpdateInferenceService(*models.InferenceService) error
	
	UpdateInferenceServiceSpec(svc *models.InferenceService) error
	DeleteInferenceService(id string) error
	
	GetInferenceServiceByName(tenantID, name string) (*models.InferenceService, error)
	
	ApplySpec(tenantID string, svc *models.InferenceService, oldGPUs, oldMemGB, newGPUs, newMemGB int) error
	
	DeleteInferenceServiceAndReleaseQuota(id, tenantID string, gpus, memGB int) error
}

type ResourceReader interface {
	GetResources() ([]models.Resource, error)
	GetResourcesByCluster(clusterID string) ([]models.Resource, error)
	GetResource(id string) (*models.Resource, error)
}

type ResourceWriter interface {
	ResourceReader
	CreateResource(*models.Resource) error
}

type ModelReader interface {
	GetModels() ([]models.Model, error)
	GetModelsByTenant(tenantID string) ([]models.Model, error)
	GetModel(id string) (*models.Model, error)
	GetModelVersions(modelID string) ([]models.ModelVersion, error)
	GetModelVersion(modelID, versionID string) (*models.ModelVersion, error)
}

type ModelWriter interface {
	ModelReader
	CreateModel(*models.Model) error
	UpdateModel(*models.Model) error
	DeleteModel(id string) error
	CreateModelVersion(*models.ModelVersion) error
	DeleteModelVersion(modelID, versionID string) error
}

type DatasetReader interface {
	GetDatasets() ([]models.Dataset, error)
	
	GetDatasetsByTenant(tenantID string) ([]models.Dataset, error)
	GetDataset(id string) (*models.Dataset, error)
	
	GetDatasetForTenant(tenantID, id string) (*models.Dataset, error)
}

type DatasetWriter interface {
	DatasetReader
	CreateDataset(*models.Dataset) error
	DeleteDataset(id string) error
}

type TenantReader interface {
	ListTenants() ([]models.Tenant, error)
	GetTenant(id string) (*models.Tenant, error)
}

type TenantWriter interface {
	TenantReader
	CreateTenant(*models.Tenant) error
	UpdateTenant(*models.Tenant) error
	DeleteTenant(id string) error
}

type QuotaReader interface {
	GetQuota(tenantID string) (*models.Quota, error)
	ListQuotas() ([]models.Quota, error)
}

type QuotaWriter interface {
	QuotaReader
	UpsertQuota(*models.Quota) error
	CheckAndReserve(tenantID string, gpus, memGB, jobs int) error
	Release(tenantID string, gpus, memGB, jobs int) error
	
	AdjustReservation(tenantID string, oldGPUs, oldMemGB, newGPUs, newMemGB int) error
}

type AuditReader interface {
	ListAudit(opts AuditQuery) ([]models.AuditLog, error)
}

type AuditWriter interface {
	AuditReader
	Record(*models.AuditLog) error
}

type UserReader interface {
	GetUserByUsername(username string) (*models.User, error)
}

type UserWriter interface {
	UserReader
	UpsertSSOUser(u *models.User) (*models.User, error)
	
	UpdateUser(u *models.User) error
	
	UpdateUserMFARecovery(userID, recoveryEnc string) error
	
	GetUserByID(id string) (*models.User, error)
	
	RecordLoginFailure(username string, maxFails, lockSec int) (*models.User, bool, error)
	
	ClearLoginFailures(username string) error
}

type WorkspaceFilter struct {
	
	Status models.WorkspaceStatus
	
	OwnerID string
}

type WorkspaceReader interface {
	
	ListWorkspaces(tenantID string, filter WorkspaceFilter) ([]models.Workspace, error)
	
	GetWorkspaceForTenant(tenantID, id string) (*models.Workspace, error)
}

type WorkspaceWriter interface {
	WorkspaceReader
	CreateWorkspace(*models.Workspace) error
	UpdateWorkspace(*models.Workspace) error
	DeleteWorkspace(id string) error
	
	GetWorkspaceByName(tenantID, name string) (*models.Workspace, error)
	
	DeleteWorkspaceAndReleaseQuota(id, tenantID string, gpus, memGB int) error
}

type WorkspaceRepository interface {
	WorkspaceWriter
	
	TouchWorkspace(id string, at time.Time) error
	
	ListReclaimable(now time.Time) ([]models.Workspace, error)
}

type WorkspaceRuntime interface {
	
	Provision(ctx context.Context, ws *models.Workspace) (runtimeName string, err error)
	
	Deprovision(ctx context.Context, ws *models.Workspace) error
	
	Heartbeat(ctx context.Context, wsID string, at time.Time) error
	
	URL(ws *models.Workspace) (string, error)
	
	ProxyTarget(ws *models.Workspace) (string, error)
}