package api

import (
	"net/http"
	"strings"
	"time"

	"fuze-ai-paas/backend/internal/auth"
	"fuze-ai-paas/backend/internal/k8s"
	"fuze-ai-paas/backend/internal/metrics"
	"fuze-ai-paas/backend/internal/models"

	"github.com/gin-gonic/gin"
)

func RegisterRoutes(r *gin.Engine, h *Handler, authMgr *auth.Manager, sso SSOConfig, authEnabled bool, clusterMgr k8s.ClusterRegistry) {
	
	cors := newCORSMiddleware(sso.FrontendURL)
	r.Use(cors)

	reg := metrics.NewRegistry(h.BusinessSnapshot)
	r.Use(metrics.NewMiddleware(reg))

	public := r.Group("/api/v1")
	{
		public.GET("/health", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{
				"status":    "healthy",
				"timestamp": time.Now(),
				"mode":      getSchedulerMode(clusterMgr),
				"auth":      authEnabled,
			})
		})
		public.POST("/auth/login", h.Login)

		public.GET("/auth/sso", h.ListSSOProviders)
		
		if sso.Registry != nil || sso.OIDC != nil || sso.LDAP.Enabled {
			public.GET("/auth/sso/:provider/start", h.OIDCStart)
			public.GET("/auth/sso/:provider/callback", h.OIDCCallback)
			public.POST("/auth/sso/:provider/login", h.LDAPLogin)
		}
		
		public.POST("/auth/mfa/verify", h.VerifyMFA)
		
		public.POST("/auth/passkey/login/begin", h.PasskeyLoginBegin)
		public.POST("/auth/passkey/login/finish", h.PasskeyLoginFinish)
	}

	protected := r.Group("/api/v1")
	if authEnabled {
		protected.Use(authMgr.Middleware())
	} else {
		protected.Use(auth.PassthroughMiddleware())
	}
	{
		protected.GET("/auth/me", h.Me)
		
		protected.POST("/auth/logout", h.Logout)

		protected.POST("/auth/tokens", h.CreateToken)
		protected.GET("/auth/tokens", h.ListTokens)
		protected.POST("/auth/tokens/:id/rotate", h.RotateToken)
		protected.DELETE("/auth/tokens/:id", h.DeleteToken)

		protected.POST("/auth/mfa/enroll", h.MFAEnroll)
		protected.POST("/auth/mfa/disable", h.MFADisable)

		protected.POST("/auth/passkey/register/begin", h.PasskeyRegisterBegin)
		protected.POST("/auth/passkey/register/finish", h.PasskeyRegisterFinish)
		protected.POST("/auth/passkey/disable", h.PasskeyDisable)

		protected.GET("/evaluations", h.ListEvaluations)
		protected.POST("/evaluations", h.CreateEvaluation)
		protected.GET("/evaluations/:id", h.GetEvaluation)
		protected.POST("/evaluations/:id/result", h.RecordEvaluationResult)
		protected.POST("/evaluations/:id/fail", h.FailEvaluation)
		protected.DELETE("/evaluations/:id", h.DeleteEvaluation)
		
		protected.GET("/evaluations/:id/reviews", h.ListEvaluationReviews)
		protected.POST("/evaluations/:id/reviews", h.SubmitEvaluationReview)
		protected.POST("/evaluations/:id/llm-judge", h.RunEvaluationLLMJudge)
		protected.GET("/evaluations/:id/report", h.GetEvaluationReport)
		protected.POST("/evaluations/:id/finalize", h.FinalizeEvaluationReport)
		
		protected.GET("/experiments/:id/evaluations", h.ListExperimentEvaluations)

		protected.GET("/resources", h.GetResources)
		protected.GET("/resources/:id", h.GetResource)
		protected.POST("/resources", h.CreateResource)

		protected.GET("/training-jobs", h.GetTrainingJobs)
		protected.POST("/training-jobs", h.CreateTrainingJob)
		protected.GET("/training-jobs/:id", h.GetTrainingJob)
		protected.DELETE("/training-jobs/:id", h.DeleteTrainingJob)
		protected.GET("/training-jobs/:id/logs", h.GetTrainingJobLogs)
		protected.POST("/training-jobs/:id/cancel", h.CancelTrainingJob)
		protected.POST("/training-jobs/:id/resume", h.ResumeTrainingJob)
		protected.POST("/training-jobs/:id/complete", h.CompleteTrainingJob)
		protected.POST("/training-jobs/:id/failures", h.ReportTrainingFailure)
		protected.GET("/training-jobs/:id/checkpoints", h.GetTrainingCheckpoint)
		protected.POST("/training-jobs/:id/checkpoints", h.RecordTrainingCheckpoint)

		protected.GET("/training-templates", h.GetTrainingTemplates)

		protected.GET("/experiments", h.ListExperiments)
		protected.POST("/experiments", h.CreateExperiment)
		protected.GET("/experiments/compare", h.CompareExperiments)
		protected.GET("/experiments/:id", h.GetExperiment)
		protected.POST("/experiments/:id/archive", h.ArchiveExperiment)
		protected.DELETE("/experiments/:id", h.DeleteExperiment)
		protected.GET("/experiments/:id/runs", h.ListRuns)
		protected.POST("/experiments/:id/runs", h.CreateRun)
		protected.POST("/experiments/:id/runs/:runId/complete", h.CompleteRun)
		protected.POST("/experiments/:id/runs/:runId/fail", h.FailRun)
		protected.POST("/experiments/:id/runs/:runId/cancel", h.CancelRun)
		protected.POST("/experiments/runs/:runId/reproduce", h.ReproduceRun)
		protected.GET("/experiments/runs/:runId/reproduction", h.GetReproduction)

		h.HPORegisterRoutes(protected)

		protected.POST("/metrics/query", h.QueryMetrics)
		protected.POST("/metrics/latest", h.QueryMetricLatest)

		alertAdmin := authMgr.RequireRole(models.RolePlatformAdmin, models.RoleTenantAdmin)
		
		protected.GET("/alerts/rules", alertAdmin, h.ListAlertRules)
		protected.POST("/alerts/rules", alertAdmin, h.CreateAlertRule)
		protected.PUT("/alerts/rules/:id", alertAdmin, h.UpdateAlertRule)
		protected.DELETE("/alerts/rules/:id", alertAdmin, h.DeleteAlertRule)
		protected.POST("/alerts/rules/:id/toggle", alertAdmin, h.ToggleAlertRule)
		
		protected.GET("/alerts/active", h.ListActiveAlerts)
		
		protected.GET("/alerts/silences", alertAdmin, h.ListSilences)
		protected.POST("/alerts/silences", alertAdmin, h.CreateSilence)
		protected.DELETE("/alerts/silences/:id", alertAdmin, h.DeleteSilence)

		protected.GET("/inference-services", h.GetInferenceServices)
		protected.GET("/inference-services/:id", h.GetInferenceService)
		protected.POST("/inference-services", h.CreateInferenceService)
		
		protected.PUT("/inference-services", h.ApplyInferenceService)
		protected.PATCH("/inference-services/:id", h.PatchInferenceService)
		protected.DELETE("/inference-services/:id", h.DeleteInferenceService)

		h.registerEdgeRoutes(protected)

		protected.GET("/models", h.GetModels)
		protected.GET("/models/:id", h.GetModel)
		protected.POST("/models", h.CreateModel)
		protected.PUT("/models/:id", h.UpdateModel)
		protected.DELETE("/models/:id", h.DeleteModel)
		protected.GET("/models/:id/versions", h.GetModelVersions)
		protected.GET("/models/:id/versions/:vid", h.GetModelVersion)
		protected.GET("/models/:id/versions/:vid/lineage", h.GetModelVersionLineage)
		protected.POST("/models/:id/versions", h.CreateModelVersion)
		protected.DELETE("/models/:id/versions/:vid", h.DeleteModelVersion)

		protected.GET("/datasets", h.GetDatasets)
		protected.GET("/datasets/:id", h.GetDataset)
		protected.POST("/datasets", h.CreateDataset)
		protected.DELETE("/datasets/:id", h.DeleteDataset)

		protected.GET("/metrics", h.GetMetrics)
		protected.GET("/queues", h.GetQueues)

		protected.GET("/clusters", h.GetClusters)
		
		protected.POST("/clusters", authMgr.RequireRole(models.RolePlatformAdmin), h.RegisterCluster)
		protected.GET("/clusters/:id", h.GetCluster)
		protected.PUT("/clusters/:id", authMgr.RequireRole(models.RolePlatformAdmin), h.UpdateCluster)
		protected.DELETE("/clusters/:id", authMgr.RequireRole(models.RolePlatformAdmin), h.DeleteCluster)
		protected.POST("/clusters/:id/discover", authMgr.RequireRole(models.RolePlatformAdmin), h.DiscoverCluster)
		protected.POST("/clusters/:id/test", authMgr.RequireRole(models.RolePlatformAdmin), h.TestCluster)
		protected.GET("/clusters/:id/resources", h.GetClusterResources)

		tenantAdmin := authMgr.RequireRole(models.RolePlatformAdmin, models.RoleTenantAdmin)
		protected.GET("/tenants", tenantAdmin, h.ListTenants)
		protected.GET("/tenants/:id", h.GetTenant)
		protected.POST("/tenants", authMgr.RequireRole(models.RolePlatformAdmin, models.RoleTenantAdmin), h.CreateTenant)
		protected.PUT("/tenants/:id", authMgr.RequireRole(models.RolePlatformAdmin, models.RoleTenantAdmin), h.UpdateTenant)
		protected.DELETE("/tenants/:id", authMgr.RequireRole(models.RolePlatformAdmin), h.DeleteTenant)

		protected.GET("/quotas", tenantAdmin, h.ListQuotas)
		protected.GET("/quotas/:tenantId", h.GetQuota)
		protected.PUT("/quotas/:tenantId", authMgr.RequireRole(models.RolePlatformAdmin, models.RoleTenantAdmin), h.UpdateQuota)

		protected.GET("/audit", authMgr.RequireRole(models.RolePlatformAdmin, models.RoleTenantAdmin), h.ListAudit)

		idpAdmin := authMgr.RequireRole(models.RolePlatformAdmin)
		protected.GET("/sso/idps", idpAdmin, h.ListIdPs)
		protected.POST("/sso/idps", idpAdmin, h.CreateIdP)
		protected.PUT("/sso/idps/:id", idpAdmin, h.UpdateIdP)
		protected.DELETE("/sso/idps/:id", idpAdmin, h.DeleteIdP)
		protected.POST("/sso/idps/:id/test", idpAdmin, h.TestIdP)

		protected.GET("/llm/routes", h.ListRoutes)
		protected.PUT("/llm/routes", h.UpsertRoute)
		protected.DELETE("/llm/routes/:model", h.DeleteRoute)

		protected.GET("/llm/quota", h.LLMGetQuota)
		protected.PUT("/llm/quota", h.LLMSetQuota)
		protected.GET("/llm/usage", h.ListUsage)
		protected.GET("/llm/usage/sum", h.SumUsage)

		priceAdmin := authMgr.RequireRole(models.RolePlatformAdmin, models.RoleTenantAdmin)
		protected.GET("/llm/prices/llm", priceAdmin, h.ListLLMPrices)
		protected.PUT("/llm/prices/llm", priceAdmin, h.UpsertLLMPrice)
		protected.DELETE("/llm/prices/llm/:model", priceAdmin, h.DeleteLLMPrice)
		protected.GET("/llm/prices/gpu", priceAdmin, h.ListGPUPrices)
		protected.PUT("/llm/prices/gpu", priceAdmin, h.UpsertGPUPrice)
		protected.DELETE("/llm/prices/gpu/:gpuType", priceAdmin, h.DeleteGPUPrice)

		protected.GET("/cost/summary", h.CostSummary)

		protected.GET("/llm/traces", h.ListTraces)
		protected.GET("/llm/traces/:id", h.GetTrace)

		protected.GET("/llm/prompts", h.ListPrompts)
		protected.POST("/llm/prompts", h.CreatePrompt)
		protected.GET("/llm/prompts/:name", h.GetPrompt)
		protected.POST("/llm/prompts/:name/versions", h.AddPromptVersion)
		protected.POST("/llm/prompts/:name/activate", h.ActivatePrompt)
		protected.DELETE("/llm/prompts/:name", h.DeletePrompt)

		guardrailAdmin := authMgr.RequireRole(models.RolePlatformAdmin, models.RoleTenantAdmin)
		protected.GET("/llm/guardrail/rules", guardrailAdmin, h.ListGuardrailRules)
		protected.POST("/llm/guardrail/rules", guardrailAdmin, h.UpsertGuardrailRule)
		protected.DELETE("/llm/guardrail/rules/:id", guardrailAdmin, h.DeleteGuardrailRule)

		protected.GET("/llm/knowledge", h.ListKnowledgeBases)
		protected.POST("/llm/knowledge", h.CreateKnowledgeBase)
		protected.GET("/llm/knowledge/:id", h.GetKnowledgeBase)
		protected.DELETE("/llm/knowledge/:id", h.DeleteKnowledgeBase)
		protected.POST("/llm/knowledge/:id/documents", h.AddKnowledgeDocument)
		protected.GET("/llm/knowledge/:id/documents", h.ListKnowledgeDocuments)
		protected.DELETE("/llm/knowledge/:id/documents/:docId", h.DeleteKnowledgeDocument)

		adapterAdmin := authMgr.RequireRole(models.RolePlatformAdmin, models.RoleTenantAdmin)
		protected.GET("/llm/finetune/adapters", h.ListFineTuneAdapters)
		protected.POST("/llm/finetune/adapters", h.CreateFineTuneAdapter)
		protected.GET("/llm/finetune/adapters/:id", h.GetFineTuneAdapter)
		protected.DELETE("/llm/finetune/adapters/:id", adapterAdmin, h.DeleteFineTuneAdapter)

		protected.GET("/llm/finetune/adapters/:id/mounts", h.ListFineTuneAdapterMounts)
		protected.POST("/llm/finetune/adapters/:id/mounts", adapterAdmin, h.MountFineTuneAdapter)
		protected.DELETE("/llm/finetune/adapters/:id/mounts/:serviceId", adapterAdmin, h.UnmountFineTuneAdapter)

		protected.POST("/llm/chat", h.LLMChat)

		protected.GET("/agents", h.ListAgents)
		protected.POST("/agents", h.CreateAgent)
		protected.GET("/agents/:id", h.GetAgent)
		protected.POST("/agents/:id/compile", h.CompileAgent)
		protected.DELETE("/agents/:id", h.DeleteAgent)
		protected.POST("/agents/:id/runs", h.StartRun)
		protected.GET("/agents/:id/runs", h.ListAgentRuns)
		protected.GET("/agents/:id/runs/:runId", h.GetRun)
		protected.POST("/agents/:id/runs/:runId/resume", h.ResumeRun)

		protected.GET("/tools", h.ListTools)
		protected.POST("/tools", h.CreateTool)
		protected.GET("/tools/:id", h.GetTool)
		protected.DELETE("/tools/:id", h.DeleteTool)

		protected.GET("/workspaces", h.ListWorkspaces)
		protected.POST("/workspaces", h.CreateWorkspace)
		protected.GET("/workspaces/:id", h.GetWorkspace)
		protected.POST("/workspaces/:id/start", h.StartWorkspace)
		protected.POST("/workspaces/:id/stop", h.StopWorkspace)
		protected.DELETE("/workspaces/:id", h.DeleteWorkspace)
		protected.POST("/workspaces/:id/activity", h.WorkspaceActivity)
		protected.Any("/workspaces/:id/proxy/*path", h.WorkspaceProxy)

		protected.POST("/data/pipelines", h.CreateDataPipeline)
		protected.GET("/data/pipelines", h.ListDataPipelines)
		protected.GET("/data/pipelines/:id", h.GetDataPipeline)
		protected.POST("/data/pipelines/:id/submit", h.SubmitDataPipeline)
		protected.POST("/data/pipelines/:id/cancel", h.CancelDataPipeline)
		protected.POST("/data/annotations", h.CreateAnnotation)
		protected.GET("/data/annotations", h.ListAnnotations)
		protected.GET("/data/annotations/:id", h.GetAnnotation)
		protected.POST("/data/annotations/:id/export", h.ExportAnnotation)

		protected.GET("/optimize/tasks", h.ListCompressionTasks)
		protected.POST("/optimize/tasks", h.CreateCompressionTask)
		protected.GET("/optimize/tasks/:id", h.GetCompressionTask)
		protected.POST("/optimize/tasks/:id/cancel", h.CancelCompressionTask)
		protected.POST("/optimize/tasks/:id/result", h.HandleCompressionResult)
		protected.DELETE("/optimize/tasks/:id", h.DeleteCompressionTask)
	}

	v1grp := r.Group("/v1")
	if authEnabled {
		v1grp.Use(authMgr.Middleware())
	} else {
		v1grp.Use(auth.PassthroughMiddleware())
	}
	v1grp.POST("/chat/completions", h.OpenAIChat)

	r.GET("/metrics", gin.WrapH(reg.Handler()))
}

func newCORSMiddleware(frontendURL string) gin.HandlerFunc {
	allowList := make(map[string]struct{})
	credentials := false
	if frontendURL != "" && frontendURL != "*" && !strings.HasPrefix(frontendURL, "http://localhost") {
		allowList[frontendURL] = struct{}{}
		credentials = true
	}
	
	for _, dev := range []string{"http://localhost:3000", "http://localhost:8080"} {
		if frontendURL == "" || frontendURL == "http://localhost:3000" {
			allowList[dev] = struct{}{}
		}
	}

	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")
		if origin != "" {
			if _, ok := allowList[origin]; ok {
				c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
				if credentials {
					c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
				}
				c.Writer.Header().Set("Vary", "Origin")
			}
			
		}
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	}
}

const (
	schedulerModeMock         = "mock"
	schedulerModeMultiCluster = "multi-cluster"
)

func getSchedulerMode(mgr k8s.ClusterRegistry) string {
	for _, id := range mgr.List() {
		if c, err := mgr.Get(id); err == nil && c.Enabled() {
			return schedulerModeMultiCluster
		}
	}
	return schedulerModeMock
}