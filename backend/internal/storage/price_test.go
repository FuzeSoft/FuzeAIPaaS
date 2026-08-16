package storage

import (
	"context"
	"testing"

	"fuze-ai-paas/backend/internal/domain/llm"
	"fuze-ai-paas/backend/internal/models"
)

func priceTestStore(t *testing.T) *Storage {
	t.Helper()
	s := newTestStorage(t)
	if err := Migrate(s.db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return s
}

func TestGPUPriceUpsertAndFallback(t *testing.T) {
	store := priceTestStore(t)
	repo := NewPriceRepository(store.db)

	ctx := context.Background()
	if err := repo.SaveGPUPrice(ctx, models.GPUPrice{GPUType: "A100", PricePerGPUHour: 2.5, Currency: "CNY"}); err != nil {
		t.Fatalf("SaveGPUPrice: %v", err)
	}
	
	if err := repo.SaveGPUPrice(ctx, models.GPUPrice{GPUType: "", PricePerGPUHour: 1.0, Currency: "CNY"}); err != nil {
		t.Fatalf("SaveGPUPrice default: %v", err)
	}

	got, err := repo.GetGPUPrice(ctx, "A100")
	if err != nil {
		t.Fatalf("GetGPUPrice A100: %v", err)
	}
	if got.PricePerGPUHour != 2.5 {
		t.Fatalf("A100 price = %v, want 2.5", got.PricePerGPUHour)
	}

	fb, err := repo.GetGPUPrice(ctx, "UNKNOWN-GPU")
	if err != nil {
		t.Fatalf("GetGPUPrice fallback: %v", err)
	}
	if fb.PricePerGPUHour != 1.0 {
		t.Fatalf("fallback price = %v, want 1.0", fb.PricePerGPUHour)
	}

	if err := repo.SaveGPUPrice(ctx, models.GPUPrice{GPUType: "A100", PricePerGPUHour: 3.0, Currency: "CNY"}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	got2, _ := repo.GetGPUPrice(ctx, "A100")
	if got2.PricePerGPUHour != 3.0 {
		t.Fatalf("after upsert A100 price = %v, want 3.0", got2.PricePerGPUHour)
	}

	if err := repo.DeleteGPUPrice(ctx, "A100"); err != nil {
		t.Fatalf("DeleteGPUPrice: %v", err)
	}
	fbAfter, err := repo.GetGPUPrice(ctx, "A100")
	if err != nil {
		t.Fatalf("GetGPUPrice after delete should fallback: %v", err)
	}
	if fbAfter.PricePerGPUHour != 1.0 {
		t.Fatalf("after delete, A100 should fallback to default 1.0, got %v", fbAfter.PricePerGPUHour)
	}
	
	if err := repo.DeleteGPUPrice(ctx, ""); err != nil {
		t.Fatalf("DeleteGPUPrice default: %v", err)
	}
	if _, err := repo.GetGPUPrice(ctx, "A100"); err == nil {
		t.Fatal("expected ErrNotFound after default deleted")
	}
}

func TestLLMPriceCRUD(t *testing.T) {
	store := priceTestStore(t)
	repo := NewPriceRepository(store.db)
	ctx := context.Background()

	if err := repo.SaveLLMPrice(ctx, models.LLMPrice{Model: "qwen2-72b", InputPer1K: 0.01, OutputPer1K: 0.03, Currency: "CNY"}); err != nil {
		t.Fatalf("SaveLLMPrice: %v", err)
	}
	got, err := repo.GetLLMPrice(ctx, "qwen2-72b")
	if err != nil {
		t.Fatalf("GetLLMPrice: %v", err)
	}
	if got.OutputPer1K != 0.03 {
		t.Fatalf("output price = %v, want 0.03", got.OutputPer1K)
	}
	if err := repo.DeleteLLMPrice(ctx, "qwen2-72b"); err != nil {
		t.Fatalf("DeleteLLMPrice: %v", err)
	}
	if _, err := repo.GetLLMPrice(ctx, "qwen2-72b"); err == nil {
		t.Fatal("expected ErrNotFound after delete")
	}
}

func TestBuildPriceBookFromDB(t *testing.T) {
	store := priceTestStore(t)
	repo := NewPriceRepository(store.db)
	ctx := context.Background()

	_ = repo.SaveLLMPrice(ctx, models.LLMPrice{Model: "qwen2-72b", InputPer1K: 0.01, OutputPer1K: 0.03, Currency: "CNY"})
	_ = repo.SaveGPUPrice(ctx, models.GPUPrice{GPUType: "A100", PricePerGPUHour: 2.5, Currency: "CNY"})
	_ = repo.SaveGPUPrice(ctx, models.GPUPrice{GPUType: "", PricePerGPUHour: 1.0, Currency: "CNY"})

	fallback := llm.NewPriceBookFromConfig(func(k string) string {
		if k == "LLM_DEFAULT_INPUT_PER_1K" {
			return "0.005"
		}
		return ""
	})

	book, err := BuildPriceBook(ctx, repo, fallback)
	if err != nil {
		t.Fatalf("BuildPriceBook: %v", err)
	}

	p, ok := book.Lookup("qwen2-72b")
	if !ok || p.OutputPer1K != 0.03 {
		t.Fatalf("llm price from db = %+v ok=%v", p, ok)
	}
	
	fb, _ := book.Lookup("unknown-model")
	if fb.InputPer1K != 0.005 {
		t.Fatalf("fallback price = %v, want 0.005", fb.InputPer1K)
	}
	
	if per, ok := book.GPUPerHour("A100"); !ok || per != 2.5 {
		t.Fatalf("gpu price = %v ok=%v, want 2.5", per, ok)
	}
	if per, ok := book.GPUPerHour("UNKNOWN"); !ok || per != 1.0 {
		t.Fatalf("gpu fallback = %v ok=%v, want 1.0", per, ok)
	}
	
	cost := book.GPUCost("A100", 4, 2.0)
	if cost != 2.5*4*2.0 {
		t.Fatalf("gpu cost = %v, want %v", cost, 2.5*4*2.0)
	}
}