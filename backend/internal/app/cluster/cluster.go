
package cluster

import (
	"context"
	"fmt"
	"time"

	domaincluster "fuze-ai-paas/backend/internal/domain/cluster"
	"fuze-ai-paas/backend/internal/domain/gpu"
	"fuze-ai-paas/backend/internal/events"
	"fuze-ai-paas/backend/internal/k8s/chip"
	"fuze-ai-paas/backend/internal/models"
	"fuze-ai-paas/backend/internal/ports"
)

type Service struct {
	clusterRepo ports.ClusterRepository
	clusterMgr  ports.ClusterRegistry
	bus         *events.Bus
}

func NewService(clusterRepo ports.ClusterRepository, clusterMgr ports.ClusterRegistry, bus *events.Bus) *Service {
	return &Service{clusterRepo: clusterRepo, clusterMgr: clusterMgr, bus: bus}
}

func (svc *Service) List() []string {
	return svc.clusterMgr.List()
}

func (svc *Service) Refresh(ctx context.Context, clusterID string) error {
	client, err := svc.clusterMgr.Get(clusterID)
	if err != nil {
		return err
	}
	
	if !client.Enabled() {
		return fmt.Errorf("cluster %s client unavailable", clusterID)
	}

	cl, err := svc.clusterRepo.GetCluster(clusterID)
	if err != nil {
		return err
	}

	stats := models.Cluster{Status: models.ClusterStatusHealthy, LastSyncAt: time.Now()}
	version, verr := client.ServerVersion(ctx)
	if verr == nil {
		stats.Version = version
	}

	devices, derr := client.DiscoverGPUInventory(ctx)
	if derr != nil {
		stats.Status = models.ClusterStatusUnhealthy
		stats.SyncError = derr.Error()
		_ = svc.clusterRepo.UpdateClusterStats(clusterID, stats)
		return derr
	}

	resources := ExpandNodesToResources(clusterID, devices)
	if uerr := svc.clusterRepo.UpsertClusterResources(clusterID, resources); uerr != nil {
		stats.Status = models.ClusterStatusUnhealthy
		stats.SyncError = uerr.Error()
		_ = svc.clusterRepo.UpdateClusterStats(clusterID, stats)
		return uerr
	}

	agg := domaincluster.New(clusterID, cl.Name)
	agg.Version = version
	discovered := agg.Discover(devices)

	stats.NodeCount = discovered.NodeCount
	stats.TotalGPUs = discovered.TotalGPUs
	stats.UsedGPUs = discovered.UsedGPUs
	stats.GPUCount = discovered.TotalGPUs
	stats.Status = discovered.Status
	stats.Version = discovered.Version
	stats.LastSyncAt = discovered.OccurredAt()
	if err := svc.clusterRepo.UpdateClusterStats(clusterID, stats); err != nil {
		return fmt.Errorf("回写集群 %s 统计失败: %w", clusterID, err)
	}

	if svc.bus != nil {
		svc.bus.Publish(discovered)
	}

	return nil
}

func ExpandNodesToResources(clusterID string, devices []gpu.GPUDevice) []models.Resource {
	var resources []models.Resource
	for _, d := range devices {
		resType := models.ResourceTypeGPU
		if d.IsNPU() {
			resType = models.ResourceTypeNPU
		}

		_, perCardMem := chip.Recognize(d)
		for i := 0; i < d.Total; i++ {
			status := models.ResourceStatusAvailable
			availMem := perCardMem
			if i < d.Used {
				status = models.ResourceStatusAllocated
				availMem = 0
			}
			resources = append(resources, models.Resource{
				Name:            fmt.Sprintf("%s__%s__%d", clusterID, d.NodeName, i),
				Type:            resType,
				Vendor:          d.Vendor,
				Model:           d.Model,
				TotalGPUs:       1,
				UsedGPUs:        0,
				TotalMemory:     perCardMem,
				AvailableMemory: availMem,
				Status:          status,
				NodeName:        d.NodeName,
			})
		}
	}
	return resources
}