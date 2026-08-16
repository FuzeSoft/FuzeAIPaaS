
package metrics

import (
	"fuze-ai-paas/backend/internal/adapter"
	jobdomain "fuze-ai-paas/backend/internal/domain/job"
	"fuze-ai-paas/backend/internal/models"
	"fuze-ai-paas/backend/internal/ports"
)

type Service struct {
	jobRepo ports.MetricsRepository
}

func NewService(jobRepo ports.MetricsRepository) *Service {
	return &Service{jobRepo: jobRepo}
}

func (svc *Service) Get(clusterID string) (*models.Metrics, error) {
	var modelResources []models.Resource
	var err error
	if clusterID != "" {
		modelResources, err = svc.jobRepo.GetResourcesByCluster(clusterID)
	} else {
		modelResources, err = svc.jobRepo.GetResources()
	}
	if err != nil {
		return nil, err
	}

	jobsModel, err := svc.jobRepo.GetJobs()
	if err != nil {
		return nil, err
	}

	resources := make([]jobdomain.Resource, len(modelResources))
	for i, r := range modelResources {
		resources[i] = adapter.ResourceFromModel(r)
	}
	jobs := make([]jobdomain.Job, 0, len(jobsModel))
	for i := range jobsModel {
		jobs = append(jobs, *adapter.JobFromModel(&jobsModel[i]))
	}

	m := jobdomain.ComputeMetrics(resources, jobs, clusterID)
	return &models.Metrics{
		TotalGPUs:         m.TotalGPUs,
		UsedGPUs:          m.UsedGPUs,
		AvailableGPUs:     m.AvailableGPUs,
		GPUUtilization:    m.GPUUtilization,
		TotalJobs:         m.TotalJobs,
		RunningJobs:       m.RunningJobs,
		PendingJobs:       m.PendingJobs,
		CompletedJobs:     m.CompletedJobs,
		TotalMemory:       m.TotalMemory,
		UsedMemory:        m.UsedMemory,
		MemoryUtilization: m.MemoryUtilization,
	}, nil
}