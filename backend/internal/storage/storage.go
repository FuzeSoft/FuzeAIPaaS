package storage

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"sync/atomic"
	"time"

	"fuze-ai-paas/backend/internal/crypto/aes"
	"fuze-ai-paas/backend/internal/models"
	"fuze-ai-paas/backend/internal/ports"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var nowFunc = time.Now

func now() time.Time { return nowFunc() }

func SetClock(fn func() time.Time) { nowFunc = fn }

type Storage struct {
	db *gorm.DB
	
	cipher *aes.Cipher

	*dataRepo
}

func NewStorage(db *gorm.DB) *Storage {
	return &Storage{db: db, dataRepo: newDataRepo(db)}
}

func (s *Storage) Now() time.Time { return now() }

func (s *Storage) SetCipher(c *aes.Cipher) {
	s.cipher = c
}

func (s *Storage) encryptKubeConfig(cluster *models.Cluster) error {
	if cluster.KubeConfig == "" {
		return nil
	}
	if s.cipher == nil {
		
		cluster.KubeConfigEnc = ""
		return nil
	}
	enc, err := s.cipher.EncryptString(cluster.KubeConfig)
	if err != nil {
		return err
	}
	cluster.KubeConfigEnc = enc
	cluster.KubeConfig = "" 
	return nil
}

func (s *Storage) decryptKubeConfig(cluster *models.Cluster) error {
	if cluster.KubeConfigEnc == "" {
		
		if cluster.KubeConfig != "" && s.cipher != nil {
			if err := s.encryptKubeConfig(cluster); err != nil {
				return err
			}
		}
		return nil
	}
	if s.cipher == nil {
		return fmt.Errorf("storage: cluster %s has encrypted kubeconfig but no cipher configured", cluster.ID)
	}
	plain, err := s.cipher.DecryptString(cluster.KubeConfigEnc)
	if err != nil {
		return fmt.Errorf("storage: decrypt kubeconfig for cluster %s: %w", cluster.ID, err)
	}
	cluster.KubeConfig = plain
	cluster.KubeConfigEnc = "" 
	return nil
}

func (s *Storage) Experiment() ports.ExperimentRepository {
	return NewExperimentRepository(s.db)
}

type AuditQuery = ports.AuditQuery

var (
	_ ports.ClusterRepository   = (*Storage)(nil)
	_ ports.JobRepository       = (*Storage)(nil)
	_ ports.InferenceRepository = (*Storage)(nil)
	_ ports.DatasetRepository   = (*Storage)(nil)
	_ ports.MetricsRepository   = (*Storage)(nil)
	_ ports.ModelRepository     = (*Storage)(nil)
	_ ports.UserRepository      = (*Storage)(nil)
	_ ports.TenantRepository    = (*Storage)(nil)
	_ ports.QuotaRepository     = (*Storage)(nil)
	_ ports.AuditRepository     = (*Storage)(nil)
	_ ports.WorkspaceRepository = (*Storage)(nil)

	_ ports.ClusterWriter   = (*Storage)(nil)
	_ ports.JobWriter       = (*Storage)(nil)
	_ ports.InferenceWriter = (*Storage)(nil)
	_ ports.ResourceWriter  = (*Storage)(nil)
	_ ports.ModelWriter     = (*Storage)(nil)
	_ ports.DatasetWriter   = (*Storage)(nil)
	_ ports.TenantWriter    = (*Storage)(nil)
	_ ports.QuotaWriter     = (*Storage)(nil)
	_ ports.AuditWriter     = (*Storage)(nil)
	_ ports.UserWriter      = (*Storage)(nil)

	_ ports.RouteRepository      = (*llmRouteRepo)(nil)
	_ ports.TokenQuotaRepository = (*llmQuotaRepo)(nil)
	_ ports.TokenUsageRepository = (*llmUsageRepo)(nil)
	_ ports.TraceRepository      = (*llmTraceRepo)(nil)
	_ ports.PromptRepository     = (*llmPromptRepo)(nil)
	_ ports.KnowledgeRepository  = (*llmKnowledgeRepo)(nil)
)

func (s *Storage) CreateResource(resource *models.Resource) error {
	resource.ID = generateID()
	resource.CreatedAt = now()
	resource.UpdatedAt = now()
	return s.db.Create(resource).Error
}

func (s *Storage) GetResources() ([]models.Resource, error) {
	var resources []models.Resource
	err := s.db.Find(&resources).Error
	return resources, err
}

func (s *Storage) GetResourcesByCluster(clusterID string) ([]models.Resource, error) {
	var resources []models.Resource
	err := s.db.Where("cluster_id = ?", clusterID).Find(&resources).Error
	return resources, err
}

func (s *Storage) UpsertClusterResources(clusterID string, incoming []models.Resource) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		for i := range incoming {
			res := incoming[i]
			res.ClusterID = clusterID
			var existing models.Resource
			err := tx.Where("cluster_id = ? AND name = ?", clusterID, res.Name).First(&existing).Error
			if err != nil {
				
				res.ID = generateID()
				now := now()
				res.CreatedAt = now
				res.UpdatedAt = now
				if err := tx.Create(&res).Error; err != nil {
					return err
				}
				continue
			}
			
			existing.Type = res.Type
			existing.Vendor = res.Vendor
			existing.Model = res.Model
			existing.TotalGPUs = res.TotalGPUs
			existing.UsedGPUs = res.UsedGPUs
			existing.TotalMemory = res.TotalMemory
			existing.AvailableMemory = res.AvailableMemory
			existing.Status = res.Status
			existing.UpdatedAt = now()
			if err := tx.Save(&existing).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Storage) GetResource(id string) (*models.Resource, error) {
	var resource models.Resource
	if err := s.db.First(&resource, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &resource, nil
}

func (s *Storage) UpdateResource(resource *models.Resource) error {
	resource.UpdatedAt = now()
	return s.db.Save(resource).Error
}

func (s *Storage) DeleteResource(id string) error {
	return s.db.Delete(&models.Resource{}, "id = ?", id).Error
}

func (s *Storage) CreateJob(job *models.Job) error {
	job.ID = generateID()
	job.Status = models.JobStatusPending
	job.CreatedAt = now()
	job.UpdatedAt = now()
	return s.db.Create(job).Error
}

func (s *Storage) GetJobs() ([]models.Job, error) {
	var jobs []models.Job
	err := s.db.Order("created_at DESC").Find(&jobs).Error
	return jobs, err
}

func (s *Storage) GetJobsByTenant(tenantID string) ([]models.Job, error) {
	var jobs []models.Job
	q := s.db.Model(&models.Job{})
	if tenantID != "" {
		q = q.Where("tenant_id = ?", tenantID)
	}
	err := q.Order("created_at DESC").Find(&jobs).Error
	return jobs, err
}

func (s *Storage) GetJob(id string) (*models.Job, error) {
	var job models.Job
	if err := s.db.First(&job, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &job, nil
}

func (s *Storage) JobExistsForTenant(ctx context.Context, tenantID, jobID string) (bool, error) {
	if jobID == "" {
		return false, nil
	}
	q := s.db.WithContext(ctx).Model(&models.Job{}).Where("id = ?", jobID)
	if tenantID != "" {
		q = q.Where("tenant_id = ?", tenantID)
	}

	var count int64
	if err := q.Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func (s *Storage) UpdateJob(job *models.Job) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var existing models.Job
		if err := tx.Select("id").First(&existing, "id = ?", job.ID).Error; err != nil {
			return err
		}
		job.UpdatedAt = now()
		return tx.Save(job).Error
	})
}

func (s *Storage) UpdateJobSpec(job *models.Job) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var existing models.Job
		if err := tx.Select("id").First(&existing, "id = ?", job.ID).Error; err != nil {
			return err
		}
		job.UpdatedAt = now()
		return tx.Model(&models.Job{}).
			Where("id = ?", job.ID).
			Select(
				"Name", "Type", "Priority", "Image", "Command",
				"GPUs", "Memory", "GPUMemory", "GPUCores",
				"Distributed", "Framework", "Replicas", "MinAvailable",
				"DatasetName", "MountPath", "MaxRuntime",
				"CheckpointEnabled", "CheckpointInterval", "CheckpointMaxRetries",
				"RegisterModelEnabled", "RegisterModelID", "RegisterVersionTag",
				"CodeCommit", "TemplateID", "UpdatedAt",
			).
			Updates(job).Error
	})
}

func (s *Storage) UpdateJobStatus(job *models.Job) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var existing models.Job
		if err := tx.Select("id").First(&existing, "id = ?", job.ID).Error; err != nil {
			return err
		}
		job.UpdatedAt = now()
		return tx.Model(&models.Job{}).
			Where("id = ?", job.ID).
			Select(
				"Status", "FailureReason",
				"SubmitAttempts", "LastSubmitAt",
				"StartedAt", "VolcanoJobName", "QueueName",
				"LatestCheckpointURI", "LatestCheckpointStep",
				"LatestCheckpointAt", "LatestCheckpointHash", "LatestCheckpointSizeBytes",
				"ResumeFrom", "RetryAttempts", "RegisteredVersionID",
				"RegisterAdapterEnabled", "AdapterBaseModel", "AdapterMethod", "AdapterRank", "RegisteredAdapterID",
				"UpdatedAt",
			).
			Updates(job).Error
	})
}

func (s *Storage) DeleteJob(id string) error {
	return s.db.Delete(&models.Job{}, "id = ?", id).Error
}

func (s *Storage) GetPendingJobs() ([]models.Job, error) {
	var jobs []models.Job
	err := s.db.Where("status = ?", models.JobStatusPending).Order("priority DESC, created_at ASC").Find(&jobs).Error
	return jobs, err
}

func (s *Storage) GetActiveJobs() ([]models.Job, error) {
	var jobs []models.Job
	err := s.db.
		
		Where("status NOT IN ?", []models.JobStatus{
			models.JobStatusCompleted,
			models.JobStatusFailed,
			models.JobStatusCancelled,
		}).
		Order("priority DESC, created_at ASC").
		Find(&jobs).Error
	return jobs, err
}

func (s *Storage) GetAvailableResources() ([]models.Resource, error) {
	var resources []models.Resource
	err := s.db.Where("status = ? AND available_memory > 0", models.ResourceStatusAvailable).Find(&resources).Error
	return resources, err
}

func (s *Storage) CreateInferenceService(svc *models.InferenceService) error {
	svc.ID = generateID()
	if svc.Status == "" {
		svc.Status = models.InferenceStatusPending
	}
	svc.CreatedAt = now()
	svc.UpdatedAt = now()
	return s.db.Create(svc).Error
}

func (s *Storage) GetInferenceServices() ([]models.InferenceService, error) {
	var svcs []models.InferenceService
	err := s.db.Order("created_at DESC").Find(&svcs).Error
	return svcs, err
}

func (s *Storage) GetInferenceServicesByTenant(tenantID string) ([]models.InferenceService, error) {
	var svcs []models.InferenceService
	q := s.db.Order("created_at DESC")
	if tenantID != "" {
		q = q.Where("tenant_id = ?", tenantID)
	}
	err := q.Find(&svcs).Error
	return svcs, err
}

func (s *Storage) GetInferenceService(id string) (*models.InferenceService, error) {
	var svc models.InferenceService
	err := s.db.First(&svc, "id = ?", id).Error
	return &svc, err
}

func (s *Storage) GetInferenceServiceForTenant(tenantID, id string) (*models.InferenceService, error) {
	var svc models.InferenceService
	q := s.db.Where("id = ?", id)
	if tenantID != "" {
		q = q.Where("tenant_id = ?", tenantID)
	}
	if err := q.First(&svc).Error; err != nil {
		return nil, err
	}
	return &svc, nil
}

func (s *Storage) GetInferenceServiceByName(tenantID, name string) (*models.InferenceService, error) {
	var svc models.InferenceService
	if err := s.db.First(&svc, "tenant_id = ? AND name = ?", tenantID, name).Error; err != nil {
		return nil, err
	}
	return &svc, nil
}

func (s *Storage) UpdateInferenceService(svc *models.InferenceService) error {
	svc.UpdatedAt = now()
	return s.db.Save(svc).Error
}

func (s *Storage) UpdateInferenceServiceSpec(svc *models.InferenceService) error {
	svc.UpdatedAt = now()
	return s.db.Model(&models.InferenceService{}).
		Where("id = ?", svc.ID).
		Select(
			"Name", "Framework", "StorageURI", "Image", "RuntimeVer",
			"MinReplicas", "MaxReplicas", "CPU", "Memory",
			"GPUs", "GPUMemory", "GPUCores", "Chip",
			"Runtime", "TargetReplicas", "CanaryWeight", "UpdatedAt",
		).
		Updates(svc).Error
}

func (s *Storage) UpdateInferenceRuntimeStatus(svc *models.InferenceService) error {
	svc.UpdatedAt = now()
	return s.db.Model(&models.InferenceService{}).
		Where("id = ?", svc.ID).
		Select("Status", "URL", "KServeName", "ReadyReplicas", "FailureReason", "UpdatedAt").
		Updates(svc).Error
}

func (s *Storage) ApplySpec(tenantID string, svc *models.InferenceService, oldGPUs, oldMemGB, newGPUs, newMemGB int) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := adjustReservationTx(tx, tenantID, oldGPUs, oldMemGB, newGPUs, newMemGB); err != nil {
			return err
		}
		svc.UpdatedAt = now()
		return tx.Model(&models.InferenceService{}).
			Where("id = ?", svc.ID).
			Select(
				"Name", "Framework", "StorageURI", "Image", "RuntimeVer",
				"MinReplicas", "MaxReplicas", "CPU", "Memory",
				"GPUs", "GPUMemory", "GPUCores", "Chip",
				"Runtime", "TargetReplicas", "CanaryWeight", "KServeName", "UpdatedAt",
			).
			Updates(svc).Error
	})
}

func (s *Storage) DeleteInferenceService(id string) error {
	return s.db.Delete(&models.InferenceService{}, "id = ?", id).Error
}

func (s *Storage) DeleteInferenceServiceAndReleaseQuota(id, tenantID string, gpus, memGB int) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		
		var q models.Quota
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("tenant_id = ?", tenantID).First(&q).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				
				return tx.Delete(&models.InferenceService{}, "id = ?", id).Error
			}
			return err
		}
		q.GPUUsed = max(0, q.GPUUsed-gpus)
		q.MemoryUsedGB = max(0, q.MemoryUsedGB-memGB)
		q.JobUsed = max(0, q.JobUsed-1)
		if err := tx.Save(&q).Error; err != nil {
			return err
		}
		return tx.Delete(&models.InferenceService{}, "id = ?", id).Error
	})
}

func (s *Storage) CreateDataset(ds *models.Dataset) error {
	ds.ID = generateID()
	if ds.Status == "" {
		ds.Status = models.DatasetStatusPending
	}
	ds.CreatedAt = now()
	ds.UpdatedAt = now()
	return s.db.Create(ds).Error
}

func (s *Storage) GetDatasets() ([]models.Dataset, error) {
	var datasets []models.Dataset
	err := s.db.Order("created_at DESC").Find(&datasets).Error
	return datasets, err
}

func (s *Storage) GetDatasetsByTenant(tenantID string) ([]models.Dataset, error) {
	var datasets []models.Dataset
	q := s.db.Model(&models.Dataset{})
	if tenantID != "" {
		q = q.Where("tenant_id = ?", tenantID)
	}
	err := q.Order("created_at DESC").Find(&datasets).Error
	return datasets, err
}

func (s *Storage) GetDataset(id string) (*models.Dataset, error) {
	var ds models.Dataset
	err := s.db.First(&ds, "id = ?", id).Error
	return &ds, err
}

func (s *Storage) GetDatasetForTenant(tenantID, id string) (*models.Dataset, error) {
	var ds models.Dataset
	q := s.db.Where("id = ?", id)
	if tenantID != "" {
		q = q.Where("tenant_id = ?", tenantID)
	}
	if err := q.First(&ds).Error; err != nil {
		return nil, err
	}
	return &ds, nil
}

func (s *Storage) GetDatasetByName(name string) (*models.Dataset, error) {
	var ds models.Dataset
	err := s.db.First(&ds, "name = ?", name).Error
	return &ds, err
}

func (s *Storage) UpdateDataset(ds *models.Dataset) error {
	ds.UpdatedAt = now()
	return s.db.Save(ds).Error
}

func (s *Storage) DeleteDataset(id string) error {
	return s.db.Delete(&models.Dataset{}, "id = ?", id).Error
}

func (s *Storage) CreateModel(model *models.Model) error {
	model.ID = generateID()
	model.CreatedAt = now()
	model.UpdatedAt = now()
	return s.db.Create(model).Error
}

func (s *Storage) GetModels() ([]models.Model, error) {
	var list []models.Model
	err := s.db.Order("created_at DESC").Find(&list).Error
	return list, err
}

func (s *Storage) GetModelsByTenant(tenantID string) ([]models.Model, error) {
	var list []models.Model
	q := s.db.Model(&models.Model{})
	if tenantID != "" {
		q = q.Where("tenant_id = ?", tenantID)
	}
	err := q.Order("created_at DESC").Find(&list).Error
	return list, err
}

func (s *Storage) GetModel(id string) (*models.Model, error) {
	var m models.Model
	if err := s.db.First(&m, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &m, nil
}

func (s *Storage) UpdateModel(model *models.Model) error {
	model.UpdatedAt = now()
	return s.db.Transaction(func(tx *gorm.DB) error {
		var existing models.Model
		if err := tx.Select("id").First(&existing, "id = ?", model.ID).Error; err != nil {
			return err
		}
		return tx.Model(&models.Model{}).
			Where("id = ?", model.ID).
			Select("Name", "Description", "Framework", "Owner", "UpdatedAt").
			Updates(model).Error
	})
}

func (s *Storage) DeleteModel(id string) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("model_id = ?", id).Delete(&models.ModelVersion{}).Error; err != nil {
			return err
		}
		if err := tx.Where("id = ?", id).Delete(&models.Model{}).Error; err != nil {
			return err
		}
		return nil
	})
}

func (s *Storage) CreateModelVersion(version *models.ModelVersion) error {
	if version.ID == "" {
		version.ID = generateID()
	}
	version.CreatedAt = now()
	return s.db.Create(version).Error
}

func (s *Storage) GetModelVersions(modelID string) ([]models.ModelVersion, error) {
	var versions []models.ModelVersion
	err := s.db.Where("model_id = ?", modelID).Order("created_at DESC").Find(&versions).Error
	return versions, err
}

func (s *Storage) GetModelVersion(modelID string, versionID string) (*models.ModelVersion, error) {
	var v models.ModelVersion
	err := s.db.First(&v, "id = ? AND model_id = ?", versionID, modelID).Error
	return &v, err
}

func (s *Storage) DeleteModelVersion(modelID string, versionID string) error {
	return s.db.Where("id = ? AND model_id = ?", versionID, modelID).Delete(&models.ModelVersion{}).Error
}

func (s *Storage) GetClusters() ([]models.Cluster, error) {
	var clusters []models.Cluster
	if err := s.db.Find(&clusters).Error; err != nil {
		return nil, err
	}
	
	for i := range clusters {
		clusters[i].KubeConfig = ""
	}
	return clusters, nil
}

func (s *Storage) GetCluster(id string) (*models.Cluster, error) {
	var cluster models.Cluster
	if err := s.db.First(&cluster, "id = ?", id).Error; err != nil {
		return nil, err
	}
	if err := s.decryptKubeConfig(&cluster); err != nil {
		return nil, err
	}
	return &cluster, nil
}

func (s *Storage) GetClusterByName(name string) (*models.Cluster, error) {
	var cluster models.Cluster
	if err := s.db.First(&cluster, "name = ?", name).Error; err != nil {
		return nil, err
	}
	if err := s.decryptKubeConfig(&cluster); err != nil {
		return nil, err
	}
	return &cluster, nil
}

func (s *Storage) GetEnabledClusters() ([]models.Cluster, error) {
	var clusters []models.Cluster
	if err := s.db.Where("enabled = ?", true).Find(&clusters).Error; err != nil {
		return nil, err
	}
	
	for i := range clusters {
		clusters[i].KubeConfig = ""
	}
	return clusters, nil
}

func (s *Storage) CreateCluster(cluster *models.Cluster) error {
	cluster.ID = generateID()
	cluster.CreatedAt = now()
	cluster.UpdatedAt = now()
	if err := s.encryptKubeConfig(cluster); err != nil {
		return err
	}
	return s.db.Create(cluster).Error
}

func (s *Storage) UpdateCluster(cluster *models.Cluster) error {
	cluster.UpdatedAt = now()
	if err := s.encryptKubeConfig(cluster); err != nil {
		return err
	}
	return s.db.Save(cluster).Error
}

func (s *Storage) DeleteCluster(id string) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("cluster_id = ?", id).Delete(&models.Resource{}).Error; err != nil {
			return err
		}
		if err := tx.Where("id = ?", id).Delete(&models.Cluster{}).Error; err != nil {
			return err
		}
		return nil
	})
}

func (s *Storage) UpdateClusterStats(id string, stats models.Cluster) error {
	return s.db.Model(&models.Cluster{}).Where("id = ?", id).Updates(map[string]interface{}{
		"node_count":   stats.NodeCount,
		"gpu_count":    stats.GPUCount,
		"total_gpus":   stats.TotalGPUs,
		"used_gpus":    stats.UsedGPUs,
		"version":      stats.Version,
		"status":       stats.Status,
		"last_sync_at": stats.LastSyncAt,
		"sync_error":   stats.SyncError,
	}).Error
}

func (s *Storage) GetQueues() ([]models.Queue, error) {
	var queues []models.Queue
	err := s.db.Find(&queues).Error
	return queues, err
}

var fallbackSeq atomic.Uint64

func generateID() string {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		
		seq := fallbackSeq.Add(1)
		return fmt.Sprintf("%s-%d-%d", now().Format("20060102150405"), os.Getpid(), seq)
	}
	return now().Format("20060102150405") + "-" + hex.EncodeToString(b)[:8]
}