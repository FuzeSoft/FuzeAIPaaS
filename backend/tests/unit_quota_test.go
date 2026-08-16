package tests

import (
	"testing"

	"fuze-ai-paas/backend/internal/models"
)

func TestQuotaCheckAndReserve(t *testing.T) {
	env := NewTestEnv(t)

	t.Run("reserve quota within limits", func(t *testing.T) {
		err := env.Store.UpsertQuota(&models.Quota{
			ID:            "test-tenant",
			TenantID:      "test-tenant",
			GPUQuota:      10,
			MemoryQuotaGB: 128,
			JobQuota:      5,
		})
		if err != nil {
			t.Fatal(err)
		}

		err = env.Store.CheckAndReserve("test-tenant", 4, 32, 1)
		if err != nil {
			t.Errorf("expected successful reservation, got: %v", err)
		}
	})

	t.Run("reserve quota exceeding GPU limit fails", func(t *testing.T) {
		err := env.Store.CheckAndReserve("test-tenant", 100, 32, 1)
		if err == nil {
			t.Error("expected error for exceeding GPU quota")
		}
	})

	t.Run("reserve quota exceeding job limit fails", func(t *testing.T) {
		
		_ = env.Store.CheckAndReserve("test-tenant", 1, 8, 1)
		_ = env.Store.CheckAndReserve("test-tenant", 1, 8, 1)
		_ = env.Store.CheckAndReserve("test-tenant", 1, 8, 1)
		_ = env.Store.CheckAndReserve("test-tenant", 1, 8, 1)
		_ = env.Store.CheckAndReserve("test-tenant", 1, 8, 1)

		err := env.Store.CheckAndReserve("test-tenant", 1, 8, 1)
		if err == nil {
			t.Error("expected error for exceeding job quota")
		}
	})
}

func TestQuotaRelease(t *testing.T) {
	env := NewTestEnv(t)

	t.Run("release quota after reservation", func(t *testing.T) {
		err := env.Store.UpsertQuota(&models.Quota{
			ID:            "release-tenant",
			TenantID:      "release-tenant",
			GPUQuota:      10,
			MemoryQuotaGB: 128,
			JobQuota:      5,
		})
		if err != nil {
			t.Fatal(err)
		}

		err = env.Store.CheckAndReserve("release-tenant", 4, 32, 1)
		if err != nil {
			t.Fatal(err)
		}

		err = env.Store.Release("release-tenant", 4, 32, 1)
		if err != nil {
			t.Errorf("expected successful release, got: %v", err)
		}

		err = env.Store.CheckAndReserve("release-tenant", 4, 32, 1)
		if err != nil {
			t.Errorf("expected reservation to succeed after release, got: %v", err)
		}
	})
}

func TestQuotaUpsert(t *testing.T) {
	env := NewTestEnv(t)

	t.Run("upsert new quota", func(t *testing.T) {
		err := env.Store.UpsertQuota(&models.Quota{
			ID:            "new-tenant",
			TenantID:      "new-tenant",
			GPUQuota:      50,
			MemoryQuotaGB: 512,
			JobQuota:      10,
		})
		if err != nil {
			t.Fatalf("UpsertQuota failed: %v", err)
		}

		quota, err := env.Store.GetQuota("new-tenant")
		if err != nil {
			t.Fatal(err)
		}
		if quota.GPUQuota != 50 {
			t.Errorf("expected GPU quota 50, got %d", quota.GPUQuota)
		}
	})

	t.Run("upsert existing quota updates values", func(t *testing.T) {
		err := env.Store.UpsertQuota(&models.Quota{
			ID:            "default",
			TenantID:      "default",
			GPUQuota:      256,
			MemoryQuotaGB: 4096,
			JobQuota:      50,
		})
		if err != nil {
			t.Fatal(err)
		}

		quota, err := env.Store.GetQuota("default")
		if err != nil {
			t.Fatal(err)
		}
		if quota.GPUQuota != 256 {
			t.Errorf("expected GPU quota 256, got %d", quota.GPUQuota)
		}
	})
}