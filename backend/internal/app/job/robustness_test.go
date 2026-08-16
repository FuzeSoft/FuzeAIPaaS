package job

import (
	"testing"
)

func TestSubmitJobNilNoPanic(t *testing.T) {
	svc := NewService(nil, nil, nil)
	if err := svc.SubmitJob(nil); err == nil {
		t.Fatal("expect error, not silent success")
	}
}

func TestCancelJobNilNoPanic(t *testing.T) {
	svc := NewService(nil, nil, nil)
	if err := svc.CancelJob(nil); err == nil {
		t.Fatal("expect error, not silent success")
	}
}

func TestTerminateJobNilNoPanic(t *testing.T) {
	svc := NewService(nil, nil, nil)
	if err := svc.TerminateJob(nil); err == nil {
		t.Fatal("expect error, not silent success")
	}
}

func TestMockAllocateNilNoPanic(t *testing.T) {
	svc := NewService(nil, nil, nil)
	if svc.MockAllocate(nil) {
		t.Fatal("expect false, not silent success")
	}
}