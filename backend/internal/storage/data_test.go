package storage

import (
	"testing"

	"fuze-ai-paas/backend/internal/models"
)

func newDataStorage(t *testing.T) *Storage {
	t.Helper()
	db, err := NewSQLiteDBAt(t.TempDir() + "/data-test.db")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	return NewStorage(db)
}

func dataPipelineFixture(tenant, name, dataset string) *models.DataPipeline {
	return &models.DataPipeline{
		ID:          "pipe-1",
		TenantID:    tenant,
		Name:        name,
		Description: "fixture pipeline",
		DatasetName: dataset,
		MountPath:   "/mnt/data",
		Status:      models.PipelineStatusDraft,
	}
}

func TestDataPipelineCRUD(t *testing.T) {
	s := newDataStorage(t)

	p := dataPipelineFixture("t1", "clean-etl", "ds-a")
	if err := s.CreatePipeline(p); err != nil {
		t.Fatalf("create pipeline: %v", err)
	}

	got, err := s.GetPipeline("pipe-1")
	if err != nil {
		t.Fatalf("get pipeline: %v", err)
	}
	if got.Name != "clean-etl" || got.DatasetName != "ds-a" {
		t.Fatalf("unexpected pipeline: %+v", got)
	}

	got.Status = models.PipelineStatusRunning
	if err := s.UpdatePipeline(got); err != nil {
		t.Fatalf("update pipeline: %v", err)
	}
	reread, _ := s.GetPipeline("pipe-1")
	if reread.Status != models.PipelineStatusRunning {
		t.Fatalf("status not persisted: %v", reread.Status)
	}

	list, err := s.ListPipelines("t1")
	if err != nil {
		t.Fatalf("list pipelines: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 pipeline, got %d", len(list))
	}
}

func TestPipelineStepCRUDAndRuns(t *testing.T) {
	s := newDataStorage(t)
	if err := s.CreatePipeline(dataPipelineFixture("t1", "p", "ds-a")); err != nil {
		t.Fatalf("create pipeline: %v", err)
	}

	step := &models.PipelineStep{
		ID:         "step-1",
		PipelineID: "pipe-1",
		Name:       "dedup",
		Kind:       models.StepKindClean,
		Operator:   "dedup",
		DependsOn:  "[]",
		InputPath:  "raw",
		OutputPath: "clean",
		Status:     models.StepStatusPending,
	}
	if err := s.CreateStep(step); err != nil {
		t.Fatalf("create step: %v", err)
	}

	steps, err := s.GetSteps("pipe-1")
	if err != nil {
		t.Fatalf("get steps: %v", err)
	}
	if len(steps) != 1 || steps[0].Operator != "dedup" {
		t.Fatalf("unexpected steps: %+v", steps)
	}

	step.Status = models.StepStatusSucceeded
	if err := s.UpdateStep(step); err != nil {
		t.Fatalf("update step: %v", err)
	}
	reread, _ := s.GetSteps("pipe-1")
	if reread[0].Status != models.StepStatusSucceeded {
		t.Fatalf("step status not persisted: %v", reread[0].Status)
	}

	run := &models.PipelineStepRun{
		ID:            "run-1",
		StepID:        "step-1",
		PipelineID:    "pipe-1",
		VolcanoJobName: "vj-1",
		Status:        models.StepStatusRunning,
	}
	if err := s.CreateStepRun(run); err != nil {
		t.Fatalf("create step run: %v", err)
	}
	runs, err := s.GetStepRuns("step-1")
	if err != nil {
		t.Fatalf("get step runs: %v", err)
	}
	if len(runs) != 1 || runs[0].VolcanoJobName != "vj-1" {
		t.Fatalf("unexpected runs: %+v", runs)
	}
}

func TestAnnotationCRUD(t *testing.T) {
	s := newDataStorage(t)
	a := &models.AnnotationTask{
		ID:           "ann-1",
		TenantID:     "t1",
		Name:         "label cats",
		DatasetName:  "ds-a",
		DataGlob:     "images/*.jpg",
		TaskType:     "image-detection",
		Categories:   `["cat","dog"]`,
		Status:       models.AnnotationStatusOpen,
		OutputFormat: "coco",
	}
	if err := s.CreateAnnotation(a); err != nil {
		t.Fatalf("create annotation: %v", err)
	}
	got, err := s.GetAnnotation("ann-1")
	if err != nil {
		t.Fatalf("get annotation: %v", err)
	}
	if got.Categories != `["cat","dog"]` {
		t.Fatalf("unexpected annotation: %+v", got)
	}
	list, err := s.ListAnnotations("t1")
	if err != nil {
		t.Fatalf("list annotations: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 annotation, got %d", len(list))
	}
}

func TestJobTypeToDataQueue(t *testing.T) {
	cases := []models.JobType{
		models.JobTypeDataClean,
		models.JobTypeDataAugment,
		models.JobTypeDataETL,
		models.JobTypeDataAnnotation,
	}
	for _, jt := range cases {
		q, ok := models.JobTypeToQueue[jt]
		if !ok || q != "data-queue" {
			t.Fatalf("job type %s should map to data-queue, got %q (ok=%v)", jt, q, ok)
		}
	}
}