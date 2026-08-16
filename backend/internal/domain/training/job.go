package training

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"fuze-ai-paas/backend/internal/domain/job"
)

const maxNameLen = 200

type Status = job.JobStatus

const (
	StatusPending   = job.JobStatusPending
	StatusRunning   = job.JobStatusRunning
	StatusRetrying  = job.JobStatusRetrying
	StatusPaused    = job.JobStatusPaused
	StatusCompleted = job.JobStatusCompleted
	StatusFailed    = job.JobStatusFailed
	StatusCancelled = job.JobStatusCancelled
)

type FailureOutcome int

const (
	
	OutcomeRetry FailureOutcome = iota
	
	OutcomeFailed
)

func (o FailureOutcome) String() string {
	if o == OutcomeRetry {
		return "retry"
	}
	return "failed"
}

type TrainingJob struct {
	ID        string
	TenantID  string
	UserID    string
	ClusterID string
	Name      string

	Spec          Spec
	Checkpointing CheckpointPolicy
	Registration  ModelRegistration

	ExperimentID string
	RunID        string

	Status        Status
	Attempts      int
	FailureReason string
	StartedAt     *time.Time
	
	FinishedAt *time.Time

	LatestCheckpoint *Checkpoint
	
	ResumeFrom string
}

func (j *TrainingJob) Normalize() {
	j.Name = strings.TrimSpace(j.Name)
	j.ClusterID = strings.TrimSpace(j.ClusterID)
	if j.ClusterID == "" {
		j.ClusterID = DefaultClusterID
	}
	if j.Status == "" {
		j.Status = StatusPending
	}
	j.Spec.Normalize()
	j.Checkpointing.Normalize()
	j.Registration.Normalize()
}

func (j *TrainingJob) Validate() error {
	if strings.TrimSpace(j.Name) == "" {
		return errors.New("name is required")
	}
	if len(j.Name) > maxNameLen {
		return fmt.Errorf("name exceeds %d characters", maxNameLen)
	}
	if err := j.Spec.Validate(); err != nil {
		return err
	}
	if err := j.Checkpointing.Validate(); err != nil {
		return err
	}
	return j.Registration.Validate()
}

func (j *TrainingJob) IsTerminal() bool { return job.IsTerminal(j.Status) }

func (j *TrainingJob) transition(to Status) bool {
	if !job.CanTransition(j.Status, to) {
		return false
	}
	j.Status = to
	return true
}

func (j *TrainingJob) MarkRunning(at time.Time) bool {
	if !j.transition(StatusRunning) {
		return false
	}
	if j.StartedAt == nil {
		t := at
		j.StartedAt = &t
	}
	return true
}

func (j *TrainingJob) MarkCompleted() bool {
	if !j.transition(StatusCompleted) {
		return false
	}
	return j.markFinished()
}

func (j *TrainingJob) MarkCancelled() bool {
	if !j.transition(StatusCancelled) {
		return false
	}
	return j.markFinished()
}

func (j *TrainingJob) MarkFailed(reason string) bool {
	if !j.transition(StatusFailed) {
		return false
	}
	j.FailureReason = reason
	return j.markFinished()
}

func (j *TrainingJob) markFinished() bool {
	if j.FinishedAt == nil {
		now := time.Now()
		j.FinishedAt = &now
	}
	return true
}

func (j *TrainingJob) MarkRetrying(reason string) bool {
	if !j.transition(StatusRetrying) {
		return false
	}
	j.FailureReason = reason
	return true
}

func (j *TrainingJob) RecordCheckpoint(c Checkpoint) error {
	if !j.Checkpointing.Enabled {
		return errors.New("checkpointing is not enabled for this job")
	}
	if err := c.Validate(); err != nil {
		return err
	}
	
	if j.LatestCheckpoint != nil && c.Step <= j.LatestCheckpoint.Step {
		return fmt.Errorf("stale checkpoint: step %d is not newer than %d", c.Step, j.LatestCheckpoint.Step)
	}
	if c.CreatedAt.IsZero() {
		c.CreatedAt = time.Now()
	}
	j.LatestCheckpoint = &c
	return nil
}

func (j *TrainingJob) HandleFailure(reason string) FailureOutcome {
	if j.canResume() && j.MarkRetrying(reason) {
		j.Attempts++
		j.ResumeFrom = j.LatestCheckpoint.URI
		return OutcomeRetry
	}
	j.MarkFailed(reason)
	return OutcomeFailed
}

func (j *TrainingJob) canResume() bool {
	return j.Checkpointing.Enabled &&
		j.LatestCheckpoint != nil &&
		j.Attempts < j.Checkpointing.MaxRetries
}

func (j *TrainingJob) PrepareRerun() bool {
	if j.Status != StatusRetrying {
		return false
	}
	return j.transition(StatusPending)
}

func (j *TrainingJob) TimedOut(now time.Time) bool {
	if j.Spec.MaxRuntime <= 0 || j.StartedAt == nil {
		return false
	}
	return now.Sub(*j.StartedAt) > time.Duration(j.Spec.MaxRuntime)*time.Minute
}

func (j *TrainingJob) RuntimeHours(now time.Time) float64 {
	if j.StartedAt == nil {
		return 0
	}
	end := now
	if j.FinishedAt != nil {
		end = *j.FinishedAt
	}
	d := end.Sub(*j.StartedAt)
	if d <= 0 {
		return 0
	}
	return d.Hours()
}

func (j *TrainingJob) GPUType() string { return strings.TrimSpace(j.Spec.GPUType) }

func (j *TrainingJob) ShouldRegisterModel() bool {
	return j.Status == StatusCompleted &&
		j.Registration.Enabled &&
		j.LatestCheckpoint != nil
}