package models

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

type TaskState struct {
	Pending   int32 `json:"pending,omitempty"`
	Running   int32 `json:"running,omitempty"`
	Succeeded int32 `json:"succeeded,omitempty"`
	Failed    int32 `json:"failed,omitempty"`
	Unknown   int32 `json:"unknown,omitempty"`
}

var JobTypeToQueue = map[JobType]string{
	JobTypeInference: "inference-queue",
	JobTypeTraining:  "training-queue",
	JobTypeBatch:     "batch-queue",
	
	JobTypeDataClean:      "data-queue",
	JobTypeDataAugment:    "data-queue",
	JobTypeDataETL:        "data-queue",
	JobTypeDataAnnotation: "data-queue",
}