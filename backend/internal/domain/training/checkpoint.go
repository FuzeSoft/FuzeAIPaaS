package training

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	maxCheckpointRetries   = 20
	defaultCheckpointSteps = 500
	defaultCheckpointRetry = 3
)

type CheckpointPolicy struct {
	Enabled bool `json:"enabled"`
	
	IntervalSteps int `json:"interval_steps"`
	
	MaxRetries int `json:"max_retries"`
}

func (p *CheckpointPolicy) Normalize() {
	if !p.Enabled {
		
		p.IntervalSteps = 0
		p.MaxRetries = 0
		return
	}
	if p.IntervalSteps == 0 {
		p.IntervalSteps = defaultCheckpointSteps
	}
	if p.MaxRetries == 0 {
		p.MaxRetries = defaultCheckpointRetry
	}
}

func (p CheckpointPolicy) Validate() error {
	if !p.Enabled {
		return nil
	}
	if p.IntervalSteps < 0 {
		return errors.New("checkpoint interval_steps must not be negative")
	}
	if p.MaxRetries < 0 || p.MaxRetries > maxCheckpointRetries {
		return fmt.Errorf("checkpoint max_retries must be within [0, %d]", maxCheckpointRetries)
	}
	return nil
}

type Checkpoint struct {
	URI       string    `json:"uri"`
	Step      int       `json:"step"`
	CreatedAt time.Time `json:"created_at"`

	Hash      string `json:"hash"`
	SizeBytes int64  `json:"size_bytes"`
}

func (c Checkpoint) Validate() error {
	if strings.TrimSpace(c.URI) == "" {
		return errors.New("checkpoint uri is required")
	}
	if c.Step < 0 {
		return errors.New("checkpoint step must not be negative")
	}
	if c.SizeBytes < 0 {
		return errors.New("checkpoint size_bytes must not be negative")
	}
	return nil
}