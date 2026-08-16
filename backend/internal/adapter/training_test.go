package adapter

import (
	"testing"
	"time"

	"fuze-ai-paas/backend/internal/domain/training"
	"fuze-ai-paas/backend/internal/models"
)

func TestTrainingFromModelNilSafe(t *testing.T) {
	if TrainingFromModel(nil) != nil {
		t.Fatal("nil model must map to nil aggregate")
	}
}

func TestTrainingRoundTrip(t *testing.T) {
	at := time.Now().Truncate(time.Second)
	m := &models.Job{
		ID:        "j1",
		ClusterID: "c1",
		TenantID:  "t1",
		Name:      "train",
		Status:    models.JobStatusRunning,
		Image:     "img",
		Command:   "python train.py",
		Priority:  5,
		GPUs:      2, Memory: 32, GPUMemory: 8192, GPUCores: 50,
		Distributed: true, Framework: training.FrameworkPyTorchDDP, Replicas: 3, MinAvailable: 4,
		DatasetName: "ds", MountPath: "/data",
		MaxRuntime: 120, StartedAt: &at,
		CheckpointEnabled: true, CheckpointInterval: 100, CheckpointMaxRetries: 2,
		LatestCheckpointURI: "s3://b/ck", LatestCheckpointStep: 300, LatestCheckpointAt: &at,
		ResumeFrom: "s3://b/ck", RetryAttempts: 1,
		RegisterModelEnabled: true, RegisterModelID: "m1", RegisterVersionTag: "v2",
		CodeCommit: "abc123", TemplateID: training.TemplatePyTorchDDP,
		FailureReason: "boom",
	}

	agg := TrainingFromModel(m)
	if agg.ID != "j1" || agg.TenantID != "t1" || agg.ClusterID != "c1" || agg.Name != "train" {
		t.Fatalf("identity not carried: %+v", agg)
	}
	if agg.Spec.GPUs != 2 || agg.Spec.Replicas != 3 || agg.Spec.MinAvailable != 4 || !agg.Spec.Distributed {
		t.Fatalf("spec not carried: %+v", agg.Spec)
	}
	if agg.Spec.CodeCommit != "abc123" || agg.Spec.TemplateID != training.TemplatePyTorchDDP {
		t.Fatalf("provenance not carried: %+v", agg.Spec)
	}
	if !agg.Checkpointing.Enabled || agg.Checkpointing.IntervalSteps != 100 || agg.Checkpointing.MaxRetries != 2 {
		t.Fatalf("checkpoint policy not carried: %+v", agg.Checkpointing)
	}
	if agg.LatestCheckpoint == nil || agg.LatestCheckpoint.Step != 300 {
		t.Fatalf("latest checkpoint not carried: %+v", agg.LatestCheckpoint)
	}
	if agg.Attempts != 1 || agg.ResumeFrom != "s3://b/ck" {
		t.Fatalf("resume state not carried: %+v", agg)
	}
	if !agg.Registration.Enabled || agg.Registration.ModelID != "m1" || agg.Registration.VersionTag != "v2" {
		t.Fatalf("registration not carried: %+v", agg.Registration)
	}
	if agg.StartedAt == nil || !agg.StartedAt.Equal(at) {
		t.Fatal("StartedAt not carried")
	}

	out := &models.Job{ID: "j1"}
	agg.MarkCompleted()
	_ = agg.RecordCheckpoint(training.Checkpoint{URI: "s3://b/ck-400", Step: 400, CreatedAt: at})
	TrainingSyncToModel(agg, out)

	if out.Status != models.JobStatusCompleted {
		t.Fatalf("status not synced: %q", out.Status)
	}
	if out.LatestCheckpointURI != "s3://b/ck-400" || out.LatestCheckpointStep != 400 {
		t.Fatalf("checkpoint not synced: %+v", out)
	}
	if out.RetryAttempts != 1 || out.ResumeFrom != "s3://b/ck" {
		t.Fatalf("resume state not synced: %+v", out)
	}
	if out.FailureReason != "boom" {
		t.Fatalf("failure reason not synced: %q", out.FailureReason)
	}
}

func TestTrainingFromModelWithoutCheckpoint(t *testing.T) {
	agg := TrainingFromModel(&models.Job{ID: "j", Name: "n"})
	if agg.LatestCheckpoint != nil {
		t.Fatalf("expected no checkpoint, got %+v", agg.LatestCheckpoint)
	}
}

func TestTrainingSyncToModelNilSafe(t *testing.T) {
	TrainingSyncToModel(nil, nil)
	TrainingSyncToModel(&training.TrainingJob{}, nil)
	TrainingSyncToModel(nil, &models.Job{})
}

func TestTrainingSpecToModel(t *testing.T) {
	agg := &training.TrainingJob{
		ID: "j2", Name: "n", ClusterID: "c9", TenantID: "t9",
		Spec:          training.Spec{Image: "i", Command: "cmd", GPUs: 1, Memory: 8, Distributed: true, Replicas: 2, MaxRuntime: 30},
		Checkpointing: training.CheckpointPolicy{Enabled: true},
		Registration:  training.ModelRegistration{Enabled: true, ModelID: "m", VersionTag: "v"},
	}
	agg.Normalize()

	m := &models.Job{}
	TrainingSpecToModel(agg, m)

	if m.ID != "j2" || m.Name != "n" || m.ClusterID != "c9" || m.TenantID != "t9" {
		t.Fatalf("identity not written: %+v", m)
	}
	if m.Image != "i" || m.GPUs != 1 || m.Memory != 8 || !m.Distributed || m.Replicas != 2 {
		t.Fatalf("spec not written: %+v", m)
	}
	if m.Framework != training.FrameworkPyTorchDDP {
		t.Fatalf("normalized framework not written: %q", m.Framework)
	}
	if !m.CheckpointEnabled || m.CheckpointInterval == 0 || m.CheckpointMaxRetries == 0 {
		t.Fatalf("normalized checkpoint policy not written: %+v", m)
	}
	if m.Type != models.JobTypeTraining {
		t.Fatalf("training job must be typed as training, got %q", m.Type)
	}
}