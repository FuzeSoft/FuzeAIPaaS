package bootstrap

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"fuze-ai-paas/backend/internal/domain/agent"
	domainevent "fuze-ai-paas/backend/internal/domain/event"
	"fuze-ai-paas/backend/internal/events"
	"fuze-ai-paas/backend/internal/models"
	"fuze-ai-paas/backend/internal/notify"
	"fuze-ai-paas/backend/internal/ports"
	"fuze-ai-paas/backend/internal/quota"
	"fuze-ai-paas/backend/internal/telemetry"
)

func TestSplitList(t *testing.T) {
	if got := splitList(""); got != nil {
		t.Errorf("splitList(\"\") = %v, want nil", got)
	}
	got := splitList("a, b ,c")
	if len(got) != 3 || got[0] != "a" || got[1] != "b" || got[2] != "c" {
		t.Errorf("splitList(\"a, b ,c\") = %v, want [a b c]", got)
	}
}

func TestBudgetAlertSinkNilWhenURLEmpty(t *testing.T) {
	var sink notify.EventSink = budgetAlertSink("")
	if sink != nil {
		t.Fatalf("budgetAlertSink(\"\") = %v, want nil (got a boxed nil interface)", sink)
	}
	nonNil := budgetAlertSink("https://example.com/hook")
	if nonNil == nil {
		t.Fatalf("budgetAlertSink(url) = nil, want non-nil notifier")
	}
}

func TestGetEnv(t *testing.T) {
	t.Setenv("BOOTSTRAP_TEST_KEY", "value")
	if got := getEnv("BOOTSTRAP_TEST_KEY", "def"); got != "value" {
		t.Errorf("getEnv = %q, want value", got)
	}
	if got := getEnv("BOOTSTRAP_TEST_MISSING", "def"); got != "def" {
		t.Errorf("getEnv missing = %q, want def", got)
	}
}

type fakeJobRepo struct {
	jobs    []models.Job
	jobByID map[string]*models.Job
}

func (f *fakeJobRepo) GetJobs() ([]models.Job, error)        { return f.jobs, nil }
func (f *fakeJobRepo) GetPendingJobs() ([]models.Job, error) { return nil, nil }

func (f *fakeJobRepo) GetActiveJobs() ([]models.Job, error) {
	var active []models.Job
	for _, j := range f.jobs {
		if !j.IsTerminal() {
			active = append(active, j)
		}
	}
	return active, nil
}

func (f *fakeJobRepo) GetResourcesByCluster(string) ([]models.Resource, error) { return nil, nil }
func (f *fakeJobRepo) CreateJob(*models.Job) error                             { return nil }
func (f *fakeJobRepo) GetJob(id string) (*models.Job, error)                   { return f.jobByID[id], nil }
func (f *fakeJobRepo) GetJobsByTenant(string) ([]models.Job, error)            { return f.jobs, nil }
func (f *fakeJobRepo) UpdateJobSpec(*models.Job) error                         { return nil }
func (f *fakeJobRepo) DeleteJob(string) error                                  { return nil }
func (f *fakeJobRepo) UpdateJob(*models.Job) error                             { return nil }
func (f *fakeJobRepo) UpdateJobStatus(*models.Job) error                       { return nil }
func (f *fakeJobRepo) UpdateResource(*models.Resource) error                   { return nil }

type fakeQuotaRepo struct {
	mu     sync.Mutex
	quotas []models.Quota
	upsert []models.Quota
}

func (f *fakeQuotaRepo) GetQuota(string) (*models.Quota, error) { return nil, nil }
func (f *fakeQuotaRepo) ListQuotas() ([]models.Quota, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]models.Quota(nil), f.quotas...), nil
}
func (f *fakeQuotaRepo) UpsertQuota(q *models.Quota) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.upsert = append(f.upsert, *q)
	return nil
}
func (f *fakeQuotaRepo) CheckAndReserve(string, int, int, int) error { return nil }
func (f *fakeQuotaRepo) Release(string, int, int, int) error         { return nil }
func (f *fakeQuotaRepo) AdjustReservation(string, int, int, int, int) error {
	return nil
}

func waitFor(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("condition not met within %s", timeout)
}

func TestSubscribeDownstreamWiring(t *testing.T) {
	var mu sync.Mutex
	var received []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		received = append(received, r.Header.Get("X-Ignore"))
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	bus := events.NewBus(16, 4)
	tele := telemetry.NewCollector()
	qr := quota.NewReconciler(&fakeJobRepo{jobs: []models.Job{
		{TenantID: "t1", Status: models.JobStatusRunning, GPUs: 2, Memory: 4},
	}}, &fakeQuotaRepo{quotas: []models.Quota{{TenantID: "t1"}}})
	notifier := notify.NewWebhookNotifier(srv.URL)

	subscribeDownstream(bus, tele, qr, notifier, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	bus.Run(ctx)

	bus.Publish(domainevent.NewClusterDiscovered("c1", "n", domainevent.ClusterStats{TotalGPUs: 8}))
	bus.Publish(domainevent.NewJobSubmitted("j1", "c1", "train", 4, "t1"))
	bus.Publish(domainevent.NewAssignmentCompleted("j1", "c1", 4, 8192))

	waitFor(t, 2*time.Second, func() bool {
		s := tele.Snapshot()
		return s.ClustersDiscovered == 1 && s.JobsSubmitted == 1 && s.AssignmentsDone == 1
	})

	waitFor(t, 2*time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(received) == 3
	})
}

func TestSubscribeDownstreamSkipsNilNotifier(t *testing.T) {
	bus := events.NewBus(16, 4)
	tele := telemetry.NewCollector()
	qr := quota.NewReconciler(&fakeJobRepo{}, &fakeQuotaRepo{})

	subscribeDownstream(bus, tele, qr, nil, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	bus.Run(ctx)

	bus.Publish(domainevent.NewClusterDiscovered("c1", "n", domainevent.ClusterStats{TotalGPUs: 2}))

	waitFor(t, 2*time.Second, func() bool {
		return tele.Snapshot().ClustersDiscovered == 1
	})
}

var (
	_ ports.JobRepository   = (*fakeJobRepo)(nil)
	_ ports.QuotaRepository = (*fakeQuotaRepo)(nil)
)

type fakeAuditRepo struct {
	mu   sync.Mutex
	logs []models.AuditLog
}

func (f *fakeAuditRepo) Record(entry *models.AuditLog) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.logs = append(f.logs, *entry)
	return nil
}

func (f *fakeAuditRepo) ListAudit(q ports.AuditQuery) ([]models.AuditLog, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]models.AuditLog{}, f.logs...), nil
}

func (f *fakeAuditRepo) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.logs)
}

func TestAgentRunFinishedAuditSubscription(t *testing.T) {
	bus := events.NewBus(16, 4)
	audit := &fakeAuditRepo{}
	subscribeDownstream(bus, telemetry.NewCollector(), quota.NewReconciler(&fakeJobRepo{}, &fakeQuotaRepo{}), nil, audit)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	bus.Run(ctx)

	bus.Publish(domainevent.NewAgentRunFinished("t1", "ag1", "run-1", agent.RunSucceeded, "u1", "alice",
		3, 120, "answer:done", ""))

	waitFor(t, 2*time.Second, func() bool { return audit.count() == 1 })
	rec := audit.logs[0]
	if rec.Action != models.ActionRunFinish || rec.ResourceType != models.ResAgent || rec.TenantID != "t1" || rec.ActorID != "u1" {
		t.Fatalf("audit record wrong: %+v", rec)
	}
	if !containsSubstr(rec.Detail, "status=succeeded") || !containsSubstr(rec.Detail, "answer:done") {
		t.Fatalf("audit detail wrong: %q", rec.Detail)
	}
}

func containsSubstr(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}