package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	evaluationapp "fuze-ai-paas/backend/internal/app/evaluation"
	experimentapp "fuze-ai-paas/backend/internal/app/experiment"
	agentapp "fuze-ai-paas/backend/internal/app/agent"
	optimizeapp "fuze-ai-paas/backend/internal/app/optimize"
	edgeapp "fuze-ai-paas/backend/internal/app/edge"
	hpoapp "fuze-ai-paas/backend/internal/app/hpo"
	llmgateway "fuze-ai-paas/backend/internal/app/llmgateway"
	token "fuze-ai-paas/backend/internal/app/token"
	trainingapp "fuze-ai-paas/backend/internal/app/training"
	workspaceapp "fuze-ai-paas/backend/internal/app/workspace"
	dataapp "fuze-ai-paas/backend/internal/app/data"
	"fuze-ai-paas/backend/internal/auth"
	"fuze-ai-paas/backend/internal/domain/event"
	"fuze-ai-paas/backend/internal/domain/llm"
	"fuze-ai-paas/backend/internal/domain/optimize"
	"fuze-ai-paas/backend/internal/events"
	"fuze-ai-paas/backend/internal/notify"

	"fuze-ai-paas/backend/internal/k8s"
	"fuze-ai-paas/backend/internal/models"
	"fuze-ai-paas/backend/internal/ports"
	"fuze-ai-paas/backend/internal/scheduler"
	"fuze-ai-paas/backend/internal/storage"
	"fuze-ai-paas/backend/internal/telemetry"
	"log"
	"os"

	"github.com/gin-gonic/gin"
)

type Repos struct {
	Cluster      ports.ClusterWriter
	Job          ports.JobWriter
	Inference    ports.InferenceWriter
	Resource     ports.ResourceWriter
	Model        ports.ModelWriter
	Dataset      ports.DatasetWriter
	Tenant       ports.TenantWriter
	Quota        ports.QuotaWriter
	Audit        ports.AuditWriter
	User         ports.UserWriter
	Experiment   ports.ExperimentRepository
	MetricsQuery ports.MetricsQuery
	Token        ports.TokenRepository
	Evaluation   ports.EvaluationRepository
	
	Judge ports.JudgeLLM

	Route      ports.RouteRepository
	TokenQuota ports.TokenQuotaRepository
	TokenUsage ports.TokenUsageRepository
	Trace      ports.TraceRepository
	Prompt     ports.PromptRepository
	Knowledge  ports.KnowledgeRepository
	
	FineTune ports.FineTuneRepository
	
	AdapterMounts ports.AdapterMountRepository
	JudgeLLM      ports.JudgeLLM 

	LLMCompleter ports.ChatCompleter

	Guardrail ports.GuardrailRepository

	BudgetAlert notify.EventSink

	PriceRepo ports.PriceRepository
	
	CostRepo ports.CostRepository

	Workspace   ports.WorkspaceRepository
	WorkspaceRT ports.WorkspaceRuntime
	
	WorkspaceNotifier notify.EventSink

	Alert ports.AlertRepository

	VectorStore ports.VectorStore
	
	Embedder ports.Embedder

	Agent ports.AgentRepository
	
	ToolRegistry ports.ToolRepository

	Data ports.DataRepository
	
	DataArtifact ports.ArtifactStore

	Study ports.StudyRepository
	Trial ports.TrialRepository
	
	HPOReport ports.ReportRepository

	OptimizeRepo    optimize.CompressionRepository
	OptimizeExecutor optimize.CompressionExecutor
}

type Handler struct {
	clusterRepo   ports.ClusterWriter
	inferenceRepo ports.InferenceWriter
	resourceRepo  ports.ResourceWriter
	modelRepo     ports.ModelWriter
	datasetRepo   ports.DatasetWriter
	tenantRepo    ports.TenantWriter
	quotaRepo     ports.QuotaWriter
	auditRepo     ports.AuditWriter
	userRepo      ports.UserWriter

	training *trainingapp.Service

	experiment *experimentapp.Service

	hpo *hpoapp.Service

	hpoGatewayBase string

	hpoClient *http.Client

	proxyLimiter *tenantLimiter

	jobRepo ports.JobWriter

	metrics ports.MetricsQuery

	token *token.Service

	evaluation *evaluationapp.Service

	llmRoute     ports.RouteRepository
	llmQuota     ports.TokenQuotaRepository
	llmUsage     ports.TokenUsageRepository
	llmTrace     ports.TraceRepository
	llmPrompt    ports.PromptRepository
	llmKnowledge ports.KnowledgeRepository
	
	finetune ports.FineTuneRepository
	
	adapterMounts ports.AdapterMountRepository
	
	adapterServices adapterServiceReader
	
	adapterJobs ports.JobExistenceChecker

	guardrail ports.GuardrailRepository
	
	guardCache *llmgateway.GuardCache

	priceRepo ports.PriceRepository
	
	costRepo ports.CostRepository

	llmgw *llmgateway.Service

	workspace     *workspaceapp.Service
	workspaceRepo ports.WorkspaceRepository
	workspaceRT   ports.WorkspaceRuntime

	alert ports.AlertRepository

	vectorStore ports.VectorStore
	
	embedder ports.Embedder

	agent *agentapp.Service
	
	toolRepo ports.ToolRepository

	data *dataapp.Service

	compress *optimizeapp.Service

	edge *edgeapp.Service

	scheduler  *scheduler.Scheduler
	clusterMgr k8s.ClusterRegistry
	auth       *auth.Manager
	sso        SSOConfig
	telemetry  *telemetry.Collector
	
	bus *events.Bus
}

type noopScheduler struct{}

func (noopScheduler) SubmitJob(*models.Job) error    { return nil }
func (noopScheduler) TerminateJob(*models.Job) error { return nil }
func (noopScheduler) CancelJob(*models.Job) error    { return nil }

func trainingScheduler(s *scheduler.Scheduler) trainingapp.Scheduler {
	if s == nil {
		return noopScheduler{}
	}
	return s
}

func (h *Handler) SetTelemetry(c *telemetry.Collector) { h.telemetry = c }

func (h *Handler) SetEdge(svc *edgeapp.Service) { h.edge = svc }

func (h *Handler) StopSSO() {
	h.sso.Stop()
}

func (h *Handler) StopAuth() {
	h.auth.Stop()
}

func (c *SSOConfig) ldapConfigFor(ctx context.Context, providerID string) (*auth.LDAPConfig, *models.IdPConfig, error) {
	if c.Registry != nil && providerID != "" {
		cfg, err := c.Registry.Get(ctx, providerID)
		if err != nil {
			return nil, nil, err
		}
		if !cfg.Enabled {
			return nil, nil, ports.ErrNotFound
		}
		if cfg.Type != models.IdPLDAP {
			return nil, nil, fmt.Errorf("sso: provider %q is not ldap", providerID)
		}
		lc := &auth.LDAPConfig{
			Enabled:       true,
			Addr:          cfg.LDAPAddr,
			UseTLS:        cfg.LDAPUseTLS,
			SkipTLSVerify: cfg.LDAPSkipVerify,
			UserDNFormat:  cfg.LDAPUserDNFormat,
			DefaultRole:   cfg.DefaultRole,
			AdminGroups:   cfg.AdminGroups,
			AdminRole:     cfg.AdminRole,
			DefaultTenant: cfg.DefaultTenant,
		}
		return lc, cfg, nil
	}
	
	if c.LDAP.Enabled {
		lc := c.LDAP
		return &lc, nil, nil
	}
	return nil, nil, errors.New("sso: no ldap provider configured")
}

func NewHandler(repos Repos, scheduler *scheduler.Scheduler, clusterMgr k8s.ClusterRegistry, authMgr *auth.Manager, bus *events.Bus, sso ...SSOConfig) *Handler {
	h := &Handler{
		clusterRepo:   repos.Cluster,
		inferenceRepo: repos.Inference,
		resourceRepo:  repos.Resource,
		modelRepo:     repos.Model,
		datasetRepo:   repos.Dataset,
		tenantRepo:    repos.Tenant,
		quotaRepo:     repos.Quota,
		auditRepo:     repos.Audit,
		userRepo:      repos.User,
		toolRepo:      repos.ToolRegistry,
		scheduler:     scheduler,
		clusterMgr:    clusterMgr,
		auth:          authMgr,
		bus:           bus,
	}
	
	var trainingOpts []trainingapp.TrainingOption
	if repos.PriceRepo != nil && repos.CostRepo != nil {
		prices := loadPriceBook(repos.PriceRepo)
		trainingOpts = append(trainingOpts, trainingapp.WithCostBilling(
			prices,
			repos.CostRepo,
			newBudgetAlert(repos.BudgetAlert),
		))
	}
	
	if repos.FineTune != nil {
		trainingOpts = append(trainingOpts, trainingapp.WithFineTuneRegistry(repos.FineTune))
	}
	
	trainingOpts = append(trainingOpts, trainingapp.WithExperimentRepository(repos.Experiment))
	h.training = trainingapp.NewService(repos.Job, repos.Quota, repos.Model, trainingScheduler(scheduler), trainingOpts...)
	h.experiment = experimentapp.NewService(repos.Experiment)
	
	if repos.Study != nil && repos.Trial != nil {
		h.hpo = hpoapp.NewService(repos.Study, repos.Trial, repos.Experiment, h.training, repos.Quota)
		if repos.HPOReport != nil {
			h.hpo.SetReportRepository(repos.HPOReport)
		}
		
		if h.bus != nil {
			hpoSvc := h.hpo
			h.bus.Subscribe(event.JobStateChangedType, func(ctx context.Context, e event.Event) {
				ev, ok := e.(event.JobStateChanged)
				if !ok || !ev.Terminal {
					return
				}
				if err := hpoSvc.OnJobTerminal(ctx, ev.JobID, ev.Status); err != nil {
					log.Printf("[HPO] OnJobTerminal for job %s (status=%s) failed: %v", ev.JobID, ev.Status, err)
				}
			})
		}
	}
	
	h.hpoGatewayBase = os.Getenv("HPO_GATEWAY_BASE_URL")
	
	h.hpoClient = &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 20,
			IdleConnTimeout:     90 * time.Second,
		},
	}
	
	h.proxyLimiter = newTenantLimiter(20)
	h.jobRepo = repos.Job
	if repos.MetricsQuery != nil {
		h.metrics = repos.MetricsQuery
	}
	if repos.Token != nil {
		h.token = token.NewService(authMgr, repos.Token)
	}
	if repos.Evaluation != nil {
		h.evaluation = evaluationapp.NewService(repos.Evaluation, repos.Judge)
	}
	
	h.llmRoute = repos.Route
	h.llmQuota = repos.TokenQuota
	h.llmUsage = repos.TokenUsage
	h.llmTrace = repos.Trace
	h.llmPrompt = repos.Prompt
	h.llmKnowledge = repos.Knowledge
	h.finetune = repos.FineTune
	h.adapterMounts = repos.AdapterMounts
	
	if repos.Inference != nil {
		h.adapterServices = repos.Inference
	}
	
	if chk, ok := repos.Job.(ports.JobExistenceChecker); ok {
		h.adapterJobs = chk
	}
	
	if repos.LLMCompleter != nil && repos.Route != nil {
		
		deps := llmgateway.Deps{
			Routes:    llmgateway.NewRepoRouteTable(repos.Route),
			Completer: repos.LLMCompleter,
			Traces:    repos.Trace,
			Usage:     repos.TokenUsage,
			Quota:     repos.TokenQuota,
			
			Prices: loadPriceBook(repos.PriceRepo),
			Guard:  defaultGuard(),
			
			Alert: newBudgetAlert(repos.BudgetAlert),
		}
		
		if repos.AdapterMounts != nil {
			deps.Mounts = repos.AdapterMounts
		}
		if repos.Guardrail != nil {
			h.guardCache = llmgateway.NewGuardCache(repos.Guardrail)
			deps.Guards = h.guardCache
		}
		gw, err := llmgateway.NewService(deps)
		if err != nil {
			
			log.Fatalf("llmgateway service build failed: %v", err)
		}
		h.llmgw = gw
	}
	h.guardrail = repos.Guardrail
	h.alert = repos.Alert
	h.priceRepo = repos.PriceRepo
	h.costRepo = repos.CostRepo
	
	h.vectorStore = repos.VectorStore
	h.embedder = repos.Embedder
	if len(sso) > 0 {
		h.sso = sso[0]
	}
	
	if repos.Workspace != nil && repos.WorkspaceRT != nil {
		h.workspace = workspaceapp.NewService(
			repos.Workspace, repos.Quota, repos.WorkspaceRT, workspaceapp.DefaultImagePolicy(),
			workspaceapp.WithNotifier(repos.WorkspaceNotifier),
		)
		h.workspaceRepo = repos.Workspace
		h.workspaceRT = repos.WorkspaceRT
	}
	
	if repos.Agent != nil {
		deps := agentapp.Deps{Agents: repos.Agent}
		if h.llmgw != nil {
			deps.LLM = &llmGatewayCaller{gw: h.llmgw}
			
			if r := h.llmgw.GetRetriever(); r != nil {
				deps.Retriever = &llmGatewayRetriever{retriever: r}
			}
		}
		
		deps.Tools = agentapp.NewToolExecutor(repos.ToolRegistry, deps.Retriever)
		
		if h.bus != nil {
			deps.Sink = &eventSinkAdapter{bus: h.bus}
		}
		svc, err := agentapp.NewService(deps)
		if err != nil {
			log.Printf("[api] agent service init failed: %v", err)
		} else {
			h.agent = svc
		}
	}
	
	if repos.Data != nil {
		h.data = dataapp.NewService(repos.Data, repos.Job, repos.DataArtifact)
		h.data.SetExportRoot(annotationExportRoot())
	}
	
	if repos.OptimizeRepo != nil && repos.OptimizeExecutor != nil {
		h.compress = optimizeapp.NewService(repos.OptimizeRepo, repos.OptimizeExecutor)
	}
	return h
}

func newBudgetAlert(sink notify.EventSink) func(ctx context.Context, tenantID string, limitCost, usedCost float64) error {
	
	if sink == nil {
		return nil
	}
	if v, ok := sink.(*notify.WebhookNotifier); ok && v == nil {
		return nil
	}
	return func(ctx context.Context, tenantID string, limitCost, usedCost float64) error {
		e := event.NewBudgetThresholdExceeded(tenantID, limitCost, usedCost)
		return sink.Notify(ctx, e)
	}
}

func loadPriceBook(repo ports.PriceRepository) *llm.PriceBook {
	fallback := llm.NewPriceBookFromConfig(os.Getenv)
	if repo == nil {
		return fallback
	}
	book, err := storage.BuildPriceBook(context.Background(), repo, fallback)
	if err != nil {
		log.Printf("[api] load price book from db failed, fallback to env: %v", err)
		return fallback
	}
	return book
}

func (h *Handler) audit(c *gin.Context, action, resType, resID, detail string) {
	if h.auditRepo == nil {
		return
	}
	claims, ok := auth.Principal(c)
	if !ok {
		claims = auth.SyntheticAdmin()
	}
	h.auditAs(c, claims.Username, claims.UserID, claims.Role, claims.TenantID, action, resType, resID, detail)
}

func (h *Handler) auditAs(c *gin.Context, actor, actorID string, role models.Role, tenant, action, resType, resID, detail string) {
	entry := &models.AuditLog{
		Actor:        actor,
		ActorID:      actorID,
		ActorRole:    role,
		TenantID:     tenant,
		Action:       action,
		ResourceType: resType,
		ResourceID:   resID,
		Detail:       detail,
		ClientIP:     c.ClientIP(),
	}
	if err := h.auditRepo.Record(entry); err != nil {
		log.Printf("[audit] record failed: %v", err)
	}
}

func annotationExportRoot() string {
	if root := os.Getenv("ANNOTATION_EXPORT_ROOT"); root != "" {
		return root
	}
	return "data/annotation-exports"
}

func (h *Handler) principalTenant(c *gin.Context) string {
	if claims, ok := auth.Principal(c); ok && claims.TenantID != "" {
		return claims.TenantID
	}
	return auth.DefaultTenantID
}

func (h *Handler) principalTenantStrict(c *gin.Context) (string, bool) {
	if claims, ok := auth.Principal(c); ok && claims.TenantID != "" {
		return claims.TenantID, true
	}
	return "", false
}

func (h *Handler) requireWriteTenant(c *gin.Context) (string, bool) {
	return h.principalTenantStrict(c)
}

func (h *Handler) claimsOf(c *gin.Context) *auth.Claims {
	if claims, ok := auth.Principal(c); ok {
		return claims
	}
	return auth.SyntheticAdmin()
}

func (h *Handler) isPlatformAdmin(c *gin.Context) bool {
	return h.claimsOf(c).Role == models.RolePlatformAdmin
}

func (h *Handler) canAccessTenant(ownerTenant string, c *gin.Context) bool {
	if h.isPlatformAdmin(c) {
		return true
	}
	return h.claimsOf(c).TenantID == ownerTenant
}

func (h *Handler) tenantScope(c *gin.Context) string {
	if h.isPlatformAdmin(c) {
		return ""
	}
	return h.principalTenant(c)
}