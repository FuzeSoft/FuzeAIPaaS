
package scheduler

import (
	"context"
	"log"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"fuze-ai-paas/backend/internal/app/cluster"
	"fuze-ai-paas/backend/internal/app/dataset"
	"fuze-ai-paas/backend/internal/app/data"
	"fuze-ai-paas/backend/internal/app/inference"
	"fuze-ai-paas/backend/internal/app/job"
	"fuze-ai-paas/backend/internal/app/metrics"
	"fuze-ai-paas/backend/internal/domain/gpu"
	"fuze-ai-paas/backend/internal/events"
	"fuze-ai-paas/backend/internal/models"
	"fuze-ai-paas/backend/internal/ports"
)

const (
	discoverInterval    = 10 * time.Second 
	syncInterval        = 5 * time.Second  
	submitInterval      = 3 * time.Second  
	reconcileInterval   = 5 * time.Second  
	dataSyncInterval     = 5 * time.Second  
	discoverConcurrency = 16               
)

type Scheduler struct {
	clusterSvc   *cluster.Service
	jobSvc       *job.Service
	inferenceSvc *inference.Service
	datasetSvc   *dataset.Service
	dataSvc      *data.Service
	metricsSvc   *metrics.Service
	bus          *events.Bus

	running atomic.Bool
	
	mu     sync.Mutex
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

type Repos struct {
	Cluster   ports.ClusterRepository
	Job       ports.JobRepository
	Inference ports.InferenceRepository
	Dataset   ports.DatasetRepository
	Data      ports.DataRepository
	Metrics   ports.MetricsRepository
}

func NewScheduler(repos Repos, clusterMgr ports.ClusterRegistry, runtimeReg ports.RuntimeRegistry, bus *events.Bus) *Scheduler {
	mode := "mock"
	if len(clusterMgr.List()) > 0 {
		mode = "multi-cluster"
	}
	log.Printf("[Scheduler] Initialized in %s mode (managed clusters: %d)", mode, len(clusterMgr.List()))
	sched := &Scheduler{
		clusterSvc:   cluster.NewService(repos.Cluster, clusterMgr, bus),
		jobSvc:       job.NewService(repos.Job, clusterMgr, bus),
		inferenceSvc: inference.NewService(repos.Inference, clusterMgr, runtimeReg),
		datasetSvc:   dataset.NewService(repos.Dataset, clusterMgr),
		metricsSvc:   metrics.NewService(repos.Metrics),
		bus:          bus,
	}
	
	if repos.Data != nil {
		sched.dataSvc = data.NewService(repos.Data, repos.Job, nil)
	}
	return sched
}

func (s *Scheduler) Run(ctx context.Context) {
	if !s.running.CompareAndSwap(false, true) {
		return
	}
	ctx, cancel := context.WithCancel(ctx)
	
	s.mu.Lock()
	s.cancel = cancel
	s.wg.Add(1)
	s.mu.Unlock()

	discoverTicker := time.NewTicker(discoverInterval)
	syncTicker := time.NewTicker(syncInterval)
	submitTicker := time.NewTicker(submitInterval)
	reconcileTicker := time.NewTicker(reconcileInterval)
	dataSyncTicker := time.NewTicker(dataSyncInterval)

	go func() {
		defer s.wg.Done()
		defer discoverTicker.Stop()
		defer syncTicker.Stop()
		defer submitTicker.Stop()
		defer reconcileTicker.Stop()
		defer dataSyncTicker.Stop()

		log.Printf("[Scheduler] async loop started (discover=%s sync=%s submit=%s reconcile=%s data=%s concurrency=%d)",
			discoverInterval, syncInterval, submitInterval, reconcileInterval, dataSyncInterval, discoverConcurrency)

		s.safeInvoke("discoverAllAsync", func() { s.discoverAllAsync(ctx) })
		s.safeInvoke("syncVolcanoStatuses", func() { s.syncVolcanoStatuses(ctx) })
		s.safeInvoke("submitPendingJobs", func() { s.submitPendingJobs(ctx) })
		s.safeInvoke("ReconcileInference", func() { s.ReconcileInference(ctx) })
		if s.dataSvc != nil {
			s.safeInvoke("syncDataPipelines", func() { s.syncDataPipelines(ctx) })
		}

		for {
			select {
			case <-ctx.Done():
				log.Println("[Scheduler] async loop stopped")
				return
			case <-discoverTicker.C:
				s.safeInvoke("discoverAllAsync", func() { s.discoverAllAsync(ctx) })
			case <-syncTicker.C:
				s.safeInvoke("syncVolcanoStatuses", func() { s.syncVolcanoStatuses(ctx) })
			case <-submitTicker.C:
				s.safeInvoke("submitPendingJobs", func() { s.submitPendingJobs(ctx) })
			case <-reconcileTicker.C:
				s.safeInvoke("ReconcileInference", func() { s.ReconcileInference(ctx) })
			case <-dataSyncTicker.C:
				if s.dataSvc != nil {
					s.safeInvoke("syncDataPipelines", func() { s.syncDataPipelines(ctx) })
				}
			}
		}
	}()
}

func (s *Scheduler) Stop() {
	if !s.running.CompareAndSwap(true, false) {
		return
	}
	
	s.mu.Lock()
	cancel := s.cancel
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	s.wg.Wait()
	log.Println("[Scheduler] async loop fully stopped")
}

func (s *Scheduler) safeInvoke(name string, fn func()) {
	defer func() {
		if r := recover(); r != nil {
			
			log.Printf("[Scheduler] %s panic recovered: %v\n%s", name, r, debug.Stack())
		}
	}()
	fn()
}

func (s *Scheduler) discoverAllAsync(ctx context.Context) {
	ids := s.clusterSvc.List()
	if len(ids) == 0 {
		return
	}
	sem := make(chan struct{}, discoverConcurrency)
	var wg sync.WaitGroup
	for _, id := range ids {
		wg.Add(1)
		sem <- struct{}{}
		go func(cid string) {
			defer wg.Done()
			defer func() { <-sem }()
			
			defer func() {
				if r := recover(); r != nil {
					log.Printf("[Scheduler] cluster %s discovery panic recovered: %v\n%s", cid, r, debug.Stack())
				}
			}()
			if err := s.RefreshCluster(ctx, cid); err != nil {
				log.Printf("[Scheduler] cluster %s discovery skipped: %v", cid, err)
			}
		}(id)
	}
	wg.Wait()
}

func (s *Scheduler) RefreshCluster(ctx context.Context, clusterID string) error {
	return s.clusterSvc.Refresh(ctx, clusterID)
}

func (s *Scheduler) syncVolcanoStatuses(ctx context.Context) { s.jobSvc.SyncVolcanoStatuses(ctx) }

func (s *Scheduler) submitPendingJobs(ctx context.Context) { s.jobSvc.SubmitPending(ctx) }

func (s *Scheduler) syncDataPipelines(ctx context.Context) {
	if s.dataSvc == nil {
		return
	}
	active, err := s.dataSvc.ListActivePipelines()
	if err != nil {
		log.Printf("[Scheduler] list active pipelines skipped: %v", err)
		return
	}
	for _, p := range active {
		if err := s.dataSvc.SyncPipeline(p.ID); err != nil {
			log.Printf("[Scheduler] sync pipeline %s skipped: %v", p.ID, err)
		}
	}
}

func (s *Scheduler) mockAllocate(job *models.Job) bool { return s.jobSvc.MockAllocate(job) }

func (s *Scheduler) GetMetrics(clusterID string) (*models.Metrics, error) {
	return s.metricsSvc.Get(clusterID)
}

func (s *Scheduler) SubmitJob(job *models.Job) error { return s.jobSvc.SubmitJob(job) }

func (s *Scheduler) DataService() *data.Service { return s.dataSvc }

func (s *Scheduler) CancelJob(job *models.Job) error { return s.jobSvc.CancelJob(job) }

func (s *Scheduler) TerminateJob(job *models.Job) error { return s.jobSvc.TerminateJob(job) }

func (s *Scheduler) GetJobLogs(job *models.Job, query ports.LogQuery) (ports.JobLogs, bool, error) {
	return s.jobSvc.GetJobLogs(job, query)
}

func (s *Scheduler) ReconcileInference(ctx context.Context) { s.inferenceSvc.Reconcile(ctx) }

func (s *Scheduler) DeployInferenceService(svc *models.InferenceService) error {
	return s.inferenceSvc.Deploy(svc)
}

func (s *Scheduler) DeleteInferenceService(svc *models.InferenceService) error {
	return s.inferenceSvc.Delete(svc)
}

func (s *Scheduler) UndeployInferenceService(ctx context.Context, svc *models.InferenceService) error {
	return s.inferenceSvc.Undeploy(ctx, svc)
}

func (s *Scheduler) CreateDataset(ds *models.Dataset) error { return s.datasetSvc.Create(ds) }

func (s *Scheduler) DeleteDataset(ds *models.Dataset) error { return s.datasetSvc.Delete(ds) }

func expandNodesToResources(clusterID string, devices []gpu.GPUDevice) []models.Resource {
	return cluster.ExpandNodesToResources(clusterID, devices)
}