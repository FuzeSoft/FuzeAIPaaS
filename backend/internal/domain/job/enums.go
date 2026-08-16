package job

type ResourceType string

const (
	ResourceTypeGPU ResourceType = "GPU"
	ResourceTypeNPU ResourceType = "NPU"
	ResourceTypeCPU ResourceType = "CPU"
)

type ResourceStatus string

const (
	ResourceStatusAvailable   ResourceStatus = "available"
	ResourceStatusAllocated   ResourceStatus = "allocated"
	ResourceStatusMaintenance ResourceStatus = "maintenance"
	ResourceStatusError       ResourceStatus = "error"
)

type JobType string

const (
	JobTypeTraining  JobType = "training"
	JobTypeInference JobType = "inference"
	JobTypeBatch     JobType = "batch"
)

type JobStatus string

const (
	JobStatusPending JobStatus = "pending"
	JobStatusRunning JobStatus = "running"
	JobStatusPaused  JobStatus = "paused"
	
	JobStatusRetrying  JobStatus = "retrying"
	JobStatusCompleted JobStatus = "completed"
	JobStatusFailed    JobStatus = "failed"
	JobStatusCancelled JobStatus = "cancelled"
)

type JobState string

const (
	JobStatePending     JobState = "Pending"
	JobStateRunning     JobState = "Running"
	JobStateCompleted   JobState = "Completed"
	JobStateFailed      JobState = "Failed"
	JobStateTerminated  JobState = "Terminated"
	JobStateAborted     JobState = "Aborted"
	JobStateTerminating JobState = "Terminating"
	JobStateAborting    JobState = "Aborting"
	JobStateRestarting  JobState = "Restarting"
	JobStateCompleting  JobState = "Completing"
)