package storage

import (
	"context"
	"testing"

	"fuze-ai-paas/backend/internal/domain/agent"
	"fuze-ai-paas/backend/internal/ports"
)

func TestToolRepository_CRUD_TenantScoped(t *testing.T) {
	db := testDB(t)
	repo := NewToolRepository(db)
	ctx := context.Background()

	t1 := &agent.Tool{ID: "tool-1", TenantID: "t1", Name: "search", Kind: agent.ToolKindHTTP,
		HTTP: &agent.HTTPToolSpec{URL: "https://api.example.com/s", Method: "POST"}}
	if err := repo.Create(ctx, t1); err != nil {
		t.Fatal(err)
	}
	
	dup := &agent.Tool{TenantID: "t1", Name: "search", Kind: agent.ToolKindHTTP}
	if err := repo.Create(ctx, dup); err != ports.ErrToolConflict {
		t.Fatalf("expected ErrToolConflict, got %v", err)
	}
	
	t2 := &agent.Tool{ID: "tool-2", TenantID: "t2", Name: "search", Kind: agent.ToolKindHTTP}
	if err := repo.Create(ctx, t2); err != nil {
		t.Fatal(err)
	}
	
	got, err := repo.GetByName(ctx, "t1", "search")
	if err != nil || got.TenantID != "t1" {
		t.Fatalf("GetByName t1 failed: %v %+v", err, got)
	}
	if _, err := repo.GetByName(ctx, "t9", "search"); err != ports.ErrToolNotFound {
		t.Fatalf("expected ErrToolNotFound for other tenant, got %v", err)
	}
	
	list, err := repo.List(ctx, "t1")
	if err != nil || len(list) != 1 {
		t.Fatalf("List t1 = %d, want 1 (err=%v)", len(list), err)
	}
	
	got.HTTP.URL = "https://api.example.com/v2"
	if err := repo.Update(ctx, got); err != nil {
		t.Fatal(err)
	}
	got2, _ := repo.GetByName(ctx, "t1", "search")
	if got2.HTTP.URL != "https://api.example.com/v2" {
		t.Fatalf("update not persisted: %q", got2.HTTP.URL)
	}
	
	if err := repo.Delete(ctx, "t1", got.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Get(ctx, "t1", got.ID); err != ports.ErrToolNotFound {
		t.Fatalf("expected not found after delete, got %v", err)
	}
}