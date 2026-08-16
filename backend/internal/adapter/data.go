
package adapter

import (
	datadomain "fuze-ai-paas/backend/internal/domain/data"
	"fuze-ai-paas/backend/internal/models"
)

func StepsToDomain(steps []models.PipelineStep) []datadomain.PipelineStep {
	out := make([]datadomain.PipelineStep, 0, len(steps))
	for i := range steps {
		out = append(out, StepToDomain(&steps[i]))
	}
	return out
}

func StepToDomain(s *models.PipelineStep) datadomain.PipelineStep {
	if s == nil {
		return datadomain.PipelineStep{}
	}
	return datadomain.PipelineStep{
		ID:            s.ID,
		PipelineID:    s.PipelineID,
		Name:          s.Name,
		Kind:          datadomain.StepKind(s.Kind),
		Operator:      s.Operator,
		DependsOn:     s.DependsOn,
		InputPath:     s.InputPath,
		OutputPath:    s.OutputPath,
		Params:        s.Params,
		Image:         s.Image,
		Command:       s.Command,
		GPUs:          s.GPUs,
		Memory:        s.Memory,
		Status:        datadomain.StepStatus(s.Status),
		JobID:         s.JobID,
		FailureReason: s.FailureReason,
	}
}

func StepToModel(d datadomain.PipelineStep) models.PipelineStep {
	return models.PipelineStep{
		ID:            d.ID,
		PipelineID:    d.PipelineID,
		Name:          d.Name,
		Kind:          models.StepKind(d.Kind),
		Operator:      d.Operator,
		DependsOn:     d.DependsOn,
		InputPath:     d.InputPath,
		OutputPath:    d.OutputPath,
		Params:        d.Params,
		Image:         d.Image,
		Command:       d.Command,
		GPUs:          d.GPUs,
		Memory:        d.Memory,
		Status:        models.StepStatus(d.Status),
		JobID:         d.JobID,
		FailureReason: d.FailureReason,
	}
}

func StepsToModel(steps []datadomain.PipelineStep) []models.PipelineStep {
	out := make([]models.PipelineStep, 0, len(steps))
	for i := range steps {
		out = append(out, StepToModel(steps[i]))
	}
	return out
}

func StepKindToJobType(k datadomain.StepKind) models.JobType {
	switch k {
	case datadomain.StepKindClean:
		return models.JobTypeDataClean
	case datadomain.StepKindAugment:
		return models.JobTypeDataAugment
	case datadomain.StepKindETL:
		return models.JobTypeDataETL
	case datadomain.StepKindAnnotation:
		return models.JobTypeDataAnnotation
	default:
		return models.JobTypeDataETL
	}
}