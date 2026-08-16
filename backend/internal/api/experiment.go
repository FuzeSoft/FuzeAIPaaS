package api

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	adapter "fuze-ai-paas/backend/internal/adapter"
	experimentapp "fuze-ai-paas/backend/internal/app/experiment"
	trainingapp "fuze-ai-paas/backend/internal/app/training"
	"fuze-ai-paas/backend/internal/domain/reproduction"
	"fuze-ai-paas/backend/internal/models"
	"fuze-ai-paas/backend/internal/ports"

	"github.com/gin-gonic/gin"
)

const (
	actionTypeExperiment = "experiment"
	actionTypeRun        = "run"
)

type experimentCreateRequest struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Objective   string   `json:"objective"` 
	MetricName  string   `json:"metric_name"`
	Tags        []string `json:"tags"`
}

type runCreateRequest struct {
	Name            string                 `json:"name"`
	Hyperparameters map[string]interface{} `json:"hyperparameters"`
	SourceJobID     string                 `json:"source_job_id"`
}

type runCompleteRequest struct {
	MetricValue *float64               `json:"metric_value"`
	Metrics     map[string]interface{} `json:"metrics"`
	ArtifactURI string                 `json:"artifact_uri"`
}

func (h *Handler) ListExperiments(c *gin.Context) {
	tenant := ""
	if !h.isPlatformAdmin(c) {
		tenant = h.principalTenant(c)
	}
	list, err := h.experiment.ListExperiments(tenant)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, list)
}

func (h *Handler) CreateExperiment(c *gin.Context) {
	var req experimentCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	tenantID, ok := h.requireWriteTenant(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing tenant context"})
		return
	}
	exp, err := h.experiment.CreateExperiment(experimentapp.ExperimentInput{
		TenantID:    tenantID,
		Name:        req.Name,
		Description: req.Description,
		Objective:   req.Objective,
		MetricName:  req.MetricName,
		Tags:        req.Tags,
	})
	if err != nil {
		respondExperimentError(c, err)
		return
	}
	h.audit(c, models.ActionCreate, actionTypeExperiment, exp.ID, exp.Name)
	c.JSON(http.StatusCreated, exp)
}

func (h *Handler) GetExperiment(c *gin.Context) {
	id := c.Param("id")
	exp, err := h.experiment.GetExperiment(id)
	if err != nil {
		respondExperimentError(c, err)
		return
	}
	runs, err := h.experiment.ListRuns(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if !h.canAccessTenant(exp.TenantID, c) {
		c.JSON(http.StatusNotFound, gin.H{"error": "experiment not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"experiment": exp, "runs": runs})
}

func (h *Handler) ArchiveExperiment(c *gin.Context) {
	id := c.Param("id")
	
	exp, err := h.experiment.GetExperiment(id)
	if err != nil {
		respondExperimentError(c, err)
		return
	}
	if !h.canAccessTenant(exp.TenantID, c) {
		c.JSON(http.StatusNotFound, gin.H{"error": "experiment not found"})
		return
	}
	archived, err := h.experiment.ArchiveExperiment(id)
	if err != nil {
		respondExperimentError(c, err)
		return
	}
	h.audit(c, models.ActionUpdate, actionTypeExperiment, archived.ID, archived.Name)
	c.JSON(http.StatusOK, archived)
}

func (h *Handler) DeleteExperiment(c *gin.Context) {
	id := c.Param("id")
	exp, err := h.experiment.GetExperiment(id)
	if err != nil {
		respondExperimentError(c, err)
		return
	}
	if !h.canAccessTenant(exp.TenantID, c) {
		c.JSON(http.StatusNotFound, gin.H{"error": "experiment not found"})
		return
	}
	if err := h.experiment.DeleteExperiment(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	h.audit(c, models.ActionDelete, actionTypeExperiment, id, exp.Name)
	c.JSON(http.StatusOK, gin.H{"deleted": id})
}

func (h *Handler) ListRuns(c *gin.Context) {
	expID := c.Param("id")
	
	exp, err := h.experiment.GetExperiment(expID)
	if err != nil {
		respondExperimentError(c, err)
		return
	}
	if !h.canAccessTenant(exp.TenantID, c) {
		c.JSON(http.StatusNotFound, gin.H{"error": "experiment not found"})
		return
	}
	runs, err := h.experiment.ListRuns(expID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, runs)
}

func (h *Handler) CreateRun(c *gin.Context) {
	expID := c.Param("id")
	var req runCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	run, err := h.experiment.CreateRun(experimentapp.RunInput{
		TenantID:        h.principalTenant(c),
		ExperimentID:    expID,
		Name:            req.Name,
		Hyperparameters: req.Hyperparameters,
		SourceJobID:     req.SourceJobID,
	})
	if err != nil {
		respondExperimentError(c, err)
		return
	}
	h.audit(c, models.ActionCreate, actionTypeRun, run.ID, run.Name)
	c.JSON(http.StatusCreated, run)
}

func (h *Handler) CompleteRun(c *gin.Context) {
	id := c.Param("runId")
	
	owning, err := h.experiment.GetRun(id)
	if err != nil {
		respondExperimentError(c, err)
		return
	}
	if !h.canAccessTenant(owning.TenantID, c) {
		c.JSON(http.StatusNotFound, gin.H{"error": "run not found"})
		return
	}
	var req runCompleteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	run, err := h.experiment.CompleteRun(id, req.MetricValue, req.Metrics, req.ArtifactURI)
	if err != nil {
		respondExperimentError(c, err)
		return
	}
	h.audit(c, models.ActionUpdate, actionTypeRun, run.ID, run.Name)
	c.JSON(http.StatusOK, run)
}

func (h *Handler) FailRun(c *gin.Context) {
	id := c.Param("runId")
	
	owning, err := h.experiment.GetRun(id)
	if err != nil {
		respondExperimentError(c, err)
		return
	}
	if !h.canAccessTenant(owning.TenantID, c) {
		c.JSON(http.StatusNotFound, gin.H{"error": "run not found"})
		return
	}
	run, err := h.experiment.FailRun(id)
	if err != nil {
		respondExperimentError(c, err)
		return
	}
	c.JSON(http.StatusOK, run)
}

func (h *Handler) CancelRun(c *gin.Context) {
	id := c.Param("runId")
	
	owning, err := h.experiment.GetRun(id)
	if err != nil {
		respondExperimentError(c, err)
		return
	}
	if !h.canAccessTenant(owning.TenantID, c) {
		c.JSON(http.StatusNotFound, gin.H{"error": "run not found"})
		return
	}
	run, err := h.experiment.CancelRun(id)
	if err != nil {
		respondExperimentError(c, err)
		return
	}
	c.JSON(http.StatusOK, run)
}

func respondExperimentError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, experimentapp.ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "experiment not found"})
	case errors.Is(err, experimentapp.ErrInvalidSpec):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, experimentapp.ErrInvalidState):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	}
}

type experimentCompareResponse struct {
	MetricName             string                `json:"metric_name"`
	Objective              string                `json:"objective"`
	Experiments            []compareExperimentRow `json:"experiments"`
	OverallBestExperimentID string               `json:"overall_best_experiment_id"`
}

type compareExperimentRow struct {
	ID            string      `json:"id"`
	Name          string      `json:"name"`
	Objective     string      `json:"objective"`
	BestRun       *models.Run `json:"best_run"`
	IsOverallBest bool        `json:"is_overall_best"`
}

func (h *Handler) CompareExperiments(c *gin.Context) {
	idsParam := c.Query("ids")
	metricName := c.Query("metric_name")
	if strings.TrimSpace(idsParam) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ids required"})
		return
	}
	if strings.TrimSpace(metricName) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "metric_name required"})
		return
	}
	ids := strings.Split(idsParam, ",")

	rows := make([]compareExperimentRow, 0, len(ids))
	var overallBest *compareExperimentRow
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		exp, err := h.experiment.GetExperiment(id)
		if err != nil {
			respondExperimentError(c, err)
			return
		}
		if !h.canAccessTenant(exp.TenantID, c) {
			c.JSON(http.StatusNotFound, gin.H{"error": "experiment not found"})
			return
		}
		if exp.MetricName != metricName {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": fmt.Sprintf("experiment %q has metric_name %q, differs from compare target %q",
					exp.Name, exp.MetricName, metricName),
			})
			return
		}
		best := bestRun(h.experiment, id, exp.Objective)
		row := compareExperimentRow{ID: exp.ID, Name: exp.Name, Objective: exp.Objective, BestRun: best}
		rows = append(rows, row)
		if best != nil {
			if overallBest == nil || betterThan(exp.Objective, best, overallBest.BestRun) {
				overallBest = &row
			}
		}
	}

	resp := experimentCompareResponse{MetricName: metricName, Experiments: rows}
	if overallBest != nil {
		overallBest.IsOverallBest = true
		resp.OverallBestExperimentID = overallBest.ID
		resp.Objective = overallBest.Objective
	}
	c.JSON(http.StatusOK, resp)
}

func bestRun(svc *experimentapp.Service, experimentID, objective string) *models.Run {
	runs, err := svc.ListRuns(experimentID)
	if err != nil || len(runs) == 0 {
		return nil
	}
	var best *models.Run
	for i := range runs {
		r := &runs[i]
		if r.MetricValue == nil {
			continue
		}
		if best == nil || betterVal(objective, *r.MetricValue, *best.MetricValue) {
			best = r
		}
	}
	return best
}

func betterVal(objective string, a, b float64) bool {
	if objective == "minimize" {
		return a < b
	}
	return a > b
}

func betterThan(objective string, a, b *models.Run) bool {
	if a == nil || a.MetricValue == nil {
		return false
	}
	if b == nil || b.MetricValue == nil {
		return true
	}
	return betterVal(objective, *a.MetricValue, *b.MetricValue)
}

func (h *Handler) ReproduceRun(c *gin.Context) {
	runID := c.Param("runId")
	source, err := h.experiment.GetRun(runID)
	if err != nil {
		respondExperimentError(c, err)
		return
	}
	if !h.canAccessTenant(source.TenantID, c) {
		c.JSON(http.StatusNotFound, gin.H{"error": "run not found"})
		return
	}
	if source.SourceJobID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "run has no source_job_id to reproduce"})
		return
	}
	job, err := h.jobRepo.GetJob(source.SourceJobID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "load source job: " + err.Error()})
		return
	}
	if job == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "source job not found"})
		return
	}
	
	spec := adapter.TrainingFromModel(job).Spec
	newJob, err := h.training.Submit(source.TenantID, trainingapp.SubmitInput{
		Name: source.Name + "-repro",
		Spec: spec,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "submit reproduction job: " + err.Error()})
		return
	}
	repro, err := h.experiment.CreateReproductionRun(source, newJob.ID)
	if err != nil {
		respondExperimentError(c, err)
		return
	}
	h.audit(c, models.ActionCreate, actionTypeRun, repro.ID, repro.Name)
	c.JSON(http.StatusCreated, repro)
}

func (h *Handler) GetReproduction(c *gin.Context) {
	runID := c.Param("runId")
	repro, err := h.experiment.GetRun(runID)
	if err != nil {
		respondExperimentError(c, err)
		return
	}
	if !h.canAccessTenant(repro.TenantID, c) {
		c.JSON(http.StatusNotFound, gin.H{"error": "run not found"})
		return
	}
	if repro.ParentRunID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "run is not a reproduction run"})
		return
	}
	source, err := h.experiment.GetRun(repro.ParentRunID)
	if err != nil {
		respondExperimentError(c, err)
		return
	}
	exp, err := h.experiment.GetExperiment(repro.ExperimentID)
	if err != nil {
		respondExperimentError(c, err)
		return
	}

	if repro.MetricValue == nil {
		res := reproduction.Evaluate(reproduction.Config{AbsTol: reproduction.DefaultAbsTol, RelTol: reproduction.DefaultRelTol},
			exp.MetricName, source.ID, repro.ID, deref(source.MetricValue), 0)
		c.JSON(http.StatusOK, gin.H{
			"source_run_id":  res.SourceRunID,
			"repro_run_id":   res.ReproRunID,
			"metric_name":    res.MetricName,
			"source_metric":  res.SourceMetric,
			"repro_metric":   nil,
			"reproducible":   false,
			"status":         "pending",
			"reproduction_state": repro.ReproductionState,
		})
		return
	}

	res := reproduction.Evaluate(reproduction.Config{AbsTol: reproduction.DefaultAbsTol, RelTol: reproduction.DefaultRelTol},
		exp.MetricName, source.ID, repro.ID, deref(source.MetricValue), *repro.MetricValue)

	if isTerminalRun(repro.Status) && ((res.Reproducible && repro.ReproductionState != "matched") ||
		(!res.Reproducible && repro.ReproductionState != "diverged")) {
		repro.ReproductionState = "matched"
		if !res.Reproducible {
			repro.ReproductionState = "diverged"
		}
		if err := h.experiment.UpdateRun(repro); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "update reproduction state: " + err.Error()})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"source_run_id":      res.SourceRunID,
		"repro_run_id":       res.ReproRunID,
		"metric_name":        res.MetricName,
		"source_metric":      res.SourceMetric,
		"repro_metric":       res.ReproMetric,
		"abs_deviation":      res.AbsDeviation,
		"rel_deviation":      res.RelDeviation,
		"reproducible":       res.Reproducible,
		"abs_tol":            res.AbsTol,
		"rel_tol":            res.RelTol,
		"reproduction_state": repro.ReproductionState,
		"evaluated_at":       res.EvaluatedAt,
	})
}

func deref(v *float64) float64 {
	if v == nil {
		return 0
	}
	return *v
}

func isTerminalRun(status string) bool {
	switch status {
	case "completed", "failed", "cancelled":
		return true
	default:
		return false
	}
}

func (h *Handler) QueryMetrics(c *gin.Context) {
	var q ports.MetricQuery
	if err := c.ShouldBindJSON(&q); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	series, err := h.metrics.QueryRange(q)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"series": series})
}

func (h *Handler) QueryMetricLatest(c *gin.Context) {
	var q ports.MetricQuery
	if err := c.ShouldBindJSON(&q); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	sample, err := h.metrics.QueryLatest(q)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	if sample == nil {
		c.JSON(http.StatusOK, gin.H{"sample": nil})
		return
	}
	c.JSON(http.StatusOK, gin.H{"sample": sample})
}