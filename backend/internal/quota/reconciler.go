
package quota

import (
	"context"
	"log"

	"fuze-ai-paas/backend/internal/models"
	"fuze-ai-paas/backend/internal/ports"
)

type Reconciler struct {
	jobs   ports.JobRepository
	quotas ports.QuotaRepository
}

func NewReconciler(jobs ports.JobRepository, quotas ports.QuotaRepository) *Reconciler {
	return &Reconciler{jobs: jobs, quotas: quotas}
}

func (r *Reconciler) Reconcile(ctx context.Context) {
	quotas, err := r.quotas.ListQuotas()
	if err != nil {
		log.Printf("[quota] reconcile list quotas failed: %v", err)
		return
	}
	jobs, err := r.jobs.GetActiveJobs()
	if err != nil {
		log.Printf("[quota] reconcile list jobs failed: %v", err)
		return
	}

	used := make(map[string]*models.Quota, len(quotas))
	for _, q := range quotas {
		
		used[q.TenantID] = &models.Quota{
			TenantID:      q.TenantID,
			GPUQuota:      q.GPUQuota,
			MemoryQuotaGB: q.MemoryQuotaGB,
			JobQuota:      q.JobQuota,
		}
	}
	for i := range jobs {
		j := &jobs[i]
		
		if j.IsTerminal() {
			continue
		}
		rv := used[j.TenantID]
		if rv == nil {
			rv = &models.Quota{TenantID: j.TenantID}
			used[j.TenantID] = rv
		}
		
		rv.GPUUsed += j.TotalGPUs()
		rv.MemoryUsedGB += j.TotalMemory()
		rv.JobUsed++
	}

	for _, rv := range used {
		if err := r.quotas.UpsertQuota(rv); err != nil {
			log.Printf("[quota] reconcile upsert failed for tenant %s: %v", rv.TenantID, err)
		}
	}
}