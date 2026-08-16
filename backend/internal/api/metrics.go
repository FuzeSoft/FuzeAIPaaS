package api

import (
	"net/http"

	"fuze-ai-paas/backend/internal/metrics"
	"fuze-ai-paas/backend/internal/models"
	"fuze-ai-paas/backend/internal/ports"

	"github.com/gin-gonic/gin"
)

func (h *Handler) BusinessSnapshot() (metrics.BusinessSnapshot, error) {
	snap := metrics.BusinessSnapshot{}

	m, err := h.scheduler.GetMetrics("")
	if err != nil {
		return snap, err
	}
	snap.TotalGPUs = m.TotalGPUs
	snap.UsedGPUs = m.UsedGPUs
	snap.AvailableGPUs = m.AvailableGPUs
	snap.GPUUtilization = m.GPUUtilization
	snap.TotalMemoryGB = m.TotalMemory
	snap.UsedMemoryGB = m.UsedMemory
	snap.MemoryUtilization = m.MemoryUtilization
	snap.RunningJobs = m.RunningJobs
	snap.PendingJobs = m.PendingJobs
	snap.CompletedJobs = m.CompletedJobs
	snap.TotalJobs = m.TotalJobs

	if svcs, err := h.inferenceRepo.GetInferenceServices(); err == nil {
		ready := 0
		var replicas float64
		for _, s := range svcs {
			if s.Status == models.InferenceStatusReady {
				ready++
			}
			replicas += float64(s.ReadyReplicas)
		}
		snap.InferenceTotal = len(svcs)
		snap.InferenceReady = ready
		snap.InferenceReadyReplica = replicas
	}

	if datasets, err := h.datasetRepo.GetDatasets(); err == nil {
		for _, d := range datasets {
			snap.Datasets = append(snap.Datasets, metrics.DatasetCache{
				Name:          d.Name,
				CachedPercent: d.CachedPercent,
			})
		}
	}

	if h.telemetry != nil {
		t := h.telemetry.Snapshot()
		snap.ClustersDiscovered = t.ClustersDiscovered
		snap.GPUsDiscovered = t.GPUsDiscovered
		snap.JobsSubmitted = t.JobsSubmitted
		snap.GPUsSubmitted = t.GPUsSubmitted
		snap.AssignmentsCompleted = t.AssignmentsDone
		snap.GPUsAllocated = t.GPUsAllocated
	}

	if h.workspaceRepo != nil {
		items, err := h.workspaceRepo.ListWorkspaces("", ports.WorkspaceFilter{})
		if err == nil {
			byStatus := map[string]int{}
			running := 0
			for i := range items {
				byStatus[string(items[i].Status)]++
				if items[i].Status == models.WorkspaceStatusRunning {
					running++
				}
			}
			snap.WorkspaceTotal = len(items)
			snap.WorkspaceRunning = running
			snap.WorkspaceByStatus = byStatus
		}
	}

	return snap, nil
}

func (h *Handler) GetMetrics(c *gin.Context) {
	m, err := h.scheduler.GetMetrics("")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, m)
}