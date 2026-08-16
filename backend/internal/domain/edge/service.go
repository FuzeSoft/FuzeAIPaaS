package edge

import (
	"time"
)

type Publisher interface {
	Publish(e EdgeEvent)
}

type EdgeEvent interface {
	EventTopic() string
}

type DriftDetected struct {
	DeploymentID      string
	NodeID            string
	TenantID          string
	Severity          DriftSeverity
	TriggeredRollback bool
	EvaluatedAt       time.Time
}

func (e DriftDetected) EventTopic() string { return "edge.drift.detected" }

type DeploymentRolledBack struct {
	DeploymentID string
	NodeID       string
	TenantID     string
	FromVersion  string
	ToVersion    string
	Reason       string
	At           time.Time
}

func (e DeploymentRolledBack) EventTopic() string { return "edge.deployment.rolled_back" }