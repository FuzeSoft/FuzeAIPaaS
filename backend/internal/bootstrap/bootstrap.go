
package bootstrap

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"fuze-ai-paas/backend/internal/api"
	edgeapp "fuze-ai-paas/backend/internal/app/edge"
	token "fuze-ai-paas/backend/internal/app/token"
	"fuze-ai-paas/backend/internal/auth"
	"fuze-ai-paas/backend/internal/crypto/aes"
	"fuze-ai-paas/backend/internal/domain/agent"
	domainedge "fuze-ai-paas/backend/internal/domain/edge"
	domainevent "fuze-ai-paas/backend/internal/domain/event"
	"fuze-ai-paas/backend/internal/domain/optimize"
	"fuze-ai-paas/backend/internal/events"
	"fuze-ai-paas/backend/internal/k8s"
	edgek8s "fuze-ai-paas/backend/internal/k8s/edge"
	optimizek8s "fuze-ai-paas/backend/internal/k8s/optimize"
	workspacek8s "fuze-ai-paas/backend/internal/k8s/workspace"
	"fuze-ai-paas/backend/internal/llmgw"
	"fuze-ai-paas/backend/internal/llmjudge"
	"fuze-ai-paas/backend/internal/metricsquery"
	"fuze-ai-paas/backend/internal/models"
	"fuze-ai-paas/backend/internal/notify"
	"fuze-ai-paas/backend/internal/ports"
	"fuze-ai-paas/backend/internal/quota"
	"fuze-ai-paas/backend/internal/runtime"
	"fuze-ai-paas/backend/internal/scheduler"
	"fuze-ai-paas/backend/internal/storage"
	"fuze-ai-paas/backend/internal/storage/artifact"
	"fuze-ai-paas/backend/internal/telemetry"

	"github.com/gin-gonic/gin"
)

type Config struct {
	DBPath      string
	Port        string
	AuthEnabled bool
	WebhookURL  string
	SSO         api.SSOConfig
	
	WorkspaceProxyBaseURL string
}

func LoadConfig() Config {
	cfg := Config{
		DBPath:                getEnv("DB_PATH", "./fuze-ai-paas.db"),
		Port:                  getEnv("PORT", "8080"),
		AuthEnabled:           os.Getenv("AUTH_ENABLED") == "true",
		WebhookURL:            os.Getenv("EVENT_WEBHOOK_URL"),
		WorkspaceProxyBaseURL: getEnv("WORKSPACE_PROXY_BASE_URL", ""),
		SSO: api.SSOConfig{
			FrontendURL: getEnv("FRONTEND_URL", "http://localhost:3000"),
			
			SecureCookie: cfgSecureCookie(),
		},
	}
	
	return cfg
}

type Server struct {
	Router  *gin.Engine
	Handler *api.Handler
	Sched   *scheduler.Scheduler
	Store   *storage.Storage
	Bus     *events.Bus
	Tele    *telemetry.Collector
	
	Artifacts ports.ArtifactStore
	Port      string

	http *http.Server
}

func NewServer(cfg Config) (*Server, error) {
	db, err := storage.NewDBFromEnvWithPath(os.Getenv, cfg.DBPath)
	if err != nil {
		return nil, fmt.Errorf("init db: %w", err)
	}
	store := storage.NewStorage(db)

	var storeCipher *aes.Cipher
	if key, kerr := aes.LoadMasterKey(os.Getenv); kerr != nil {
		log.Printf("[Bootstrap] static encryption disabled (no KUBECONFIG_ENC_KEY): %v", kerr)
	} else {
		storeCipher = aes.NewCipher(key)
		store.SetCipher(storeCipher)
		log.Printf("[Bootstrap] static encryption enabled for sensitive credentials")
	}

	cfg.SSO.Registry = storage.NewIdPRegistry(db, storeCipher)

	artifacts, err := artifact.NewFromEnv(os.Getenv)
	if err != nil {
		return nil, fmt.Errorf("init artifact store: %w", err)
	}

	clusterMgr := k8s.NewClusterManager()
	if clusters, cerr := store.GetEnabledClusters(); cerr == nil {
		if errs := clusterMgr.LoadAll(clusters); len(errs) > 0 {
			for _, e := range errs {
				log.Printf("[Bootstrap] %v", e)
			}
		}
	}
	log.Printf("[Bootstrap] managed clusters loaded: %d", len(clusterMgr.List()))

	authMgr := auth.NewManagerForEnv(cfg.AuthEnabled)

	tokenRepo := storage.NewTokenRepository(db)
	authMgr.SetTokenTouch(func(ctx context.Context, jti string) {
		if err := token.NewService(authMgr, tokenRepo).Touch(ctx, jti); err != nil {
			log.Printf("[Auth] token touch failed for jti %s: %v", jti, err)
		}
	})

	authMgr.SetRevokedCheck(func(jti string) bool {
		ok, err := tokenRepo.IsBlacklisted(context.Background(), jti)
		if err != nil {
			log.Printf("[Auth] blacklist check failed for %s: %v", jti, err)
			return false
		}
		return ok
	})

	if key, kerr := aes.LoadMasterKey(os.Getenv); kerr == nil {
		mfaSvc := auth.NewMFAService(aes.NewCipher(key))
		authMgr.SetMFA(mfaSvc)
		
		authMgr.SetRevokeAdd(func(jti string) {
			if err := tokenRepo.BlacklistJTI(context.Background(), jti, "mfa temp token consumed"); err != nil {
				log.Printf("[Auth] mfa temp token blacklist failed: %v", err)
			}
		})
		log.Printf("[Bootstrap] MFA service enabled")

		rpID := getEnv("WEBAUTHN_RPID", rpIDFromURL(cfg.SSO.FrontendURL))
		rpName := getEnv("WEBAUTHN_RP_NAME", "FuzeAI PaaS")
		rpOrigins := strings.Split(getEnv("WEBAUTHN_RP_ORIGINS", cfg.SSO.FrontendURL), ",")
		if pkSvc, perr := auth.NewWebAuthnService(rpID, rpName, rpOrigins, aes.NewCipher(key)); perr == nil {
			authMgr.SetPasskey(pkSvc)
			log.Printf("[Bootstrap] Passkey (WebAuthn) service enabled: rpID=%s origins=%v", rpID, rpOrigins)
		} else {
			log.Printf("[Bootstrap] Passkey disabled: %v", perr)
		}
	} else {
		log.Printf("[Bootstrap] MFA disabled (no KUBECONFIG_ENC_KEY): %v", kerr)
	}

	authMaxFails := atoiDefault(os.Getenv("AUTH_MAX_FAILS"), 5)
	authLockSec := atoiDefault(os.Getenv("AUTH_LOCK_SEC"), 900)
	authMgr.SetLoginLimit(authMaxFails, authLockSec)
	log.Printf("[Bootstrap] login rate limit: maxFails=%d lockSec=%d", authMaxFails, authLockSec)

	eventBus := events.NewBus(1024, 8)
	tele := telemetry.NewCollector()
	qr := quota.NewReconciler(store, store)
	notifier := notify.NewWebhookNotifier(cfg.WebhookURL)
	subscribeDownstream(eventBus, tele, qr, notifier, store)

	sched := scheduler.NewScheduler(scheduler.Repos{
		Cluster:   store,
		Job:       store,
		Inference: store,
		Dataset:   store,
		Data:      store,
		Metrics:   store,
	}, clusterMgr, runtime.NewDefaultRegistry(), eventBus)

	k8sClient := k8s.NewClient(k8s.DefaultNamespace)

	router := gin.Default()
	
	if tp := os.Getenv("GIN_TRUSTED_PROXIES"); tp != "" {
		if err := router.SetTrustedProxies(strings.Split(tp, ",")); err != nil {
			log.Printf("[Bootstrap] invalid GIN_TRUSTED_PROXIES %q: %v", tp, err)
		}
	} else {
		_ = router.SetTrustedProxies([]string{"0.0.0.0/0", "::/0"})
	}
	
	var metricsQ ports.MetricsQuery = metricsquery.NewNoop()
	prometheusWired := false
	if promURL := os.Getenv("PROMETHEUS_URL"); promURL != "" {
		if pq, perr := metricsquery.NewPrometheus(metricsquery.PrometheusConfig{BaseURL: promURL}); perr == nil {
			metricsQ = pq
			prometheusWired = true
		} else {
			log.Printf("[Bootstrap] prometheus init failed, fallback noop: %v", perr)
		}
	}
	
	var optimizeExecutor optimize.CompressionExecutor
	if k8sClient.Enabled() {
		optimizeExecutor = optimizek8s.NewExecutor(
			optimizek8s.NewDynamicJobSubmitter(k8sClient.Dynamic(), k8sClient.Namespace()),
			parseOptimizeImages(os.Getenv("OPTIMIZE_BACKEND_IMAGES")),
		)
		log.Printf("[Bootstrap] inference acceleration (model compression) executor enabled")
	} else {
		log.Printf("[Bootstrap] inference acceleration executor disabled (no cluster): optimize endpoints degrade to 501")
	}
	optimizeWired := k8sClient.Enabled() && optimizeExecutor != nil
	edgeWired := false

	llmCompleter := llmgw.NewCompleterFromOS()

	apiHandler := api.NewHandler(api.Repos{
		Cluster:      store,
		Job:          store,
		Inference:    store,
		Resource:     store,
		Model:        store,
		Dataset:      store,
		Tenant:       store,
		Quota:        store,
		Audit:        store,
		User:         store,
		Experiment:   storage.NewExperimentRepository(db),
		MetricsQuery: metricsQ,
		Token:        tokenRepo,
		Evaluation:   storage.NewEvaluationRepository(db),
		
		Judge: llmjudge.NewFromOS(),
		
		Route:      storage.NewRouteRepository(db),
		TokenQuota: storage.NewTokenQuotaRepository(db),
		TokenUsage: storage.NewTokenUsageRepository(db),
		Trace:      storage.NewTraceRepository(db),
		Prompt:     storage.NewPromptRepository(db),
		Knowledge:  storage.NewKnowledgeRepository(db),
		
		Study:     storage.NewStudyRepository(db),
		Trial:     storage.NewTrialRepository(db),
		HPOReport: storage.NewReportRepository(db),
		
		FineTune: storage.NewFineTuneRepository(db),
		
		AdapterMounts: storage.NewAdapterMountRepository(db),
		
		Guardrail: storage.NewGuardrailRepository(db),
		JudgeLLM:  llmjudge.NewFromOS(),
		
		VectorStore: llmgw.NewVectorStoreRegistry(llmgw.VectorStoreConfig{EmbedDim: 256}, db).Store(),
		Embedder:    llmgw.NewHashEmbedder(256),
		
		PriceRepo: storage.NewPriceRepository(db),
		
		CostRepo: storage.NewCostRepository(db),
		
		Workspace:         store,
		WorkspaceRT:       workspacek8s.NewDriver(k8sClient, store, cfg.WorkspaceProxyBaseURL),
		WorkspaceNotifier: wsNotifierOf(notifier),
		
		Alert: storage.NewAlertRepository(db),
		
		Agent: storage.NewAgentRepository(db),
		
		ToolRegistry: storage.NewToolRepository(db),
		
		Data:         store,
		DataArtifact: nil,
		
		OptimizeRepo:     storage.NewCompressionRepository(db),
		OptimizeExecutor: optimizeExecutor,
		
		LLMCompleter: llmCompleter,
		
		BudgetAlert: budgetAlertSink(os.Getenv("BUDGET_WEBHOOK_URL")),
	}, sched, clusterMgr, authMgr, eventBus, cfg.SSO)
	apiHandler.SetTelemetry(tele)

	{
		nodeRepo, deployRepo, driftRepo, labelRepo := store.Edge()
		var edgeRuntime domainedge.EdgeRuntime = edgek8s.NewMockRuntime()
		switch os.Getenv("EDGE_RUNTIME") {
		case "agent":
			
			caPEM := os.Getenv("EDGE_CA_PEM")
			clientPEM := os.Getenv("EDGE_CLIENT_PEM")
			clientKey := os.Getenv("EDGE_CLIENT_KEY")
			if caPEM != "" && clientPEM != "" && clientKey != "" {
				if rt, err := edgek8s.NewAgentRuntime(caPEM, clientPEM, clientKey); err == nil {
					edgeRuntime = rt
				} else {
					log.Printf("[Bootstrap] edge agent runtime init failed, fallback to mock: %v", err)
				}
			}
		case "kubeedge":
			
			cloudHub := os.Getenv("EDGE_CLOUDHUB_URL")
			token := os.Getenv("EDGE_CLOUDHUB_TOKEN")
			ns := os.Getenv("EDGE_NAMESPACE")
			caPEM := os.Getenv("EDGE_CA_PEM")
			if cloudHub != "" && token != "" {
				if rt, err := edgek8s.NewKubeEdgeRuntime(edgek8s.KubeEdgeConfig{
					CloudHub:  cloudHub,
					Token:     token,
					Namespace: ns,
					CACertPEM: caPEM,
				}); err == nil {
					edgeRuntime = rt
				} else {
					log.Printf("[Bootstrap] edge kubeedge runtime init failed, fallback to mock: %v", err)
				}
			}
		}
		edgeSvc := edgeapp.NewService(nodeRepo, deployRepo, driftRepo, edgeRuntime, nil, nil,
			labelRepo, edgeapp.Config{OfflineThreshold: 2 * time.Minute},
			newEdgeBusPublisher(eventBus), metricsQ, edgeapp.SampleMetricNames{})
		apiHandler.SetEdge(edgeSvc)
		edgeWired = true
	}
	api.RegisterRoutes(router, apiHandler, authMgr, cfg.SSO, cfg.AuthEnabled, clusterMgr)

	verifyWiring(WiringReport{
		LLMBaseURL:    os.Getenv("LLM_BASE_URL"),
		PrometheusURL: os.Getenv("PROMETHEUS_URL"),
		EdgeRuntime:   os.Getenv("EDGE_RUNTIME"),
		IsCluster:     k8sClient.Enabled(),
		LLMCompleter:  llmCompleter != nil,
		Prometheus:    prometheusWired,
		Edge:          edgeWired,
		Optimize:      optimizeWired,
	})

	return &Server{
		Router:    router,
		Handler:   apiHandler,
		Sched:     sched,
		Store:     store,
		Bus:       eventBus,
		Tele:      tele,
		Artifacts: artifacts,
		Port:      cfg.Port,
	}, nil
}

func (s *Server) Run() error {
	ctx := context.Background()
	s.Bus.Run(ctx)
	s.Sched.Run(ctx)

	s.http = &http.Server{Addr: ":" + s.Port, Handler: s.Router}
	log.Printf("[Bootstrap] server listening on :%s", s.Port)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-stop
		log.Println("[Bootstrap] received shutdown signal")
		s.Sched.Stop()
		s.Bus.Stop()
		s.Handler.StopAuth()
		s.Handler.StopSSO()
		_ = s.Shutdown(context.Background())
	}()

	if err := s.http.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func (s *Server) Shutdown(ctx context.Context) error {
	if s.http != nil {
		return s.http.Shutdown(ctx)
	}
	return nil
}

func subscribeDownstream(bus *events.Bus, tele *telemetry.Collector, qr *quota.Reconciler, notifier *notify.WebhookNotifier, auditRepo ports.AuditRepository) {
	bus.Subscribe(domainevent.ClusterDiscoveredType, func(ctx context.Context, e domainevent.Event) {
		ev, ok := e.(domainevent.ClusterDiscovered)
		if !ok {
			return
		}
		tele.RecordCluster(ev)
		if notifier != nil {
			_ = notifier.Notify(ctx, e)
		}
	})
	bus.Subscribe(domainevent.JobSubmittedType, func(ctx context.Context, e domainevent.Event) {
		ev, ok := e.(domainevent.JobSubmitted)
		if !ok {
			return
		}
		tele.RecordSubmit(ev)
		qr.Reconcile(ctx) 
		if notifier != nil {
			_ = notifier.Notify(ctx, e)
		}
	})
	bus.Subscribe(domainevent.AssignmentCompletedType, func(ctx context.Context, e domainevent.Event) {
		ev, ok := e.(domainevent.AssignmentCompleted)
		if !ok {
			return
		}
		tele.RecordAssign(ev)
		qr.Reconcile(ctx) 
		if notifier != nil {
			_ = notifier.Notify(ctx, e)
		}
	})
	
	bus.Subscribe(domainevent.JobStateChangedType, func(ctx context.Context, e domainevent.Event) {
		ev, ok := e.(domainevent.JobStateChanged)
		if !ok || !ev.Terminal {
			return
		}
		qr.Reconcile(ctx)
		if notifier != nil {
			_ = notifier.Notify(ctx, e)
		}
	})
	
	bus.Subscribe(domainevent.AgentRunFinishedType, func(ctx context.Context, e domainevent.Event) {
		ev, ok := e.(domainevent.AgentRunFinished)
		if !ok {
			return
		}
		if auditRepo == nil {
			return
		}
		out := ev.FinalOutput
		if len(out) > 512 {
			out = out[:512] + "...(truncated)"
		}
		detail := fmt.Sprintf("status=%s nodes=%d duration_ms=%d output=%q", ev.Status, ev.NodeCount, ev.DurationMs, out)
		if ev.Error != "" {
			detail += " error=" + ev.Error
		}
		record := &models.AuditLog{
			TenantID:     ev.TenantID,
			ActorID:      ev.ActorID,
			Actor:        ev.ActorName,
			Action:       models.ActionRunFinish,
			ResourceType: models.ResAgent,
			ResourceID:   ev.AgentID,
			Detail:       detail,
			CreatedAt:    time.Now(),
		}
		if ev.Status == agent.RunFailed {
			record.Detail = "WARNING: " + record.Detail
		}
		if err := auditRepo.Record(record); err != nil {
			log.Printf("[audit] AgentRunFinished persist failed run=%s: %v", ev.RunID, err)
		}
	})
}

func getEnv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func cfgSecureCookie() bool {
	if v := os.Getenv("COOKIE_SECURE"); v != "" {
		return v == "true"
	}
	return os.Getenv("AUTH_ENABLED") == "true"
}

func splitList(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func parseOptimizeImages(raw string) map[optimize.CompressionBackend]string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	out := map[optimize.CompressionBackend]string{}
	for _, kv := range strings.Split(raw, ",") {
		kv = strings.TrimSpace(kv)
		if kv == "" {
			continue
		}
		idx := strings.IndexByte(kv, '=')
		if idx <= 0 || idx >= len(kv)-1 {
			log.Printf("[Bootstrap] invalid OPTIMIZE_BACKEND_IMAGES entry %q, skip", kv)
			continue
		}
		out[optimize.CompressionBackend(kv[:idx])] = kv[idx+1:]
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func budgetAlertSink(url string) notify.EventSink {
	if url == "" {
		return nil
	}
	return notify.NewWebhookNotifier(url)
}

func wsNotifierOf(n *notify.WebhookNotifier) notify.EventSink {
	if n == nil {
		return nil
	}
	return n
}

func atoiDefault(s string, def int) int {
	if s == "" {
		return def
	}
	if v, err := strconv.Atoi(s); err == nil {
		return v
	}
	return def
}

type edgeBusPublisher struct{ bus *events.Bus }

func newEdgeBusPublisher(bus *events.Bus) *edgeBusPublisher { return &edgeBusPublisher{bus: bus} }

func (p *edgeBusPublisher) Publish(e domainedge.EdgeEvent) {
	p.bus.Publish(edgeEventAdapter{e})
}

type edgeEventAdapter struct{ inner domainedge.EdgeEvent }

func (a edgeEventAdapter) EventType() string {
	if a.inner == nil {
		return "edge.unknown"
	}
	return a.inner.EventTopic()
}

func (a edgeEventAdapter) OccurredAt() time.Time {
	switch e := a.inner.(type) {
	case domainedge.DriftDetected:
		return e.EvaluatedAt
	case domainedge.DeploymentRolledBack:
		return e.At
	default:
		return time.Time{}
	}
}

func (a edgeEventAdapter) AggregateID() string {
	switch e := a.inner.(type) {
	case domainedge.DriftDetected:
		return e.DeploymentID
	case domainedge.DeploymentRolledBack:
		return e.DeploymentID
	default:
		return ""
	}
}

func rpIDFromURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "localhost"
	}
	host := u.Host
	if i := strings.IndexByte(host, ':'); i >= 0 {
		host = host[:i]
	}
	if host == "" {
		return "localhost"
	}
	return host
}

type WiringReport struct {
	LLMBaseURL    string 
	PrometheusURL string 
	EdgeRuntime   string 
	IsCluster     bool   

	LLMCompleter bool 
	Prometheus   bool 
	Edge         bool 
	Optimize     bool 
}

func verifyWiring(r WiringReport) {
	checks := []struct {
		switchOn bool
		wired    bool
		name     string
		hint     string
	}{
		{r.LLMBaseURL != "", r.LLMCompleter, "llm-gateway",
			"设置了 LLM_BASE_URL 但 LLMCompleter 未装配（检查 LLM_API_KEY 与网关初始化）"},
		{r.PrometheusURL != "", r.Prometheus, "prometheus-metrics",
			"设置了 PROMETHEUS_URL 但指标查询未装配（Prometheus 初始化失败？）"},
		{r.EdgeRuntime == "agent" || r.EdgeRuntime == "kubeedge", r.Edge, "edge-runtime",
			"设置了 EDGE_RUNTIME 但边缘服务未装配（检查指标查询与标签反馈仓储）"},
		{r.IsCluster, r.Optimize, "inference-acceleration",
			"已启用集群（K8s）但推理加速执行器未装配"},
		{r.Edge && !r.Prometheus, true, "edge-drift-metrics",
			"已装配边缘服务但未装配 Prometheus 指标查询（漂移检测的样本源不可用，手动/周期漂移检测将降级）"},
	}
	ok := true
	for _, c := range checks {
		switch {
		case c.switchOn && !c.wired:
			log.Printf("[Bootstrap][WiringCheck][WARN] 开关开启但未装配: %s — %s", c.name, c.hint)
			ok = false
		case !c.switchOn && c.wired:
			log.Printf("[Bootstrap][WiringCheck][WARN] 开关关闭却已装配: %s — 确认是否为预期默认行为", c.name)
			ok = false
		}
	}
	if ok {
		log.Printf("[Bootstrap][WiringCheck] OK: 特性开关与装配一致")
	}
}