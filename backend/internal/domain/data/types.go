
package data

type PipelineStatus string

const (
	PipelineStatusDraft     PipelineStatus = "draft"     
	PipelineStatusPending   PipelineStatus = "pending"   
	PipelineStatusRunning   PipelineStatus = "running"   
	PipelineStatusCompleted PipelineStatus = "completed" 
	PipelineStatusFailed    PipelineStatus = "failed"    
	PipelineStatusCancelled PipelineStatus = "cancelled"
)

func (s PipelineStatus) IsTerminal() bool {
	return s == PipelineStatusCompleted || s == PipelineStatusFailed || s == PipelineStatusCancelled
}

type StepKind string

const (
	StepKindClean      StepKind = "clean"
	StepKindAugment    StepKind = "augment"
	StepKindETL        StepKind = "etl"
	StepKindAnnotation StepKind = "annotation"
)

type StepStatus string

const (
	StepStatusPending   StepStatus = "pending"
	StepStatusRunning   StepStatus = "running"
	StepStatusSucceeded StepStatus = "succeeded"
	StepStatusFailed    StepStatus = "failed"
	StepStatusSkipped   StepStatus = "skipped"
)

func (s StepStatus) IsTerminal() bool {
	return s == StepStatusSucceeded || s == StepStatusFailed || s == StepStatusSkipped
}

type JobStatus string

const (
	JobStatusPending   JobStatus = "pending"
	JobStatusRunning   JobStatus = "running"
	JobStatusPaused    JobStatus = "paused"
	JobStatusRetrying  JobStatus = "retrying"
	JobStatusCompleted JobStatus = "completed"
	JobStatusFailed    JobStatus = "failed"
	JobStatusCancelled JobStatus = "cancelled"
)

type PipelineStep struct {
	ID         string
	PipelineID string
	Name       string
	Kind       StepKind
	
	Operator string
	
	DependsOn string
	
	InputPath  string
	OutputPath string
	
	Params string
	
	Image   string
	Command string
	
	GPUs   int
	Memory int
	
	Status        StepStatus
	JobID         string
	FailureReason string
}