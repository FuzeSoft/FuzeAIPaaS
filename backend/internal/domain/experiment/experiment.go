
package experiment

import (
	"errors"
	"strings"
	"time"
)

const (
	
	ObjectiveMinimize = "minimize"
	
	ObjectiveMaximize = "maximize"
)

const (
	
	StatusActive = "active"
	
	StatusArchived = "archived"
)

const maxNameLen = 200

var (
	ErrInvalidSpec  = errors.New("invalid experiment spec")
	ErrInvalidState = errors.New("invalid experiment state")
)

type Experiment struct {
	ID          string
	TenantID    string
	Name        string
	Description string
	Objective   string 
	MetricName  string 
	Status      string 
	Tags        []string
	BestRunID   string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (e *Experiment) Normalize() {
	e.Name = strings.TrimSpace(e.Name)
	e.Objective = strings.ToLower(strings.TrimSpace(e.Objective))
	e.Status = strings.ToLower(strings.TrimSpace(e.Status))
	if e.Status == "" {
		e.Status = StatusActive
	}
	if e.Objective == "" {
		e.Objective = ObjectiveMaximize
	}
}

func (e *Experiment) Validate() error {
	if strings.TrimSpace(e.Name) == "" {
		return errors.New("name is required")
	}
	if len(e.Name) > maxNameLen {
		return errors.New("name exceeds length limit")
	}
	if e.Objective != ObjectiveMinimize && e.Objective != ObjectiveMaximize {
		return errors.New("objective must be minimize or maximize")
	}
	if e.MetricName == "" {
		return errors.New("metric_name is required to rank runs")
	}
	if e.Status != StatusActive && e.Status != StatusArchived {
		return errors.New("status must be active or archived")
	}
	return nil
}

func (e *Experiment) Archive() error {
	if e.Status == StatusArchived {
		return nil
	}
	e.Status = StatusArchived
	return nil
}

func (e *Experiment) IsBetter(candidate, current *Run) bool {
	if candidate == nil || candidate.MetricValue == nil {
		return false
	}
	if current == nil || current.MetricValue == nil {
		return true
	}
	if e.Objective == ObjectiveMinimize {
		return *candidate.MetricValue < *current.MetricValue
	}
	return *candidate.MetricValue > *current.MetricValue
}

func (e *Experiment) UpdateBest(candidate, currentBest *Run) bool {
	if e.IsBetter(candidate, currentBest) {
		e.BestRunID = candidate.ID
		return true
	}
	return false
}