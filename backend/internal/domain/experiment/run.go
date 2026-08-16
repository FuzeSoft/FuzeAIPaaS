package experiment

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	RunStatusPending   = "pending"
	RunStatusRunning   = "running"
	RunStatusCompleted = "completed"
	RunStatusFailed    = "failed"
	RunStatusCancelled = "cancelled"
)

type Run struct {
	ID              string
	ExperimentID    string
	TenantID        string
	Name            string
	Hyperparameters map[string]interface{}
	MetricValue     *float64
	Metrics         map[string]interface{}
	ArtifactURI     string
	SourceJobID     string
	Status          string 
	IsBest          bool
	
	ParentRunID string
	
	ReproductionState string
	StartedAt       *time.Time
	EndedAt         *time.Time
}

func (r *Run) Normalize() {
	r.Name = strings.TrimSpace(r.Name)
	if r.Status == "" {
		r.Status = RunStatusPending
	}
}

func (r *Run) Validate() error {
	if strings.TrimSpace(r.Name) == "" {
		return errors.New("name is required")
	}
	if r.ExperimentID == "" {
		return errors.New("experiment_id is required")
	}
	return nil
}

func (r *Run) IsTerminal() bool {
	switch r.Status {
	case RunStatusCompleted, RunStatusFailed, RunStatusCancelled:
		return true
	default:
		return false
	}
}

func (r *Run) MarkRunning(at time.Time) bool {
	if r.Status != RunStatusPending {
		return false
	}
	r.Status = RunStatusRunning
	t := at
	r.StartedAt = &t
	return true
}

func (r *Run) Complete(value *float64, artifactURI string, at time.Time) bool {
	if r.IsTerminal() {
		return false
	}
	r.Status = RunStatusCompleted
	r.MetricValue = value
	r.ArtifactURI = artifactURI
	t := at
	r.EndedAt = &t
	return true
}

func (r *Run) MarkFailed(at time.Time) bool {
	if r.IsTerminal() {
		return false
	}
	r.Status = RunStatusFailed
	t := at
	r.EndedAt = &t
	return true
}

func (r *Run) MarkCancelled(at time.Time) bool {
	if r.IsTerminal() {
		return false
	}
	r.Status = RunStatusCancelled
	t := at
	r.EndedAt = &t
	return true
}

func NewReproductionRun(parent *Run, jobID, reproName string) *Run {
	hp := map[string]interface{}{}
	for k, v := range parent.Hyperparameters {
		hp[k] = v
	}
		return &Run{
		ID:              fmt.Sprintf("run_%d", time.Now().UnixNano()),
		ExperimentID:    parent.ExperimentID,
		TenantID:        parent.TenantID,
		Name:            reproName,
		Hyperparameters: hp,
		SourceJobID:     jobID,
		ParentRunID:     parent.ID,
		Status:          RunStatusPending,
		ReproductionState: "pending",
	}
}