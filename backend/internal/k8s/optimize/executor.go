package optimize

import (
	"context"
	"encoding/json"
	"fmt"

	"fuze-ai-paas/backend/internal/domain/optimize"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
)

type JobSubmitter interface {
	CreateJob(ctx context.Context, obj *unstructured.Unstructured) error
	DeleteJob(ctx context.Context, name string) error
}

type dynamicJobSubmitter struct {
	client    dynamic.Interface
	namespace string
}

func NewDynamicJobSubmitter(client dynamic.Interface, namespace string) JobSubmitter {
	if namespace == "" {
		namespace = Namespace
	}
	return &dynamicJobSubmitter{client: client, namespace: namespace}
}

func (d *dynamicJobSubmitter) CreateJob(ctx context.Context, obj *unstructured.Unstructured) error {
	_, err := d.client.Resource(jobGVR()).Namespace(d.namespace).Create(ctx, obj, metav1CreateOptions())
	return err
}

func (d *dynamicJobSubmitter) DeleteJob(ctx context.Context, name string) error {
	return d.client.Resource(jobGVR()).Namespace(d.namespace).Delete(ctx, name, metav1DeleteOptions())
}

type K8sCompressionExecutor struct {
	submitter JobSubmitter
	images    map[optimize.CompressionBackend]string
}

func NewExecutor(submitter JobSubmitter, images map[optimize.CompressionBackend]string) *K8sCompressionExecutor {
	return &K8sCompressionExecutor{submitter: submitter, images: images}
}

func (e *K8sCompressionExecutor) Submit(task *optimize.CompressionTask) (string, error) {
	obj, err := BuildCompressionJob(task, e.images)
	if err != nil {
		return "", fmt.Errorf("build compression job: %w", err)
	}
	if err := Snapshot(obj, task, e.images); err != nil {
		return "", fmt.Errorf("compression job rejected by security snapshot: %w", err)
	}
	if err := e.submitter.CreateJob(context.Background(), obj); err != nil {
		return "", fmt.Errorf("submit compression job: %w", err)
	}
	return objectName(task.ID), nil
}

func (e *K8sCompressionExecutor) Cancel(jobID string) error {
	if jobID == "" {
		return fmt.Errorf("cancel: empty job id")
	}
	return e.submitter.DeleteJob(context.Background(), jobID)
}

func (e *K8sCompressionExecutor) GetResult(jobID string) (*optimize.CompressionResult, error) {
	if jobID == "" {
		return nil, fmt.Errorf("get result: empty job id")
	}
	data, err := e.readResult(jobID)
	if err != nil {
		return nil, err
	}
	var res optimize.CompressionResult
	if err := json.Unmarshal(data, &res); err != nil {
		return nil, fmt.Errorf("parse result.json: %w", err)
	}
	res.JobID = jobID
	return &res, nil
}

func (e *K8sCompressionExecutor) readResult(jobID string) ([]byte, error) {
	return nil, fmt.Errorf("result for job %q not ready", jobID)
}