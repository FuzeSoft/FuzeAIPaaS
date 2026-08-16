package api

import (
	"errors"
	"log"
	"net/http"
	"strconv"

	trainingapp "fuze-ai-paas/backend/internal/app/training"
	trainingdomain "fuze-ai-paas/backend/internal/domain/training"
	"fuze-ai-paas/backend/internal/k8s"
	"fuze-ai-paas/backend/internal/models"
	"fuze-ai-paas/backend/internal/ports"

	"github.com/gin-gonic/gin"
)

const (
	
	defaultLogTailLines = 100
	maxLogTailLines     = 5000
)

type trainingCreateRequest struct {
	Name      string `json:"name"`
	ClusterID string `json:"cluster_id"`
	
	TemplateID string `json:"template_id"`

	trainingdomain.Spec

	Checkpointing trainingdomain.CheckpointPolicy  `json:"checkpointing"`
	Registration  trainingdomain.ModelRegistration `json:"register_model"`
}

type checkpointRequest struct {
	URI       string `json:"uri"`
	Step      int    `json:"step"`
	Hash      string `json:"hash"`
	SizeBytes int64  `json:"size_bytes"`
}

type failureRequest struct {
	Reason string `json:"reason"`
}

func (h *Handler) GetTrainingJobs(c *gin.Context) {
	tenant := ""
	if !h.isPlatformAdmin(c) {
		tenant = h.principalTenant(c)
	}
	jobs, err := h.training.List(tenant)
	if err != nil {
		log.Printf("[training] GetTrainingJobs failed for tenant %q: %v", tenant, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, jobs)
}

func (h *Handler) GetTrainingJob(c *gin.Context) {
	job, ok := h.loadTrainingJob(c)
	if !ok {
		return
	}
	c.JSON(http.StatusOK, job)
}

func (h *Handler) CreateTrainingJob(c *gin.Context) {
	var req trainingCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	tenantID, ok := h.requireWriteTenant(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing tenant context"})
		return
	}
	job, err := h.training.Submit(tenantID, trainingapp.SubmitInput{
		Name:          req.Name,
		ClusterID:     req.ClusterID,
		TemplateID:    req.TemplateID,
		Spec:          req.Spec,
		Checkpointing: req.Checkpointing,
		Registration:  req.Registration,
	})
	if err != nil {
		respondTrainingError(c, err)
		return
	}

	h.audit(c, models.ActionCreate, models.ResJob, job.ID, job.Name)
	c.JSON(http.StatusCreated, job)
}

func (h *Handler) CancelTrainingJob(c *gin.Context) {
	job, ok := h.loadTrainingJob(c)
	if !ok {
		return
	}
	updated, err := h.training.Cancel(job.ID)
	if err != nil {
		respondTrainingError(c, err)
		return
	}
	h.audit(c, models.ActionUpdate, models.ResJob, updated.ID, updated.Name)
	c.JSON(http.StatusOK, updated)
}

func (h *Handler) ResumeTrainingJob(c *gin.Context) {
	job, ok := h.loadTrainingJob(c)
	if !ok {
		return
	}
	if err := h.training.Resume(job.ID); err != nil {
		respondTrainingError(c, err)
		return
	}
	updated, err := h.training.Get(job.ID)
	if err != nil {
		respondTrainingError(c, err)
		return
	}
	h.audit(c, models.ActionUpdate, models.ResJob, updated.ID, updated.Name)
	c.JSON(http.StatusOK, updated)
}

func (h *Handler) CompleteTrainingJob(c *gin.Context) {
	job, ok := h.loadTrainingJob(c)
	if !ok {
		return
	}
	if err := h.training.Complete(job.ID); err != nil {
		respondTrainingError(c, err)
		return
	}
	updated, err := h.training.Get(job.ID)
	if err != nil {
		respondTrainingError(c, err)
		return
	}
	h.audit(c, models.ActionUpdate, models.ResJob, updated.ID, updated.Name)
	c.JSON(http.StatusOK, updated)
}

func (h *Handler) ReportTrainingFailure(c *gin.Context) {
	job, ok := h.loadTrainingJob(c)
	if !ok {
		return
	}
	var req failureRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	outcome, err := h.training.HandleFailure(job.ID, req.Reason)
	if err != nil {
		respondTrainingError(c, err)
		return
	}
	updated, err := h.training.Get(job.ID)
	if err != nil {
		respondTrainingError(c, err)
		return
	}
	
	c.JSON(http.StatusOK, gin.H{"outcome": outcome.String(), "job": updated})
}

func (h *Handler) RecordTrainingCheckpoint(c *gin.Context) {
	job, ok := h.loadTrainingJob(c)
	if !ok {
		return
	}
	var req checkpointRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.training.RecordCheckpoint(job.ID, trainingdomain.Checkpoint{
		URI:       req.URI,
		Step:      req.Step,
		Hash:      req.Hash,
		SizeBytes: req.SizeBytes,
	}); err != nil {
		respondTrainingError(c, err)
		return
	}
	updated, err := h.training.Get(job.ID)
	if err != nil {
		respondTrainingError(c, err)
		return
	}
	c.JSON(http.StatusOK, trainingCheckpointView(updated))
}

func (h *Handler) GetTrainingCheckpoint(c *gin.Context) {
	job, ok := h.loadTrainingJob(c)
	if !ok {
		return
	}
	c.JSON(http.StatusOK, trainingCheckpointView(job))
}

func (h *Handler) GetTrainingTemplates(c *gin.Context) {
	c.JSON(http.StatusOK, h.training.ListTemplates())
}

func (h *Handler) GetTrainingJobLogs(c *gin.Context) {
	job, ok := h.loadTrainingJob(c)
	if !ok {
		return
	}

	tail := defaultLogTailLines
	if v := c.Query("tail"); v != "" {
		if n, e := strconv.Atoi(v); e == nil && n > 0 {
			tail = n
		}
	}
	
	if tail > maxLogTailLines {
		tail = maxLogTailLines
	}

	query := k8s.LogQuery{
		Pod:       c.Query("pod"),
		Task:      c.Query("task"),
		TailLines: tail,
	}

	result, available, err := h.scheduler.GetJobLogs(job, query)
	if err != nil {
		
		if errors.Is(err, k8s.ErrPodNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Pod not found for this job", "pods": result.Pods})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if !available {
		c.JSON(http.StatusOK, gin.H{
			"available":      false,
			"logs":           "",
			"pods":           []k8s.PodRef{},
			"failure_reason": job.FailureReason,
			"message":        "任务未提交到真实集群，暂无可拉取的日志",
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"available": true,
		"logs":      result.Logs,
		"pods":      result.Pods,
		"pod":       query.Pod,
		"task":      query.Task,
		"tail":      tail,
	})
}

func (h *Handler) DeleteTrainingJob(c *gin.Context) {
	job, ok := h.loadTrainingJob(c)
	if !ok {
		return
	}
	if err := h.training.Delete(job.ID); err != nil {
		respondTrainingError(c, err)
		return
	}
	h.audit(c, models.ActionDelete, models.ResJob, job.ID, job.Name)
	c.JSON(http.StatusNoContent, nil)
}

func (h *Handler) loadTrainingJob(c *gin.Context) (*models.Job, bool) {
	job, err := h.training.Get(c.Param("id"))
	if err != nil {
		respondTrainingError(c, err)
		return nil, false
	}
	if !h.canAccessTenant(job.TenantID, c) {
		c.JSON(http.StatusNotFound, gin.H{"error": "Training job not found"})
		return nil, false
	}
	return job, true
}

func trainingCheckpointView(job *models.Job) gin.H {
	return gin.H{
		"job_id":              job.ID,
		"enabled":             job.CheckpointEnabled,
		"interval_steps":      job.CheckpointInterval,
		"max_retries":         job.CheckpointMaxRetries,
		"retry_attempts":      job.RetryAttempts,
		"latest_uri":          job.LatestCheckpointURI,
		"latest_step":         job.LatestCheckpointStep,
		"latest_at":           job.LatestCheckpointAt,
		"resume_from":         job.ResumeFrom,
		"registered_version":  job.RegisteredVersionID,
		"register_model":      job.RegisterModelEnabled,
		"register_model_id":   job.RegisterModelID,
		"register_version_tg": job.RegisterVersionTag,
	}
}

func respondTrainingError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, trainingapp.ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "Training job not found"})
	case errors.Is(err, trainingapp.ErrInvalidSpec):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, ports.ErrQuotaExceeded):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	case errors.Is(err, trainingapp.ErrInvalidState):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	}
}