package tests

import (
	"net/http"
	"testing"
	"time"

	"fuze-ai-paas/backend/internal/auth"
	"fuze-ai-paas/backend/internal/models"
)

func mintToken(env *TestEnv, userID, tenant string, role models.Role) string {
	tok, err := env.AuthMgr.Sign(&auth.Claims{
		UserID:   userID,
		Username: userID,
		TenantID: tenant,
		Role:     role,
		Expires:  time.Now().Add(time.Hour).Unix(),
	})
	if err != nil {
		panic("sign token: " + err.Error())
	}
	return tok
}

func mustCreateJob(t *testing.T, env *TestEnv, tenant string) string {
	t.Helper()
	job := &models.Job{
		TenantID: tenant,
		Name:     "tenant-test",
		Type:     models.JobTypeTraining,
		Status:   models.JobStatusPending,
		GPUs:     1,
		Memory:   8,
		Image:    "pytorch:latest",
	}
	if err := env.Store.CreateJob(job); err != nil {
		t.Fatalf("create job: %v", err)
	}
	return job.ID
}

func TestTrainingJobTenantIsolation(t *testing.T) {
	
	env := NewTestEnvWithAuth(t)

	const ta, tb = "iso-acme", "iso-other"
	acmeID := mustCreateJob(t, env, ta)
	otherID := mustCreateJob(t, env, tb)

	acmeTok := mintToken(env, "alice", ta, models.RoleDeveloper)
	otherTok := mintToken(env, "bob", tb, models.RoleDeveloper)
	adminTok := mintToken(env, "root", "default", models.RolePlatformAdmin)

	w := env.DoAuthGET(http.MethodGet, "/api/v1/training-jobs", acmeTok)
	AssertStatus(t, w, http.StatusOK)
	acmeJobs := ParseJSON[[]models.Job](t, w)
	if len(acmeJobs) != 1 || acmeJobs[0].ID != acmeID {
		t.Fatalf("acme 用户应只看到 1 条 acme 任务，实际 %+v", acmeJobs)
	}

	AssertStatus(t, env.DoAuthGET(http.MethodGet, "/api/v1/training-jobs/"+acmeID, acmeTok), http.StatusOK)
	AssertStatus(t, env.DoAuthGET(http.MethodGet, "/api/v1/training-jobs/"+otherID, acmeTok), http.StatusNotFound)
	AssertStatus(t, env.DoAuthGET(http.MethodGet, "/api/v1/training-jobs/"+acmeID, otherTok), http.StatusNotFound)
	
	AssertStatus(t, env.DoAuthJSON(http.MethodDelete, "/api/v1/training-jobs/"+acmeID, otherTok, nil), http.StatusNotFound)
	
	AssertStatus(t, env.DoAuthJSON(http.MethodPost, "/api/v1/training-jobs/"+acmeID+"/cancel", otherTok, nil), http.StatusNotFound)

	w = env.DoAuthGET(http.MethodGet, "/api/v1/training-jobs", adminTok)
	AssertStatus(t, w, http.StatusOK)
	all := ParseJSON[[]models.Job](t, w)
	seen := map[string]bool{}
	for _, j := range all {
		seen[j.ID] = true
	}
	if !seen[acmeID] || !seen[otherID] {
		t.Fatalf("平台管理员应至少能看到两条隔离测试任务，实际 %+v", all)
	}
}

func TestGetJobsByTenantStorage(t *testing.T) {
	env := NewTestEnv(t)
	const ta, tb = "sto-acme", "sto-other"
	mustCreateJob(t, env, ta)
	mustCreateJob(t, env, ta)
	mustCreateJob(t, env, tb)

	acme, err := env.Store.GetJobsByTenant(ta)
	if err != nil {
		t.Fatalf("GetJobsByTenant: %v", err)
	}
	if len(acme) != 2 {
		t.Fatalf("acme 应精确返回 2 条，实际 %d", len(acme))
	}
	for _, j := range acme {
		if j.TenantID != ta {
			t.Fatalf("acme 查询泄露了他租户任务: %+v", j)
		}
	}
	other, err := env.Store.GetJobsByTenant(tb)
	if err != nil {
		t.Fatalf("GetJobsByTenant: %v", err)
	}
	if len(other) != 1 {
		t.Fatalf("other 应精确返回 1 条，实际 %d", len(other))
	}
	for _, j := range other {
		if j.TenantID != tb {
			t.Fatalf("other 查询泄露了他租户任务: %+v", j)
		}
	}
	all, err := env.Store.GetJobsByTenant("")
	if err != nil {
		t.Fatalf("GetJobsByTenant(\"\"): %v", err)
	}
	if len(all) < 3 {
		t.Fatalf("空租户查询应返回全部任务（至少 3 条），实际 %d", len(all))
	}
}

func TestGetTrainingJobLogsUnavailableInMock(t *testing.T) {
	env := NewTestEnv(t) 
	env.EnsureDefaultQuota(t, 64, 1024, 10)

	w := env.DoJSON(http.MethodPost, "/api/v1/training-jobs", map[string]interface{}{
		"name":  "logs-job",
		"image": "pytorch:latest",
		"gpus":  1,
	})
	AssertStatus(t, w, http.StatusCreated)
	job := ParseJSON[models.Job](t, w)

	lw := env.DoJSON(http.MethodGet, "/api/v1/training-jobs/"+job.ID+"/logs", nil)
	AssertStatus(t, lw, http.StatusOK)
	body := ParseJSON[map[string]interface{}](t, lw)
	avail, _ := body["available"].(bool)
	if avail {
		t.Fatalf("mock 任务日志应 available=false，实际 %v", body)
	}
	
	if _, ok := body["pods"].([]interface{}); !ok {
		t.Fatalf("响应应始终包含 pods 数组，实际 %v", body)
	}

	dw := env.DoJSON(http.MethodGet, "/api/v1/training-jobs/"+job.ID+"/logs?pod=whatever&task=worker&tail=20", nil)
	AssertStatus(t, dw, http.StatusOK)
	if avail, _ := ParseJSON[map[string]interface{}](t, dw)["available"].(bool); avail {
		t.Fatal("mock 任务带下钻参数时仍应 available=false")
	}
}