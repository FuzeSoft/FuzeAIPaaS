package adapter

import (
	"testing"

	datadomain "fuze-ai-paas/backend/internal/domain/data"
	"fuze-ai-paas/backend/internal/models"
)

func TestStepKindToJobType(t *testing.T) {
	cases := map[datadomain.StepKind]models.JobType{
		datadomain.StepKindClean:      models.JobTypeDataClean,
		datadomain.StepKindAugment:    models.JobTypeDataAugment,
		datadomain.StepKindETL:        models.JobTypeDataETL,
		datadomain.StepKindAnnotation: models.JobTypeDataAnnotation,
	}
	for k, want := range cases {
		if got := StepKindToJobType(k); got != want {
			t.Fatalf("StepKind %s -> JobType want %s got %s", k, want, got)
		}
	}
	
	if got := StepKindToJobType(datadomain.StepKind("unknown")); got != models.JobTypeDataETL {
		t.Fatalf("unknown StepKind fallback want %s got %s", models.JobTypeDataETL, got)
	}
}

func TestStepRoundTrip(t *testing.T) {
	m := models.PipelineStep{
		ID: "s1", PipelineID: "p1", Name: "clean", Kind: models.StepKindClean,
		Operator: "dedup", DependsOn: `["s0"]`, Params: `{"method":"exact"}`,
		GPUs: 1, Memory: 4, Status: models.StepStatusSucceeded,
	}
	d := StepToDomain(&m)
	if d.Kind != datadomain.StepKindClean || d.Status != datadomain.StepStatusSucceeded {
		t.Fatalf("domain mapping mismatch: %+v", d)
	}
	back := StepToModel(d)
	if back.Kind != m.Kind || back.Operator != m.Operator || back.DependsOn != m.DependsOn {
		t.Fatalf("model round-trip mismatch: %+v vs %+v", back, m)
	}
}