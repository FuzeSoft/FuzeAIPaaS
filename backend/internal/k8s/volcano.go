package k8s

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"fuze-ai-paas/backend/internal/models"
)

type VolcanoJobSpec struct {
	SchedulerName string              `json:"schedulerName,omitempty"`
	MinAvailable  int32               `json:"minAvailable,omitempty"`
	Queue         string              `json:"queue"`
	PriorityClass string              `json:"priorityClassName,omitempty"`
	Tasks         []TaskSpec          `json:"tasks"`
	Policies      []LifecyclePolicy   `json:"policies,omitempty"`
	Plugins       map[string][]string `json:"plugins,omitempty"`
}

type TaskSpec struct {
	Name     string                 `json:"name"`
	Replicas int32                  `json:"replicas"`
	Template corev1.PodTemplateSpec `json:"template"`
	Policies []LifecyclePolicy      `json:"policies,omitempty"`
}

type LifecyclePolicy struct {
	Action   string `json:"action,omitempty"`
	Event    string `json:"event,omitempty"`
	ExitCode int32  `json:"exitCode,omitempty"`
}

type VolcanoJob struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              VolcanoJobSpec   `json:"spec,omitempty"`
	Status            VolcanoJobStatus `json:"status,omitempty"`
}

type VolcanoJobStatus struct {
	State           models.JobState             `json:"state,omitempty"`
	MinAvailable    int32                       `json:"minAvailable,omitempty"`
	TaskStatusCount map[string]models.TaskState `json:"taskStatusCount,omitempty"`
}

type VolcanoJobList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VolcanoJob `json:"items"`
}

func (in *VolcanoJob) DeepCopyInto(out *VolcanoJob) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	out.Status = in.Status
}

func (in *VolcanoJob) DeepCopy() *VolcanoJob {
	if in == nil {
		return nil
	}
	out := new(VolcanoJob)
	in.DeepCopyInto(out)
	return out
}

func (in *VolcanoJob) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

func (in *VolcanoJobSpec) DeepCopyInto(out *VolcanoJobSpec) {
	*out = *in
	if in.Tasks != nil {
		l := make([]TaskSpec, len(in.Tasks))
		for i := range in.Tasks {
			in.Tasks[i].DeepCopyInto(&l[i])
		}
		out.Tasks = l
	}
	if in.Policies != nil {
		l := make([]LifecyclePolicy, len(in.Policies))
		for i := range in.Policies {
			l[i] = in.Policies[i]
		}
		out.Policies = l
	}
	if in.Plugins != nil {
		out.Plugins = make(map[string][]string, len(in.Plugins))
		for k, v := range in.Plugins {
			out.Plugins[k] = append([]string(nil), v...)
		}
	}
}

func (in *TaskSpec) DeepCopyInto(out *TaskSpec) {
	*out = *in
	in.Template.DeepCopyInto(&out.Template)
	if in.Policies != nil {
		l := make([]LifecyclePolicy, len(in.Policies))
		for i := range in.Policies {
			l[i] = in.Policies[i]
		}
		out.Policies = l
	}
}

func (in *VolcanoJobStatus) DeepCopyInto(out *VolcanoJobStatus) {
	*out = *in
	if in.TaskStatusCount != nil {
		l := make(map[string]models.TaskState, len(in.TaskStatusCount))
		for k, v := range in.TaskStatusCount {
			l[k] = v
		}
		out.TaskStatusCount = l
	}
}

func (in *VolcanoJobList) DeepCopyInto(out *VolcanoJobList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		l := make([]VolcanoJob, len(in.Items))
		for i := range in.Items {
			in.Items[i].DeepCopyInto(&l[i])
		}
		out.Items = l
	}
}

func (in *VolcanoJobList) DeepCopy() *VolcanoJobList {
	if in == nil {
		return nil
	}
	out := new(VolcanoJobList)
	in.DeepCopyInto(out)
	return out
}

func (in *VolcanoJobList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}