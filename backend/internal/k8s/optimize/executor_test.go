package optimize

import (
	"context"
	"testing"

	"fuze-ai-paas/backend/internal/domain/optimize"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type fakeSubmitter struct {
	created map[string]*unstructured.Unstructured
	deleted map[string]bool
}

func newFakeSubmitter() *fakeSubmitter {
	return &fakeSubmitter{created: map[string]*unstructured.Unstructured{}, deleted: map[string]bool{}}
}

func (f *fakeSubmitter) CreateJob(_ context.Context, obj *unstructured.Unstructured) error {
	f.created[obj.GetName()] = obj
	return nil
}

func (f *fakeSubmitter) DeleteJob(_ context.Context, name string) error {
	f.deleted[name] = true
	return nil
}

func TestExecutorSubmitAndCancel(t *testing.T) {
	fs := newFakeSubmitter()
	e := NewExecutor(fs, nil)
	task := optimize.NewCompressionTask("T-9", "tenant-1", "q", optimize.CompressionTypeQuantize, optimize.BackendPyTorch, `{"method":"dynamic","bits":8}`, "mv-1")

	jobID, err := e.Submit(task)
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if jobID == "" {
		t.Fatal("expected non-empty job id")
	}
	if _, ok := fs.created[jobID]; !ok {
		t.Fatalf("job %q should have been created via submitter", jobID)
	}
	
	if _, err := e.Submit(task); err != nil {
		t.Fatalf("re-submit same task: %v", err)
	}
	if len(fs.created) != 1 {
		t.Fatalf("expected exactly 1 created job, got %d", len(fs.created))
	}
	if err := e.Cancel(jobID); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if !fs.deleted[jobID] {
		t.Fatal("cancel should delete the job")
	}
}

func TestExecutorSubmitRejectedBySnapshot(t *testing.T) {
	fs := newFakeSubmitter()
	e := NewExecutor(fs, nil)
	task := optimize.NewCompressionTask("T-10", "tenant-1", "q", optimize.CompressionTypeQuantize, optimize.CompressionBackend("bogus"), "{}", "mv-1")
	if _, err := e.Submit(task); err == nil {
		t.Fatal("submit with unlisted backend must be rejected before reaching cluster")
	}
	if len(fs.created) != 0 {
		t.Fatal("no job should reach cluster when snapshot rejects")
	}
}

func TestExecutorGetResultNotReady(t *testing.T) {
	fs := newFakeSubmitter()
	e := NewExecutor(fs, nil)
	if _, err := e.GetResult("opt-x"); err == nil {
		t.Fatal("missing result should error (drives polling/retry in app layer)")
	}
}