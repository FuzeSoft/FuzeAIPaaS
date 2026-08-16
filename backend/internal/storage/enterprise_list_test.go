package storage

import (
	"fmt"
	"testing"

	"fuze-ai-paas/backend/internal/models"
)

func TestListUsersNotTruncated(t *testing.T) {
	s := newEntStorage(t)

	const n = defaultListLimit + 1
	users := make([]models.User, 0, n)
	for i := 0; i < n; i++ {
		users = append(users, models.User{
			ID:       fmt.Sprintf("u-%05d", i),
			Username: fmt.Sprintf("user-%05d", i),
			Role:     models.RoleDeveloper,
			TenantID: "default",
			Enabled:  true,
		})
	}
	
	if err := s.db.Create(&users).Error; err != nil {
		t.Fatalf("bulk create users: %v", err)
	}

	got, err := s.ListUsers()
	if err != nil {
		t.Fatalf("ListUsers: %v", err)
	}
	
	if len(got) != n+1 {
		t.Fatalf("ListUsers 应返回全量 %d 条（含 seed 默认用户），实际 %d 条——被截断？", n+1, len(got))
	}
}

func TestListTenantsNotTruncated(t *testing.T) {
	s := newEntStorage(t)

	const n = defaultListLimit + 1
	tenants := make([]models.Tenant, 0, n)
	for i := 0; i < n; i++ {
		tenants = append(tenants, models.Tenant{
			ID:   fmt.Sprintf("tn-%05d", i),
			Name: fmt.Sprintf("tenant-%05d", i),
		})
	}
	if err := s.db.Create(&tenants).Error; err != nil {
		t.Fatalf("bulk create tenants: %v", err)
	}

	got, err := s.ListTenants()
	if err != nil {
		t.Fatalf("ListTenants: %v", err)
	}
	
	if len(got) != n+1 {
		t.Fatalf("ListTenants 应返回全量 %d 条（含 seed 默认租户），实际 %d 条——被截断？", n+1, len(got))
	}
}