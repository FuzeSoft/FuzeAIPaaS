package storage

import (
	"log"
	"os"

	"fuze-ai-paas/backend/internal/models"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

func NewSQLiteDB() (*gorm.DB, error) {
	return NewSQLiteDBAt(defaultSQLitePath)
}

func NewSQLiteDBAt(path string) (*gorm.DB, error) {
	return OpenDB(DBConfig{Driver: DriverSQLite, DSN: path})
}

func seedData(db *gorm.DB) {
	var count int64
	db.Model(&models.Resource{}).Count(&count)
	if count == 0 {
		resources := []models.Resource{
			{
				ID:              "res-001",
				ClusterID:       "cluster-001",
				Name:            "gpu-node-a100-01",
				Type:            models.ResourceTypeGPU,
				Vendor:          "NVIDIA",
				Model:           "A100 80GB",
				TotalGPUs:       8,
				UsedGPUs:        0,
				TotalMemory:     80,
				AvailableMemory: 80,
				Status:          models.ResourceStatusAvailable,
				NodeName:        "node-01",
				CreatedAt:       now(),
				UpdatedAt:       now(),
			},
			{
				ID:              "res-002",
				ClusterID:       "cluster-001",
				Name:            "gpu-node-a100-02",
				Type:            models.ResourceTypeGPU,
				Vendor:          "NVIDIA",
				Model:           "A100 80GB",
				TotalGPUs:       8,
				UsedGPUs:        2,
				TotalMemory:     80,
				AvailableMemory: 60,
				Status:          models.ResourceStatusAvailable,
				NodeName:        "node-02",
				CreatedAt:       now(),
				UpdatedAt:       now(),
			},
			{
				ID:              "res-003",
				ClusterID:       "cluster-001",
				Name:            "gpu-node-h100-01",
				Type:            models.ResourceTypeGPU,
				Vendor:          "NVIDIA",
				Model:           "H100 80GB",
				TotalGPUs:       8,
				UsedGPUs:        0,
				TotalMemory:     80,
				AvailableMemory: 80,
				Status:          models.ResourceStatusAvailable,
				NodeName:        "node-03",
				CreatedAt:       now(),
				UpdatedAt:       now(),
			},
			{
				ID:              "res-004",
				ClusterID:       "cluster-001",
				Name:            "npu-ascend-910-01",
				Type:            models.ResourceTypeNPU,
				Vendor:          "华为",
				Model:           "Ascend 910",
				TotalGPUs:       8,
				UsedGPUs:        0,
				TotalMemory:     32,
				AvailableMemory: 32,
				Status:          models.ResourceStatusAvailable,
				NodeName:        "node-04",
				CreatedAt:       now(),
				UpdatedAt:       now(),
			},
		}
		db.Create(&resources)
	}

	db.Model(&models.Job{}).Count(&count)
	if count == 0 {
		jobs := []models.Job{
			{
				ID:        "job-001",
				Name:      "LLaMA-2-70B 训练任务",
				Type:      models.JobTypeTraining,
				Status:    models.JobStatusRunning,
				Priority:  100,
				Image:     "nvcr.io/nvidia/pytorch:23.10-py3",
				Command:   "python train.py --epochs 100",
				GPUs:      8,
				Memory:    640,
				CreatedAt: now(),
				UpdatedAt: now(),
			},
			{
				ID:        "job-002",
				Name:      "ChatGLM 推理服务",
				Type:      models.JobTypeInference,
				Status:    models.JobStatusRunning,
				Priority:  200,
				Image:     "nvcr.io/nvidia/tritonserver:23.10-py3",
				Command:   "tritonserver --model-repository=/models",
				GPUs:      2,
				Memory:    160,
				CreatedAt: now(),
				UpdatedAt: now(),
			},
		}
		db.Create(&jobs)
	}

	db.Model(&models.Cluster{}).Count(&count)
	if count == 0 {
		clusters := []models.Cluster{
			{
				ID:          "cluster-001",
				Name:        "生产集群",
				Description: "主要生产环境",
				Region:      "cn-hangzhou",
				Provider:    "self-hosted",
				Namespace:   "fuze-ai-paas",
				Enabled:     true,
				NodeCount:   32,
				GPUCount:    128,
				TotalGPUs:   128,
				UsedGPUs:    2,
				Status:      models.ClusterStatusHealthy,
				CreatedAt:   now(),
				UpdatedAt:   now(),
			},
		}
		db.Create(&clusters)
	}

	db.Model(&models.Queue{}).Count(&count)
	if count == 0 {
		queues := []models.Queue{
			{
				ID:          "queue-001",
				Name:        "在线推理",
				Description: "高优先级在线推理任务",
				Priority:    300,
				CreatedAt:   now(),
				UpdatedAt:   now(),
			},
			{
				ID:          "queue-002",
				Name:        "离线训练",
				Description: "离线训练任务",
				Priority:    100,
				CreatedAt:   now(),
				UpdatedAt:   now(),
			},
		}
		db.Create(&queues)
	}

	db.Model(&models.InferenceService{}).Count(&count)
	if count == 0 {
		svcs := []models.InferenceService{
			{
				ID:            "isvc-001",
				Name:          "llama2-7b-chat",
				Framework:     models.FrameworkPyTorch,
				StorageURI:    "pvc://model-store/llama2-7b",
				MinReplicas:   1,
				MaxReplicas:   3,
				CPU:           "4",
				Memory:        "16Gi",
				GPUs:          1,
				GPUMemory:     40000,
				GPUCores:      100,
				Status:        models.InferenceStatusReady,
				URL:           "http://llama2-7b-chat.fuze-ai-paas.example.com/v1/models/llama2-7b:predict",
				KServeName:    "llama2-7b-chat",
				ReadyReplicas: 1,
				CreatedAt:     now(),
				UpdatedAt:     now(),
			},
			{
				ID:            "isvc-002",
				Name:          "resnet50-classifier",
				Framework:     models.FrameworkTriton,
				StorageURI:    "s3://models/resnet50",
				MinReplicas:   0,
				MaxReplicas:   5,
				CPU:           "2",
				Memory:        "8Gi",
				GPUs:          1,
				GPUMemory:     10000,
				GPUCores:      30,
				Status:        models.InferenceStatusReady,
				URL:           "http://resnet50-classifier.fuze-ai-paas.example.com/v2/models/resnet50/infer",
				KServeName:    "resnet50-classifier",
				ReadyReplicas: 1,
				CreatedAt:     now(),
				UpdatedAt:     now(),
			},
		}
		db.Create(&svcs)
	}

	db.Model(&models.Dataset{}).Count(&count)
	if count == 0 {
		datasets := []models.Dataset{
			{
				ID:             "ds-001",
				Name:           "imagenet-1k",
				MountPoint:     "oss://ai-datasets/imagenet-1k",
				Runtime:        models.RuntimeAlluxio,
				Replicas:       3,
				CacheCapacity:  "200Gi",
				CacheMedium:    models.CacheMediumSSD,
				AccessMode:     "ReadOnly",
				Status:         models.DatasetStatusBound,
				CachedPercent:  87.5,
				UFSTotal:       "150.2GiB",
				CachedCapacity: "131.4GiB",
				CreatedAt:      now(),
				UpdatedAt:      now(),
			},
			{
				ID:             "ds-002",
				Name:           "llm-corpus",
				MountPoint:     "s3://ai-datasets/llm-corpus",
				Runtime:        models.RuntimeJuiceFS,
				Replicas:       2,
				CacheCapacity:  "500Gi",
				CacheMedium:    models.CacheMediumMEM,
				AccessMode:     "ReadOnly",
				Status:         models.DatasetStatusBound,
				CachedPercent:  42.0,
				UFSTotal:       "2.1TiB",
				CachedCapacity: "882GiB",
				CreatedAt:      now(),
				UpdatedAt:      now(),
			},
		}
		db.Create(&datasets)
	}

	seedEnterprise(db)
}

func seedEnterprise(db *gorm.DB) {
	err := db.Transaction(func(tx *gorm.DB) error {
		
		var tenant models.Tenant
		if err := tx.Where(models.Tenant{ID: "default"}).
			FirstOrCreate(&tenant, models.Tenant{
				ID: "default", Name: "默认租户", Description: "平台默认租户",
			}).Error; err != nil {
			return err
		}

		var quota models.Quota
		if err := tx.Where(models.Quota{ID: "default"}).
			FirstOrCreate(&quota, models.Quota{
				ID: "default", TenantID: "default",
				GPUQuota: 64, MemoryQuotaGB: 1024, JobQuota: 10,
			}).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		log.Printf("[seed] create default tenant/quota failed: %v", err)
		return
	}

	err = db.Transaction(func(tx *gorm.DB) error {
		var uc int64
		if err := tx.Model(&models.User{}).Where("id = ?", "u-admin").Count(&uc).Error; err != nil {
			return err
		}
		if uc > 0 {
			return nil
		}

		defaultPassword := "admin123"
		envPwd := os.Getenv("ADMIN_PASSWORD")
		if envPwd != "" {
			
			defaultPassword = envPwd
			log.Println("[seed] Using ADMIN_PASSWORD from environment.")
		} else {
			
			log.Println("[seed] WARNING: creating default admin with well-known password 'admin123'.")
			log.Println("[seed] This is for LOCAL DEV/TEST ONLY. For any networked deployment set ADMIN_PASSWORD before first startup and change it immediately.")
		}
		
		hash, err := bcrypt.GenerateFromPassword([]byte(defaultPassword), 12)
		if err != nil {
			return err
		}
		return tx.Create(&models.User{
			ID:          "u-admin",
			Username:    "admin",
			DisplayName: "平台管理员",
			Password:    string(hash),
			Role:        models.RolePlatformAdmin,
			TenantID:    "default",
			Email:       "admin@fuze.local",
			SSOProvider: "local",
			Enabled:     true,
		}).Error
	})
	if err != nil {
		log.Printf("[seed] create default admin failed: %v", err)
	}
}