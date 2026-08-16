
package job

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"fuze-ai-paas/backend/internal/adapter"
	domainevent "fuze-ai-paas/backend/internal/domain/event"
	jobdomain "fuze-ai-paas/backend/internal/domain/job"
	"fuze-ai-paas/backend/internal/events"
	"fuze-ai-paas/backend/internal/models"
	"fuze-ai-paas/backend/internal/ports"
)

const (
	
	maxSubmitAttempts = 5

	submitBackoffBase = 5 * time.Second

	submitBackoffMax = 5 * time.Minute

	clusterOpTimeout = 30 * time.Second

	orphanConfirmThreshold = 3

	maxFailureReasonLen = 500
)

type Service struct {
	jobRepo    ports.JobRepository
	clusterMgr ports.ClusterRegistry
	bus        *events.Bus

	mu sync.Mutex
	
	missingSyncs map[string]int

	submitMu sync.Mutex
	
	submitLocks map[string]*jobSubmitLock
}

type jobSubmitLock struct {
	mu   sync.Mutex
	refs int
}

func NewService(jobRepo ports.JobRepository, clusterMgr ports.ClusterRegistry, bus *events.Bus) *Service {
	return &Service{
		jobRepo:      jobRepo,
		clusterMgr:   clusterMgr,
		bus:          bus,
		missingSyncs: make(map[string]int),
		submitLocks:  make(map[string]*jobSubmitLock),
	}
}

func (svc *Service) lockSubmit(jobID string) func() {
	svc.submitMu.Lock()
	l, ok := svc.submitLocks[jobID]
	if !ok {
		l = &jobSubmitLock{}
		svc.submitLocks[jobID] = l
	}
	l.refs++
	svc.submitMu.Unlock()

	l.mu.Lock()
	return func() {
		l.mu.Unlock()
		svc.submitMu.Lock()
		l.refs--
		if l.refs == 0 {
			delete(svc.submitLocks, jobID)
		}
		svc.submitMu.Unlock()
	}
}

func (svc *Service) SyncVolcanoStatuses(ctx context.Context) {
	jobs, err := svc.jobRepo.GetActiveJobs()
	if err != nil {
		log.Printf("[Job] Failed to list active jobs: %v", err)
		return
	}

	byCluster := make(map[string][]*models.Job)
	for i := range jobs {
		job := &jobs[i]
		
		if job.VolcanoJobName == "" {
			continue
		}
		byCluster[job.ClusterID] = append(byCluster[job.ClusterID], job)
	}

	for clusterID, clusterJobs := range byCluster {
		client, err := svc.clusterMgr.Get(clusterID)
		if err != nil || !client.Enabled() {
			
			continue
		}

		statuses, err := svc.syncClusterStatuses(ctx, client)
		if err != nil {
			log.Printf("[Job] Failed to sync volcano job statuses for cluster %s: %v", clusterID, err)
			continue
		}

		updated := 0
		for _, job := range clusterJobs {
			volcanoState, exists := statuses[job.VolcanoJobName]
			if !exists {
				if svc.handleMissingJob(job) {
					updated++
				}
				continue
			}
			svc.resetMissingCount(job.ID)
			if svc.applyVolcanoState(job, volcanoState) {
				updated++
			}
		}
		if updated > 0 {
			log.Printf("[Job] Synced %d job statuses from cluster %s", updated, clusterID)
		}
	}

	svc.enforceRuntimeLimits()
}

func (svc *Service) syncClusterStatuses(ctx context.Context, client ports.ClusterClientPort) (map[string]models.JobState, error) {
	callCtx, cancel := context.WithTimeout(ctx, clusterOpTimeout)
	defer cancel()
	return client.SyncJobStatuses(callCtx)
}

func (svc *Service) applyVolcanoState(job *models.Job, volcanoState models.JobState) bool {
	agg := adapter.JobFromModel(job)
	if !agg.ApplyVolcanoState(adapter.JobStateFromModel(volcanoState)) {
		
		return false
	}
	adapter.JobSyncToModel(agg, job)
	
	if job.Status == models.JobStatusRunning && job.StartedAt == nil {
		now := time.Now()
		job.StartedAt = &now
	}
	if job.Status == models.JobStatusFailed && job.FailureReason == "" {
		job.FailureReason = truncateReason(fmt.Sprintf("volcano job entered state %q", volcanoState))
	}
	if job.Status == models.JobStatusCompleted {
		job.FailureReason = ""
	}
	if err := svc.jobRepo.UpdateJobStatus(job); err != nil {
		log.Printf("[Job] Failed to update job %s status: %v", job.ID, err)
		return false
	}
	svc.onStateChanged(job)
	return true
}

func (svc *Service) handleMissingJob(job *models.Job) bool {
	if svc.bumpMissingCount(job.ID) < orphanConfirmThreshold {
		return false
	}
	
	svc.expireJob(job, fmt.Sprintf(
		"volcano job %q no longer exists in cluster %s (deleted externally or garbage collected)",
		job.VolcanoJobName, job.ClusterID))
	svc.resetMissingCount(job.ID)
	return true
}

func (svc *Service) bumpMissingCount(jobID string) int {
	svc.mu.Lock()
	defer svc.mu.Unlock()
	svc.missingSyncs[jobID]++
	return svc.missingSyncs[jobID]
}

func (svc *Service) resetMissingCount(jobID string) {
	svc.mu.Lock()
	defer svc.mu.Unlock()
	delete(svc.missingSyncs, jobID)
}

func (svc *Service) onStateChanged(job *models.Job) {
	if svc.bus == nil {
		return
	}
	svc.bus.Publish(domainevent.NewJobStateChanged(
		job.ID, job.ClusterID, string(job.Status), job.TenantID, job.IsTerminal()))
}

func (svc *Service) markRunning(agg *jobdomain.Job, job *models.Job) bool {
	if !agg.MarkRunning() {
		return false
	}
	adapter.JobSyncToModel(agg, job)
	if job.Status == models.JobStatusRunning && job.StartedAt == nil {
		now := time.Now()
		job.StartedAt = &now
	}
	return true
}

func (svc *Service) expireJob(job *models.Job, reason string) {
	if job.VolcanoJobName != "" {
		if client, err := svc.clusterMgr.Get(job.ClusterID); err == nil && client.Enabled() {
			ctx, cancel := context.WithTimeout(context.Background(), clusterOpTimeout)
			if delErr := client.DeleteVolcanoJob(ctx, job.VolcanoJobName); delErr != nil {
				log.Printf("[Job] expireJob failed to delete cluster workload %s: %v", job.VolcanoJobName, delErr)
			}
			cancel()
		}
		job.VolcanoJobName = "" 
	}
	agg := adapter.JobFromModel(job)
	if !agg.MarkFailed() {
		return
	}
	adapter.JobSyncToModel(agg, job)
	job.FailureReason = truncateReason(reason)
	if err := svc.jobRepo.UpdateJobStatus(job); err != nil {
		log.Printf("[Job] expireJob failed to persist job %s: %v", job.ID, err)
		return
	}
	svc.onStateChanged(job) 
}

func (svc *Service) enforceRuntimeLimits() {
	jobs, err := svc.jobRepo.GetActiveJobs()
	if err != nil {
		log.Printf("[Job] enforceRuntimeLimits list failed: %v", err)
		return
	}
	now := time.Now()
	for i := range jobs {
		job := &jobs[i]
		if job.Status != models.JobStatusRunning || job.MaxRuntime <= 0 || job.StartedAt == nil {
			continue
		}
		if now.Sub(*job.StartedAt) > time.Duration(job.MaxRuntime)*time.Minute {
			svc.expireJob(job, fmt.Sprintf("job exceeded max runtime of %d minutes", job.MaxRuntime))
		}
	}
}

func (svc *Service) GetJobLogs(job *models.Job, query ports.LogQuery) (ports.JobLogs, bool, error) {
	if job.VolcanoJobName == "" {
		return ports.JobLogs{}, false, nil
	}
	client, err := svc.clusterMgr.Get(job.ClusterID)
	if err != nil || !client.Enabled() {
		return ports.JobLogs{}, false, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), clusterOpTimeout)
	defer cancel()
	result, err := client.GetJobLogs(ctx, job, query)
	if err != nil {
		
		return result, true, err
	}
	return result, true, nil
}

func (svc *Service) SubmitPending(ctx context.Context) {
	jobs, err := svc.jobRepo.GetPendingJobs()
	if err != nil {
		log.Printf("[Job] Failed to get pending jobs: %v", err)
		return
	}
	if len(jobs) == 0 {
		return
	}

	for i := range jobs {
		job := &jobs[i]

		if job.VolcanoJobName != "" {
			continue
		}

		if !svc.backoffElapsed(job) {
			continue
		}

		unlock := svc.lockSubmit(job.ID)
		fresh, err := svc.jobRepo.GetJob(job.ID)
		if err != nil || fresh == nil {
			unlock()
			continue
		}
		if fresh.VolcanoJobName != "" {
			unlock()
			continue 
		}

		client, err := svc.clusterMgr.Get(fresh.ClusterID)
		if err != nil || !client.Enabled() {
			
			if svc.MockAllocate(fresh) {
				svc.publishSubmitted(fresh)
			}
			unlock()
			continue
		}

		if err := svc.submitToCluster(ctx, client, fresh); err != nil {
			svc.recordSubmitFailure(fresh, err)
			unlock()
			continue
		}
		unlock()
		log.Printf("[Job] Job %s submitted to cluster %s as Volcano Job: %s",
			fresh.ID, fresh.ClusterID, fresh.VolcanoJobName)
		svc.publishSubmitted(fresh)
	}
}

func (svc *Service) submitToCluster(ctx context.Context, client ports.ClusterClientPort, job *models.Job) error {
	createCtx, cancel := context.WithTimeout(ctx, clusterOpTimeout)
	defer cancel()

	vjName, err := client.CreateVolcanoJob(createCtx, job)
	if err != nil {
		return err
	}

	job.VolcanoJobName = vjName
	job.QueueName = models.JobTypeToQueue[job.Type]
	job.Status = models.JobStatusPending
	job.FailureReason = ""
	if err := svc.jobRepo.UpdateJobStatus(job); err != nil {
		job.VolcanoJobName = ""
		svc.deleteClusterJob(ctx, client, vjName)
		return fmt.Errorf("persist volcano job name failed (cluster object rolled back): %w", err)
	}
	return nil
}

func (svc *Service) deleteClusterJob(ctx context.Context, client ports.ClusterClientPort, name string) {
	delCtx, cancel := context.WithTimeout(ctx, clusterOpTimeout)
	defer cancel()
	if err := client.DeleteVolcanoJob(delCtx, name); err != nil {
		log.Printf("[Job] CRITICAL: orphaned volcano job %s could not be cleaned up: %v", name, err)
	}
}

func (svc *Service) recordSubmitFailure(job *models.Job, cause error) {
	job.SubmitAttempts++
	job.FailureReason = truncateReason(cause.Error())
	job.LastSubmitAt = time.Now()

	if job.SubmitAttempts >= maxSubmitAttempts {
		agg := adapter.JobFromModel(job)
		if agg.MarkFailed() {
			adapter.JobSyncToModel(agg, job)
		}
		log.Printf("[Job] Job %s failed permanently after %d submit attempts: %v",
			job.ID, job.SubmitAttempts, cause)
	} else {
		log.Printf("[Job] Failed to submit job %s to cluster %s (attempt %d/%d): %v",
			job.ID, job.ClusterID, job.SubmitAttempts, maxSubmitAttempts, cause)
	}

	if err := svc.jobRepo.UpdateJobStatus(job); err != nil {
		log.Printf("[Job] Failed to persist submit failure for job %s: %v", job.ID, err)
		return
	}
	if job.IsTerminal() {
		svc.onStateChanged(job)
	}
}

func (svc *Service) backoffElapsed(job *models.Job) bool {
	if job.SubmitAttempts <= 0 {
		return true
	}
	wait := submitBackoffBase << (job.SubmitAttempts - 1)
	if wait > submitBackoffMax || wait <= 0 { 
		wait = submitBackoffMax
	}
	last := job.LastSubmitAt
	if last.IsZero() {
		last = job.UpdatedAt
	}
	return time.Since(last) >= wait
}

func (svc *Service) publishSubmitted(job *models.Job) {
	if svc.bus != nil {
		svc.bus.Publish(domainevent.NewJobSubmitted(job.ID, job.ClusterID, string(job.Type), job.GPUs, job.TenantID))
	}
}

func (svc *Service) MockAllocate(job *models.Job) bool {
	if job == nil {
		return false
	}
	modelResources, err := svc.jobRepo.GetResourcesByCluster(job.ClusterID)
	if err != nil {
		
		log.Printf("[Job] Failed to load resources of cluster %s for job %s: %v", job.ClusterID, job.ID, err)
		return false
	}
	if len(modelResources) == 0 {
		
		agg := adapter.JobFromModel(job)
		if !svc.markRunning(agg, job) {
			return false
		}
		if err := svc.jobRepo.UpdateJobStatus(job); err != nil {
			log.Printf("[Job] Failed to update job %s status: %v", job.ID, err)
			return false
		}
		svc.onStateChanged(job)
		return true
	}
	
	resources := make([]jobdomain.Resource, len(modelResources))
	for i, r := range modelResources {
		resources[i] = adapter.ResourceFromModel(r)
	}
	agg := adapter.JobFromModel(job)
	if !agg.CanSchedule(resources) {
		return false
	}
	log.Printf("[Job] Mock scheduling job %s on cluster %s", job.Name, job.ClusterID)
	if !svc.markRunning(agg, job) {
		return false
	}
	if err := svc.jobRepo.UpdateJobStatus(job); err != nil {
		log.Printf("[Job] Failed to update job status: %v", err)
		return false
	}
	svc.onStateChanged(job)
	
	agg.Allocate(resources)
	for i := range resources {
		m := adapter.ResourceToModel(resources[i])
		if err := svc.jobRepo.UpdateResource(&m); err != nil {
			log.Printf("[Job] Failed to update resource: %v", err)
		}
	}

	if svc.bus != nil {
		allocated := 0
		for i := range resources {
			if resources[i].Status == jobdomain.ResourceStatusAllocated {
				allocated++
			}
		}
		svc.bus.Publish(domainevent.NewAssignmentCompleted(job.ID, job.ClusterID, allocated, job.Memory))
	}
	return true
}

func (svc *Service) SubmitJob(job *models.Job) error {
	if job == nil {
		return fmt.Errorf("nil job: cannot submit")
	}

	unlock := svc.lockSubmit(job.ID)
	defer unlock()

	fresh, err := svc.jobRepo.GetJob(job.ID)
	if err != nil {
		return err
	}
	if fresh == nil {
		return fmt.Errorf("job %s not found", job.ID)
	}
	if fresh.VolcanoJobName != "" {
		
		*job = *fresh
		return nil
	}

	client, err := svc.clusterMgr.Get(fresh.ClusterID)
	if err != nil || !client.Enabled() {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), clusterOpTimeout)
	defer cancel()
	if err := svc.submitToCluster(ctx, client, fresh); err != nil {
		return err
	}
	
	*job = *fresh
	return nil
}

func (svc *Service) CancelJob(job *models.Job) error {
	if job == nil {
		return fmt.Errorf("nil job: cannot cancel")
	}
	if err := svc.TerminateJob(job); err != nil {
		return err
	}

	agg := adapter.JobFromModel(job)
	if !agg.MarkCancelled() {
		
		return nil
	}
	adapter.JobSyncToModel(agg, job)
	job.VolcanoJobName = ""
	if err := svc.jobRepo.UpdateJobStatus(job); err != nil {
		return err
	}
	svc.resetMissingCount(job.ID)
	svc.onStateChanged(job)
	return nil
}

func (svc *Service) TerminateJob(job *models.Job) error {
	if job == nil {
		return fmt.Errorf("nil job: cannot terminate")
	}
	if job.VolcanoJobName == "" {
		return nil
	}
	client, err := svc.clusterMgr.Get(job.ClusterID)
	if err != nil || !client.Enabled() {
		
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), clusterOpTimeout)
	defer cancel()
	if err := client.DeleteVolcanoJob(ctx, job.VolcanoJobName); err != nil {
		return fmt.Errorf("delete volcano job %s in cluster %s: %w", job.VolcanoJobName, job.ClusterID, err)
	}
	return nil
}

func truncateReason(reason string) string {
	if len(reason) <= maxFailureReasonLen {
		return reason
	}
	return reason[:maxFailureReasonLen] + "..."
}