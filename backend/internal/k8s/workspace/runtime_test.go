package workspace

import (
	"context"
	"testing"
	"time"

	"fuze-ai-paas/backend/internal/models"
	"fuze-ai-paas/backend/internal/storage"
)

func newTestStorage(t *testing.T) *storage.Storage {
	t.Helper()
	db, err := storage.NewSQLiteDBAt(t.TempDir() + "/ws-rt.db")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	
	if err := db.AutoMigrate(&models.Workspace{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return storage.NewStorage(db)
}

func TestDriverProvisionMockReturnsName(t *testing.T) {
	drv := NewDriver(nil, nil, "") 
	ws := fixture(nil)
	name, err := drv.Provision(context.Background(), ws)
	if err != nil {
		t.Fatalf("provision (mock) failed: %v", err)
	}
	if name != "ws-ws-001" {
		t.Fatalf("runtime name = %q, want ws-ws-001", name)
	}
}

func TestDriverHeartbeatUpdatesLastActive(t *testing.T) {
	s := newTestStorage(t)
	ws := fixture(nil)
	if err := s.CreateWorkspace(ws); err != nil {
		t.Fatalf("create: %v", err)
	}
	drv := NewDriver(nil, s, "")

	first := time.Now().Add(-time.Hour)
	if err := drv.Heartbeat(context.Background(), ws.ID, first); err != nil {
		t.Fatalf("heartbeat 1: %v", err)
	}
	got, _ := s.GetWorkspaceByID(ws.ID)
	if got.LastActiveAt == nil || !got.LastActiveAt.Equal(first) {
		t.Fatalf("last active not persisted: %+v", got.LastActiveAt)
	}

	later := first.Add(10 * time.Minute)
	if err := drv.Heartbeat(context.Background(), ws.ID, later); err != nil {
		t.Fatalf("heartbeat 2: %v", err)
	}
	got, _ = s.GetWorkspaceByID(ws.ID)
	if !got.LastActiveAt.Equal(later) {
		t.Fatalf("last active not advanced: %+v", got.LastActiveAt)
	}

	stale := first
	if err := drv.Heartbeat(context.Background(), ws.ID, stale); err != nil {
		t.Fatalf("heartbeat 3: %v", err)
	}
	got, _ = s.GetWorkspaceByID(ws.ID)
	if !got.LastActiveAt.Equal(later) {
		t.Fatalf("stale heartbeat must not roll back last active: %+v", got.LastActiveAt)
	}
}

func TestDriverURL(t *testing.T) {
	drv := NewDriver(nil, nil, "")
	url, err := drv.URL(fixture(nil))
	if err != nil {
		t.Fatalf("url: %v", err)
	}
	if url != "https://nb.ws-ws-001.fuze.ai" {
		t.Fatalf("url = %q, want https://nb.ws-ws-001.fuze.ai", url)
	}
}

func TestDriverDeprovisionMockNoop(t *testing.T) {
	drv := NewDriver(nil, nil, "")
	if err := drv.Deprovision(context.Background(), fixture(nil)); err != nil {
		t.Fatalf("deprovision (mock) failed: %v", err)
	}
}