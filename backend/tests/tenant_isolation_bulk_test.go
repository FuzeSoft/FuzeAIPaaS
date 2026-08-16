package tests

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"fuze-ai-paas/backend/internal/models"
)

type idResponse struct {
	ID string `json:"id"`
}

type isolationCase struct {
	name string
	
	listPath string
	
	listWrapper string
	
	itemPath string
	
	create func(t *testing.T, env *TestEnv, victimToken string) string
	
	stillExists func(t *testing.T, env *TestEnv, id string)
}

func TestTenantIsolationBulk(t *testing.T) {
	env := NewTestEnvWithAuth(t)

	victimToken := seedTenantUser(t, env, "iso-victim", "iso-victim-dev", models.RoleDeveloper)
	attackerToken := seedTenantUser(t, env, "iso-attacker", "iso-attacker-dev", models.RoleDeveloper)

	cases := []isolationCase{
		{
			name:     "training-jobs",
			listPath: "/api/v1/training-jobs",
			itemPath: "/api/v1/training-jobs/%s",
			create: func(t *testing.T, env *TestEnv, token string) string {
				w := env.DoAuthJSON(http.MethodPost, "/api/v1/training-jobs", token, map[string]interface{}{
					"name":     "victim-train",
					"image":    "registry.example.com/train:latest",
					"gpus":     1,
					"priority": 1,
					"entrypoint": []string{"python", "train.py"},
				})
				AssertStatus(t, w, http.StatusCreated)
				id := ParseJSON[idResponse](t, w).ID
				if id == "" {
					t.Fatal("created training job has empty id")
				}
				return id
			},
			stillExists: func(t *testing.T, env *TestEnv, id string) {
				if _, err := env.Store.GetJob(id); err != nil {
					t.Fatalf("越权：受害者训练任务被攻击者删除: %v", err)
				}
			},
		},
		{
			name:     "experiments",
			listPath: "/api/v1/experiments",
			itemPath: "/api/v1/experiments/%s",
			create: func(t *testing.T, env *TestEnv, token string) string {
				w := env.DoAuthJSON(http.MethodPost, "/api/v1/experiments", token, map[string]interface{}{
					"name":        "victim-exp",
					"objective":   "maximize",
					"metric_name": "accuracy",
				})
				AssertStatus(t, w, http.StatusCreated)
				id := ParseJSON[idResponse](t, w).ID
				if id == "" {
					t.Fatal("created experiment has empty id")
				}
				return id
			},
			stillExists: func(t *testing.T, env *TestEnv, id string) {
				if _, err := env.Store.Experiment().GetExperiment(id); err != nil {
					t.Fatalf("越权：受害者实验被攻击者删除: %v", err)
				}
			},
		},
		{
			name:        "evaluations",
			listPath:    "/api/v1/evaluations",
			listWrapper: "evaluations",
			itemPath:    "/api/v1/evaluations/%s",
			create: func(t *testing.T, env *TestEnv, token string) string {
				w := env.DoAuthJSON(http.MethodPost, "/api/v1/evaluations", token, map[string]interface{}{
					"name":      "victim-eval",
					"task":      "classification",
					"dataset":   "s3://bucket/data",
					"model_id":  "m-victim-placeholder",
				})
				AssertStatus(t, w, http.StatusCreated)
				id := ParseJSON[idResponse](t, w).ID
				if id == "" {
					t.Fatal("created evaluation has empty id")
				}
				return id
			},
			stillExists: func(t *testing.T, env *TestEnv, id string) {
				if _, err := env.Store.Evaluation().Get(context.Background(), id); err != nil {
					t.Fatalf("越权：受害者评估被攻击者删除: %v", err)
				}
			},
		},
		{
			name:     "models",
			listPath: "/api/v1/models",
			itemPath: "/api/v1/models/%s",
			create: func(t *testing.T, env *TestEnv, token string) string {
				w := env.DoAuthJSON(http.MethodPost, "/api/v1/models", token, map[string]interface{}{
					"name":        "victim-model",
					"framework":   "pytorch",
					"storage_uri": "s3://bucket/model",
				})
				AssertStatus(t, w, http.StatusCreated)
				id := ParseJSON[idResponse](t, w).ID
				if id == "" {
					t.Fatal("created model has empty id")
				}
				return id
			},
			stillExists: func(t *testing.T, env *TestEnv, id string) {
				if _, err := env.Store.GetModel(id); err != nil {
					t.Fatalf("越权：受害者模型被攻击者删除: %v", err)
				}
			},
		},
		{
			name:     "datasets",
			listPath: "/api/v1/datasets",
			itemPath: "/api/v1/datasets/%s",
			create: func(t *testing.T, env *TestEnv, token string) string {
				w := env.DoAuthJSON(http.MethodPost, "/api/v1/datasets", token, map[string]interface{}{
					"name":      "victim-ds",
					"cluster_id": "cluster-001",
				})
				AssertStatus(t, w, http.StatusCreated)
				id := ParseJSON[idResponse](t, w).ID
				if id == "" {
					t.Fatal("created dataset has empty id")
				}
				return id
			},
			stillExists: func(t *testing.T, env *TestEnv, id string) {
				if _, err := env.Store.GetDataset(id); err != nil {
					t.Fatalf("越权：受害者数据集被攻击者删除: %v", err)
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			victimID := tc.create(t, env, victimToken)

			t.Run("list 不得泄露他租户资源", func(t *testing.T) {
				w := env.DoAuthGET(http.MethodGet, tc.listPath, attackerToken)
				AssertStatus(t, w, http.StatusOK)

				var items []idResponse
				if tc.listWrapper == "" {
					items = ParseJSON[[]idResponse](t, w)
				} else {
					wrapped := ParseJSON[map[string][]idResponse](t, w)
					items = wrapped[tc.listWrapper]
				}
				for _, it := range items {
					if it.ID == victimID {
						t.Fatalf("越权：攻击者列表 %s 中出现了受害者资源 %s", tc.listPath, victimID)
					}
				}
			})

			t.Run("get 他租户资源按不存在处理", func(t *testing.T) {
				w := env.DoAuthGET(http.MethodGet, fmt.Sprintf(tc.itemPath, victimID), attackerToken)
				AssertStatus(t, w, http.StatusNotFound)
			})

			t.Run("delete 他租户资源按不存在处理且不删除数据", func(t *testing.T) {
				w := env.DoAuthJSON(http.MethodDelete, fmt.Sprintf(tc.itemPath, victimID), attackerToken, nil)
				AssertStatus(t, w, http.StatusNotFound)
				tc.stillExists(t, env, victimID)
			})
		})
	}
}