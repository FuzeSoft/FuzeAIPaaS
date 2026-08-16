package storage

import (
	"testing"
	"time"

	"fuze-ai-paas/backend/internal/models"
	"fuze-ai-paas/backend/internal/ports"
)

func newWorkspaceStorage(t *testing.T) *Storage {
	t.Helper()
	s, err := NewSQLiteDBAt(t.TempDir() + "/ws-test.db")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := s.AutoMigrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return &Storage{db: s}
}

func wsFixture(tenant, name string, status models.WorkspaceStatus) *models.Workspace {
	return &models.Workspace{
		TenantID:  tenant,
		Name:      name,
		Kind:      models.WorkspaceKindNotebook,
		OwnerID:   "user-1",
		Image:     "registry.example.com/jupyter:latest",
		Status:    status,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
}

func TestWorkspaceCRUDWithTenantIsolation(t *testing.T) {
	s := newWorkspaceStorage(t)

	a := wsFixture("tenant-A", "nb-a", models.WorkspaceStatusRunning)
	if err := s.CreateWorkspace(a); err != nil {
		t.Fatalf("create A: %v", err)
	}
	b := wsFixture("tenant-B", "nb-b", models.WorkspaceStatusRunning)
	if err := s.CreateWorkspace(b); err != nil {
		t.Fatalf("create B: %v", err)
	}

	if _, err := s.GetWorkspaceForTenant("tenant-B", a.ID); err != ports.ErrNotFound {
		t.Fatalf("cross-tenant Get should be ErrNotFound, got %v", err)
	}
	
	if got, err := s.GetWorkspaceForTenant("tenant-A", a.ID); err != nil || got.ID != a.ID {
		t.Fatalf("same-tenant Get failed: got=%v err=%v", got, err)
	}

	listA, err := s.ListWorkspaces("tenant-A", ports.WorkspaceFilter{})
	if err != nil {
		t.Fatalf("list A: %v", err)
	}
	if len(listA) != 1 || listA[0].ID != a.ID {
		t.Fatalf("list A should return only A, got %+v", listA)
	}
}

func TestWorkspaceUpdatePersistsState(t *testing.T) {
	s := newWorkspaceStorage(t)
	ws := wsFixture("tenant-A", "nb", models.WorkspaceStatusPending)
	if err := s.CreateWorkspace(ws); err != nil {
		t.Fatalf("create: %v", err)
	}
	ws.Status = models.WorkspaceStatusRunning
	now := time.Now()
	ws.StartedAt = &now
	if err := s.UpdateWorkspace(ws); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, err := s.GetWorkspaceForTenant("tenant-A", ws.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Status != models.WorkspaceStatusRunning || got.StartedAt == nil {
		t.Fatalf("update not persisted: %+v", got)
	}
}

func TestWorkspaceDeleteRemovesRecord(t *testing.T) {
	s := newWorkspaceStorage(t)
	ws := wsFixture("tenant-A", "nb", models.WorkspaceStatusStopped)
	if err := s.CreateWorkspace(ws); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := s.DeleteWorkspace(ws.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := s.GetWorkspaceForTenant("tenant-A", ws.ID); err != ports.ErrNotFound {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestListReclaimableOnlyRunningAndExpired(t *testing.T) {
	s := newWorkspaceStorage(t)
	base := time.Now()

	w1 := wsFixture("tenant-A", "expired", models.WorkspaceStatusRunning)
	w1.IdleTimeout = 10 * time.Minute
	la1 := base.Add(-30 * time.Minute)
	w1.LastActiveAt = &la1
	
	w2 := wsFixture("tenant-A", "fresh", models.WorkspaceStatusRunning)
	w2.IdleTimeout = 1 * time.Hour
	la2 := base.Add(-5 * time.Minute)
	w2.LastActiveAt = &la2
	
	w3 := wsFixture("tenant-B", "stopped", models.WorkspaceStatusStopped)
	w3.IdleTimeout = 1 * time.Minute
	la3 := base.Add(-2 * time.Hour)
	w3.LastActiveAt = &la3
	
	w4 := wsFixture("tenant-B", "never", models.WorkspaceStatusRunning)
	w4.IdleTimeout = 0
	la4 := base.Add(-10 * time.Hour)
	w4.LastActiveAt = &la4

	for _, w := range []*models.Workspace{w1, w2, w3, w4} {
		if err := s.CreateWorkspace(w); err != nil {
			t.Fatalf("create %s: %v", w.Name, err)
		}
	}

	got, err := s.ListReclaimable(base)
	if err != nil {
		t.Fatalf("list reclaimable: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected exactly 1 reclaimable, got %d: %+v", len(got), got)
	}
	if got[0].ID != w1.ID {
		t.Fatalf("wrong reclaimable: want %s got %s", w1.ID, got[0].ID)
	}
}

func TestGetWorkspaceByNameTenantScoped(t *testing.T) {
	s := newWorkspaceStorage(t)
	ws := wsFixture("tenant-A", "shared-name", models.WorkspaceStatusRunning)
	if err := s.CreateWorkspace(ws); err != nil {
		t.Fatalf("create: %v", err)
	}
	got, err := s.GetWorkspaceByName("tenant-A", "shared-name")
	if err != nil || got.ID != ws.ID {
		t.Fatalf("same-tenant by-name failed: got=%v err=%v", got, err)
	}
	if _, err := s.GetWorkspaceByName("tenant-B", "shared-name"); err != ports.ErrNotFound {
		t.Fatalf("cross-tenant by-name should be ErrNotFound, got %v", err)
	}
	if _, err := s.GetWorkspaceByName("tenant-A", "absent"); err != ports.ErrNotFound {
		t.Fatalf("absent by-name should be ErrNotFound, got %v", err)
	}
}

func TestListWorkspacesFilter(t *testing.T) {
	s := newWorkspaceStorage(t)
	running := wsFixture("tenant-A", "r", models.WorkspaceStatusRunning)
	running.OwnerID = "owner-X"
	stopped := wsFixture("tenant-A", "s", models.WorkspaceStatusStopped)
	if err := s.CreateWorkspace(running); err != nil {
		t.Fatalf("create running: %v", err)
	}
	if err := s.CreateWorkspace(stopped); err != nil {
		t.Fatalf("create stopped: %v", err)
	}

	onlyRunning, err := s.ListWorkspaces("tenant-A", ports.WorkspaceFilter{Status: models.WorkspaceStatusRunning})
	if err != nil || len(onlyRunning) != 1 || onlyRunning[0].ID != running.ID {
		t.Fatalf("status filter failed: got=%+v err=%v", onlyRunning, err)
	}
	byOwner, err := s.ListWorkspaces("tenant-A", ports.WorkspaceFilter{OwnerID: "owner-X"})
	if err != nil || len(byOwner) != 1 || byOwner[0].ID != running.ID {
		t.Fatalf("owner filter failed: got=%+v err=%v", byOwner, err)
	}
}

func TestListReclaimableBoundary(t *testing.T) {
	s := newWorkspaceStorage(t)
	base := time.Now()
	ws := wsFixture("tenant-A", "boundary", models.WorkspaceStatusRunning)
	ws.IdleTimeout = 30 * time.Minute
	la := base.Add(-30 * time.Minute) 
	ws.LastActiveAt = &la
	if err := s.CreateWorkspace(ws); err != nil {
		t.Fatalf("create: %v", err)
	}
	got, err := s.ListReclaimable(base)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("boundary must not be reclaimable, got %+v", got)
	}
}