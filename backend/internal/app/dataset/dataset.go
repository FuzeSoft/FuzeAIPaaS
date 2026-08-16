
package dataset

import (
	"context"
	"log"

	"fuze-ai-paas/backend/internal/models"
	"fuze-ai-paas/backend/internal/ports"
)

type Service struct {
	jobRepo    ports.DatasetRepository
	clusterMgr ports.ClusterRegistry
}

func NewService(jobRepo ports.DatasetRepository, clusterMgr ports.ClusterRegistry) *Service {
	return &Service{jobRepo: jobRepo, clusterMgr: clusterMgr}
}

func (svc *Service) Create(ds *models.Dataset) error {
	if client, err := svc.clusterMgr.Get(ds.ClusterID); err == nil && client.Enabled() {
		ctx := context.Background()
		if err := client.CreateDataset(ctx, ds); err != nil {
			ds.Status = models.DatasetStatusFailed
			_ = svc.jobRepo.UpdateDataset(ds)
			return err
		}
		ds.Status = models.DatasetStatusPending
		return svc.jobRepo.UpdateDataset(ds)
	}

	ds.Status = models.DatasetStatusBound
	return svc.jobRepo.UpdateDataset(ds)
}

func (svc *Service) Delete(ds *models.Dataset) error {
	if client, err := svc.clusterMgr.Get(ds.ClusterID); err == nil && client.Enabled() {
		ctx := context.Background()
		if err := client.DeleteDataset(ctx, ds); err != nil {
			log.Printf("[Dataset] Failed to delete Dataset %s: %v", ds.Name, err)
		}
	}
	return svc.jobRepo.DeleteDataset(ds.ID)
}