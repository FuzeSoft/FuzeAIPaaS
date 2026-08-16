package adapter

import (
	"fuze-ai-paas/backend/internal/domain/training"
	"fuze-ai-paas/backend/internal/models"
)

func TrainingFromModel(m *models.Job) *training.TrainingJob {
	if m == nil {
		return nil
	}
	agg := &training.TrainingJob{
		ID:        m.ID,
		TenantID:  m.TenantID,
		ClusterID: m.ClusterID,
		Name:      m.Name,
		Status:    training.Status(m.Status),
		Attempts:  m.RetryAttempts,

		FailureReason: m.FailureReason,
		StartedAt:     m.StartedAt,
		ResumeFrom:    m.ResumeFrom,

		Spec: training.Spec{
			Image:        m.Image,
			Command:      m.Command,
			Priority:     m.Priority,
			GPUs:         m.GPUs,
			Memory:       m.Memory,
			GPUMemory:    m.GPUMemory,
			GPUCores:     m.GPUCores,
			Distributed:  m.Distributed,
			Framework:    m.Framework,
			Replicas:     m.Replicas,
			MinAvailable: m.MinAvailable,
			DatasetName:  m.DatasetName,
			MountPath:    m.MountPath,
			CodeCommit:   m.CodeCommit,
			TemplateID:   m.TemplateID,
			MaxRuntime:   m.MaxRuntime,
		},
		Checkpointing: training.CheckpointPolicy{
			Enabled:       m.CheckpointEnabled,
			IntervalSteps: m.CheckpointInterval,
			MaxRetries:    m.CheckpointMaxRetries,
		},
		Registration: training.ModelRegistration{
			Enabled:    m.RegisterModelEnabled,
			ModelID:    m.RegisterModelID,
			VersionTag: m.RegisterVersionTag,
		},
	}

	if m.LatestCheckpointURI != "" {
		ck := training.Checkpoint{URI: m.LatestCheckpointURI, Step: m.LatestCheckpointStep}
		if m.LatestCheckpointAt != nil {
			ck.CreatedAt = *m.LatestCheckpointAt
		}
		ck.Hash = m.LatestCheckpointHash
		ck.SizeBytes = m.LatestCheckpointSizeBytes
		agg.LatestCheckpoint = &ck
	}
	return agg
}

func TrainingSyncToModel(agg *training.TrainingJob, m *models.Job) {
	if agg == nil || m == nil {
		return
	}
	m.Status = models.JobStatus(agg.Status)
	m.RetryAttempts = agg.Attempts
	m.ResumeFrom = agg.ResumeFrom
	m.FailureReason = agg.FailureReason
	m.StartedAt = agg.StartedAt

	if agg.LatestCheckpoint != nil {
		m.LatestCheckpointURI = agg.LatestCheckpoint.URI
		m.LatestCheckpointStep = agg.LatestCheckpoint.Step
		at := agg.LatestCheckpoint.CreatedAt
		m.LatestCheckpointAt = &at
		m.LatestCheckpointHash = agg.LatestCheckpoint.Hash
		m.LatestCheckpointSizeBytes = agg.LatestCheckpoint.SizeBytes
	}
}

func TrainingSpecToModel(agg *training.TrainingJob, m *models.Job) {
	if agg == nil || m == nil {
		return
	}
	m.ID = agg.ID
	m.TenantID = agg.TenantID
	m.ClusterID = agg.ClusterID
	m.Name = agg.Name
	m.Type = models.JobTypeTraining
	m.Status = models.JobStatus(agg.Status)

	s := agg.Spec
	m.Image = s.Image
	m.Command = s.Command
	m.Priority = s.Priority
	m.GPUs = s.GPUs
	m.Memory = s.Memory
	m.GPUMemory = s.GPUMemory
	m.GPUCores = s.GPUCores
	m.Distributed = s.Distributed
	m.Framework = s.Framework
	m.Replicas = s.Replicas
	m.MinAvailable = s.MinAvailable
	m.DatasetName = s.DatasetName
	m.MountPath = s.MountPath
	m.CodeCommit = s.CodeCommit
	m.TemplateID = s.TemplateID
	m.MaxRuntime = s.MaxRuntime

	m.CheckpointEnabled = agg.Checkpointing.Enabled
	m.CheckpointInterval = agg.Checkpointing.IntervalSteps
	m.CheckpointMaxRetries = agg.Checkpointing.MaxRetries

	m.RegisterModelEnabled = agg.Registration.Enabled
	m.RegisterModelID = agg.Registration.ModelID
	m.RegisterVersionTag = agg.Registration.VersionTag
}