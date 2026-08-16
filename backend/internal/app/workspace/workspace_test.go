package workspace

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"fuze-ai-paas/backend/internal/domain/event"
	"fuze-ai-paas/backend/internal/models"
	"fuze-ai-paas/backend/internal/ports"
)

type fakeWorkspaceRepo struct {
	mu       sync.Mutex
	byID     map[string]*models.Workspace
	byName   map[string]*models.Workspace
	reclaim  []models.Workspace
	lastList []models.Workspace
}

func newFakeWorkspaceRepo() *fakeWorkspaceRepo {
	return &fakeWorkspaceRepo{
		byID:   map[string]*models.Workspace{},
		byName: map[string]*models.Workspace{},
	}
}

func (f *fakeWorkspaceRepo) ListWorkspaces(tenantID string, filter ports.WorkspaceFilter) ([]models.Workspace, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]models.Workspace, 0)
	for _, w := range f.byID {
		if tenantID != "" && w.TenantID != tenantID {
			continue
		}
		if filter.Status != "" && w.Status != filter.Status {
			continue
		}
		if filter.OwnerID != "" && w.OwnerID != filter.OwnerID {
			continue
		}
		out = append(out, *w)
	}
	f.lastList = out
	return out, nil
}

func (f *fakeWorkspaceRepo) GetWorkspaceForTenant(tenantID, id string) (*models.Workspace, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	w, ok := f.byID[id]
	if !ok || (tenantID != "" && w.TenantID != tenantID) {
		return nil, ports.ErrNotFound
	}
	return w, nil
}

func (f *fakeWorkspaceRepo) GetWorkspaceByName(tenantID, name string) (*models.Workspace, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	w, ok := f.byName[name]
	if !ok || (tenantID != "" && w.TenantID != tenantID) {
		return nil, ports.ErrNotFound
	}
	return w, nil
}

func (f *fakeWorkspaceRepo) GetWorkspaceByID(id string) (*models.Workspace, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	w, ok := f.byID[id]
	if !ok {
		return nil, ports.ErrNotFound
	}
	return w, nil
}

func (f *fakeWorkspaceRepo) CreateWorkspace(w *models.Workspace) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.byID[w.ID] = w
	f.byName[w.Name] = w
	return nil
}

func (f *fakeWorkspaceRepo) UpdateWorkspace(w *models.Workspace) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.byID[w.ID] = w
	return nil
}

func (f *fakeWorkspaceRepo) DeleteWorkspace(id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.byID, id)
	return nil
}

func (f *fakeWorkspaceRepo) DeleteWorkspaceAndReleaseQuota(id, _ string, _ int, _ int) error {
	return f.DeleteWorkspace(id)
}

func (f *fakeWorkspaceRepo) ListReclaimable(now time.Time) ([]models.Workspace, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.reclaim, nil
}

func (f *fakeWorkspaceRepo) TouchWorkspace(id string, at time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if w, ok := f.byID[id]; ok {
		
		if w.LastActiveAt != nil && at.Before(*w.LastActiveAt) {
			return nil
		}
		t := at
		w.LastActiveAt = &t
	}
	return nil
}

type fakeQuota struct {
	err error 
}

func (q *fakeQuota) GetQuota(string) (*models.Quota, error) { return &models.Quota{}, nil }
func (q *fakeQuota) ListQuotas() ([]models.Quota, error)    { return nil, nil }
func (q *fakeQuota) UpsertQuota(*models.Quota) error        { return nil }
func (q *fakeQuota) CheckAndReserve(string, int, int, int) error {
	return q.err
}
func (q *fakeQuota) Release(string, int, int, int) error { return nil }
func (q *fakeQuota) AdjustReservation(string, int, int, int, int) error {
	return nil
}

type fakeRuntime struct {
	provisioned   []string
	deprovisioned []string
	urlFor        map[string]string
	failProvision bool
	
	statusByID map[string]runtimeStatus
}

type runtimeStatus struct {
	ready  bool
	found  bool
	failed bool
}

func (r *fakeRuntime) Provision(ctx context.Context, ws *models.Workspace) (string, error) {
	if r.failProvision {
		return "", errors.New("provision boom")
	}
	r.provisioned = append(r.provisioned, ws.ID)
	return "ws-" + ws.ID, nil
}

func (r *fakeRuntime) Deprovision(ctx context.Context, ws *models.Workspace) error {
	r.deprovisioned = append(r.deprovisioned, ws.ID)
	return nil
}

func (r *fakeRuntime) Heartbeat(ctx context.Context, wsID string, at time.Time) error {
	return nil
}

func (r *fakeRuntime) URL(ws *models.Workspace) (string, error) {
	if u, ok := r.urlFor[ws.ID]; ok {
		return u, nil
	}
	return "https://nb." + ws.ID + ".fuze.ai", nil
}

func (r *fakeRuntime) ProxyTarget(ws *models.Workspace) (string, error) {
	if u, ok := r.urlFor[ws.ID]; ok {
		return u, nil
	}
	return "http://localhost:8888/" + ws.ID, nil
}

func newWorkspace(owner string, gpu int) *models.Workspace {
	return &models.Workspace{
		ID:          "ws-" + owner,
		TenantID:    "tenant-A",
		OwnerID:     owner,
		Name:        "nb-" + owner,
		Kind:        models.WorkspaceKindNotebook,
		Image:       "registry.example.com/jupyter:latest",
		Status:      models.WorkspaceStatusPending,
		GPUCount:    gpu,
		IdleTimeout: 30 * time.Minute,
	}
}

func TestCreateQuotaExceededNotPersisted(t *testing.T) {
	repo := newFakeWorkspaceRepo()
	quota := &fakeQuota{err: ports.ErrQuotaExceeded}
	rt := &fakeRuntime{}
	svc := NewService(repo, quota, rt, DefaultImagePolicy())

	ws := newWorkspace("user-1", 0)
	if err := svc.Create(context.Background(), ws); !errors.Is(err, ports.ErrQuotaExceeded) {
		t.Fatalf("want ErrQuotaExceeded, got %v", err)
	}
	if len(repo.byID) != 0 {
		t.Fatalf("quota-exceeded create must not persist, got %d records", len(repo.byID))
	}
}

func TestCreateImageNotWhitelisted(t *testing.T) {
	repo := newFakeWorkspaceRepo()
	quota := &fakeQuota{}
	rt := &fakeRuntime{}
	svc := NewService(repo, quota, rt, ImagePolicy{Allowed: []string{"registry.example.com/jupyter:latest"}})

	ws := newWorkspace("user-1", 0)
	ws.Image = "docker.io/evil/notebook:latest"
	if err := svc.Create(context.Background(), ws); !errors.Is(err, ErrImageNotAllowed) {
		t.Fatalf("want ErrImageNotAllowed, got %v", err)
	}
	if len(repo.byID) != 0 {
		t.Fatalf("disallowed image must not persist, got %d records", len(repo.byID))
	}
}

func TestCreateSuccessProvisionsAsync(t *testing.T) {
	repo := newFakeWorkspaceRepo()
	quota := &fakeQuota{}
	rt := &fakeRuntime{}
	svc := NewService(repo, quota, rt, DefaultImagePolicy())

	ws := newWorkspace("user-1", 1)
	if err := svc.Create(context.Background(), ws); err != nil {
		t.Fatalf("create: %v", err)
	}
	if len(repo.byID) != 1 {
		t.Fatalf("expected 1 persisted record, got %d", len(repo.byID))
	}
	if len(rt.provisioned) != 1 {
		t.Fatalf("expected Provision to be called, got %d", len(rt.provisioned))
	}
}

func TestStartRequiresOwnership(t *testing.T) {
	repo := newFakeWorkspaceRepo()
	quota := &fakeQuota{}
	rt := &fakeRuntime{}
	svc := NewService(repo, quota, rt, DefaultImagePolicy())

	ws := newWorkspace("owner", 0)
	ws.Status = models.WorkspaceStatusStopped
	_ = repo.CreateWorkspace(ws)

	if err := svc.Start(context.Background(), "tenant-A", "stranger", false, ws.ID); !errors.Is(err, ErrForbidden) {
		t.Fatalf("stranger start must be forbidden, got %v", err)
	}
	
	if err := svc.Start(context.Background(), "tenant-A", "owner", false, ws.ID); err != nil {
		t.Fatalf("owner start failed: %v", err)
	}
}

func TestDeleteRunningRejected(t *testing.T) {
	repo := newFakeWorkspaceRepo()
	svc := NewService(repo, &fakeQuota{}, &fakeRuntime{}, DefaultImagePolicy())

	ws := newWorkspace("owner", 0)
	ws.Status = models.WorkspaceStatusRunning
	_ = repo.CreateWorkspace(ws)

	if err := svc.Delete(context.Background(), "tenant-A", "owner", false, ws.ID); !errors.Is(err, ErrIllegalState) {
		t.Fatalf("delete running must be ErrIllegalState, got %v", err)
	}
}

func TestReconcileStartingToRunning(t *testing.T) {
	repo := newFakeWorkspaceRepo()
	rt := &fakeRuntime{}
	svc := NewService(repo, &fakeQuota{}, rt, DefaultImagePolicy())
	svc.statusFn = func(ws *models.Workspace) (ready, found, failed bool) {
		return true, true, false 
	}

	ws := newWorkspace("owner", 0)
	ws.Status = models.WorkspaceStatusStarting
	_ = repo.CreateWorkspace(ws)

	svc.Reconcile(context.Background())
	got, _ := repo.GetWorkspaceByID(ws.ID)
	if got.Status != models.WorkspaceStatusRunning {
		t.Fatalf("starting should reconcile to running, got %s", got.Status)
	}
	if got.StartedAt == nil {
		t.Fatal("running workspace must record StartedAt")
	}
}

func TestReconcileRunningButGoneToFailed(t *testing.T) {
	repo := newFakeWorkspaceRepo()
	svc := NewService(repo, &fakeQuota{}, &fakeRuntime{}, DefaultImagePolicy())
	svc.statusFn = func(ws *models.Workspace) (ready, found, failed bool) {
		return false, false, false 
	}

	ws := newWorkspace("owner", 0)
	ws.Status = models.WorkspaceStatusRunning
	_ = repo.CreateWorkspace(ws)

	svc.Reconcile(context.Background())
	got, _ := repo.GetWorkspaceByID(ws.ID)
	if got.Status != models.WorkspaceStatusFailed {
		t.Fatalf("running-but-gone should reconcile to failed, got %s", got.Status)
	}
}

func TestStopSuccess(t *testing.T) {
	repo := newFakeWorkspaceRepo()
	rt := &fakeRuntime{}
	svc := NewService(repo, &fakeQuota{}, rt, DefaultImagePolicy())

	ws := newWorkspace("owner", 0)
	ws.Status = models.WorkspaceStatusRunning
	_ = repo.CreateWorkspace(ws)

	if err := svc.Stop(context.Background(), "tenant-A", "owner", false, ws.ID); err != nil {
		t.Fatalf("stop: %v", err)
	}
	got, _ := repo.GetWorkspaceByID(ws.ID)
	if got.Status != models.WorkspaceStatusStopped {
		t.Fatalf("status = %s, want stopped", got.Status)
	}
	if got.StoppedAt == nil {
		t.Fatal("stopped workspace must record StoppedAt")
	}
	if len(rt.deprovisioned) != 1 {
		t.Fatalf("Deprovision should be called once, got %d", len(rt.deprovisioned))
	}
}

func TestDeleteSuccess(t *testing.T) {
	repo := newFakeWorkspaceRepo()
	rt := &fakeRuntime{}
	quota := &fakeQuota{}
	svc := NewService(repo, quota, rt, DefaultImagePolicy())

	ws := newWorkspace("owner", 2)
	ws.Status = models.WorkspaceStatusStopped
	_ = repo.CreateWorkspace(ws)

	if err := svc.Delete(context.Background(), "tenant-A", "owner", false, ws.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := repo.GetWorkspaceByID(ws.ID); !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("record should be deleted, got %v", err)
	}
	if len(rt.deprovisioned) != 1 {
		t.Fatalf("Deprovision should be called on delete, got %d", len(rt.deprovisioned))
	}
}

func TestReconcileRunningToFailed(t *testing.T) {
	repo := newFakeWorkspaceRepo()
	svc := NewService(repo, &fakeQuota{}, &fakeRuntime{}, DefaultImagePolicy())
	svc.statusFn = func(ws *models.Workspace) (ready, found, failed bool) {
		return false, true, true 
	}
	ws := newWorkspace("owner", 0)
	ws.Status = models.WorkspaceStatusRunning
	_ = repo.CreateWorkspace(ws)

	svc.Reconcile(context.Background())
	got, _ := repo.GetWorkspaceByID(ws.ID)
	if got.Status != models.WorkspaceStatusFailed {
		t.Fatalf("running+failed should reconcile to failed, got %s", got.Status)
	}
}

func TestReclaimIdleActuallyReclaims(t *testing.T) {
	repo := newFakeWorkspaceRepo()
	rt := &fakeRuntime{}
	svc := NewService(repo, &fakeQuota{}, rt, DefaultImagePolicy())
	svc.activeFn = func(ws *models.Workspace) (bool, error) { return false, nil }

	now := time.Now()
	ws := newWorkspace("owner", 1)
	ws.Status = models.WorkspaceStatusRunning
	ws.IdleTimeout = 10 * time.Minute
	stale := now.Add(-time.Hour)
	ws.LastActiveAt = &stale
	_ = repo.CreateWorkspace(ws)
	repo.reclaim = []models.Workspace{*ws}

	reclaimed, err := svc.ReclaimIdle(context.Background(), now)
	if err != nil {
		t.Fatalf("reclaim: %v", err)
	}
	if len(reclaimed) != 1 || reclaimed[0] != ws.ID {
		t.Fatalf("expected 1 reclaimed (%s), got %+v", ws.ID, reclaimed)
	}
	got, _ := repo.GetWorkspaceByID(ws.ID)
	if got.Status != models.WorkspaceStatusStopping {
		t.Fatalf("reclaimed ws should be stopping, got %s", got.Status)
	}
	if len(rt.deprovisioned) != 1 {
		t.Fatalf("Deprovision should be called on reclaim, got %d", len(rt.deprovisioned))
	}
}

type fakeNotifier struct {
	mu      sync.Mutex
	events  []event.Event
	failErr error
}

func (n *fakeNotifier) Notify(_ context.Context, e event.Event) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.events = append(n.events, e)
	return n.failErr
}

func TestReclaimIdleNotifiesBeforeReclaim(t *testing.T) {
	repo := newFakeWorkspaceRepo()
	rt := &fakeRuntime{}
	sink := &fakeNotifier{}
	svc := NewService(repo, &fakeQuota{}, rt, DefaultImagePolicy(), WithNotifier(sink))
	svc.activeFn = func(ws *models.Workspace) (bool, error) {
		
		return ws.ID == "ws-active", nil
	}

	now := time.Now()
	stale := now.Add(-time.Hour)

	ws := newWorkspace("owner", 1)
	ws.Status = models.WorkspaceStatusRunning
	ws.IdleTimeout = 10 * time.Minute
	ws.LastActiveAt = &stale
	_ = repo.CreateWorkspace(ws)

	active := newWorkspace("user-2", 0)
	active.ID = "ws-active"
	active.Status = models.WorkspaceStatusRunning
	active.IdleTimeout = 10 * time.Minute
	active.LastActiveAt = &stale
	_ = repo.CreateWorkspace(active)

	repo.reclaim = []models.Workspace{*ws, *active}

	if _, err := svc.ReclaimIdle(context.Background(), now); err != nil {
		t.Fatalf("reclaim: %v", err)
	}

	sink.mu.Lock()
	defer sink.mu.Unlock()
	if len(sink.events) != 1 {
		t.Fatalf("expected exactly 1 reclaim notification, got %d", len(sink.events))
	}
	ev, ok := sink.events[0].(event.WorkspaceReclaimed)
	if !ok {
		t.Fatalf("expected WorkspaceReclaimed event, got %T", sink.events[0])
	}
	if ev.WorkspaceID != ws.ID || ev.TenantID != ws.TenantID || ev.OwnerID != ws.OwnerID {
		t.Fatalf("notification payload mismatch: %+v", ev)
	}
	if ev.EventType() != event.WorkspaceReclaimedType {
		t.Fatalf("event type mismatch: %s", ev.EventType())
	}
}

func TestReclaimIdleNoNotifierIsNoop(t *testing.T) {
	repo := newFakeWorkspaceRepo()
	rt := &fakeRuntime{}
	svc := NewService(repo, &fakeQuota{}, rt, DefaultImagePolicy()) 
	svc.activeFn = func(ws *models.Workspace) (bool, error) { return false, nil }

	now := time.Now()
	stale := now.Add(-time.Hour)
	ws := newWorkspace("owner", 1)
	ws.Status = models.WorkspaceStatusRunning
	ws.IdleTimeout = 10 * time.Minute
	ws.LastActiveAt = &stale
	_ = repo.CreateWorkspace(ws)
	repo.reclaim = []models.Workspace{*ws}

	if _, err := svc.ReclaimIdle(context.Background(), now); err != nil {
		t.Fatalf("reclaim without notifier: %v", err)
	}
	if len(rt.deprovisioned) != 1 {
		t.Fatalf("Deprovision must run even without notifier, got %d", len(rt.deprovisioned))
	}
}

func TestParseMemGB(t *testing.T) {
	cases := map[string]int{
		"":      0,
		"4Gi":   4,
		"4G":    4,
		"512Mi": 0,
		"2G ":   2,
		"bad":   0,
	}
	for in, want := range cases {
		if got := parseMemGB(in); got != want {
			t.Errorf("parseMemGB(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestReclaimSkipsActiveSession(t *testing.T) {
	repo := newFakeWorkspaceRepo()
	rt := &fakeRuntime{}
	svc := NewService(repo, &fakeQuota{}, rt, DefaultImagePolicy())
	svc.activeFn = func(ws *models.Workspace) (bool, error) {
		return true, nil 
	}

	now := time.Now()
	ws := newWorkspace("owner", 1)
	ws.Status = models.WorkspaceStatusRunning
	ws.IdleTimeout = 10 * time.Minute
	stale := now.Add(-time.Hour)
	ws.LastActiveAt = &stale
	_ = repo.CreateWorkspace(ws)
	
	repo.reclaim = []models.Workspace{*ws}

	reclaimed, err := svc.ReclaimIdle(context.Background(), now)
	if err != nil {
		t.Fatalf("reclaim: %v", err)
	}
	if len(reclaimed) != 0 {
		t.Fatalf("active session must be skipped, got %d reclaimed", len(reclaimed))
	}
	got, _ := repo.GetWorkspaceByID(ws.ID)
	
	if got.LastActiveAt == nil || !got.LastActiveAt.Equal(now) {
		t.Fatalf("active session must refresh LastActiveAt to now, got %+v", got.LastActiveAt)
	}
}