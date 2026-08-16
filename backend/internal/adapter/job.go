
package adapter

import (
	"fuze-ai-paas/backend/internal/domain/job"
	"fuze-ai-paas/backend/internal/models"
)

func JobFromModel(m *models.Job) *job.Job {
	if m == nil {
		return nil
	}
	return &job.Job{
		ID:        m.ID,
		ClusterID: m.ClusterID,
		Name:      m.Name,
		Type:      job.JobType(m.Type),
		Status:    job.JobStatus(m.Status),
		Memory:    m.Memory,
	}
}

func JobSyncToModel(agg *job.Job, m *models.Job) {
	if agg == nil || m == nil {
		return
	}
	m.Status = models.JobStatus(agg.Status)
}

func JobStateFromModel(s models.JobState) job.JobState {
	return job.JobState(s)
}

func JobStatusToModel(s job.JobStatus) models.JobStatus {
	return models.JobStatus(s)
}

func ResourceFromModel(m models.Resource) job.Resource {
	return job.Resource{
		ID:              m.ID,
		ClusterID:       m.ClusterID,
		Name:            m.Name,
		Type:            job.ResourceType(m.Type),
		Vendor:          m.Vendor,
		Model:           m.Model,
		TotalGPUs:       m.TotalGPUs,
		UsedGPUs:        m.UsedGPUs,
		TotalMemory:     m.TotalMemory,
		AvailableMemory: m.AvailableMemory,
		Status:          job.ResourceStatus(m.Status),
		NodeName:        m.NodeName,
	}
}

func ResourceToModel(r job.Resource) models.Resource {
	return models.Resource{
		ID:              r.ID,
		ClusterID:       r.ClusterID,
		Name:            r.Name,
		Type:            models.ResourceType(r.Type),
		Vendor:          r.Vendor,
		Model:           r.Model,
		TotalGPUs:       r.TotalGPUs,
		UsedGPUs:        r.UsedGPUs,
		TotalMemory:     r.TotalMemory,
		AvailableMemory: r.AvailableMemory,
		Status:          models.ResourceStatus(r.Status),
		NodeName:        r.NodeName,
	}
}