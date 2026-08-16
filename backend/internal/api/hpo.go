package api

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"time"

	hpoapp "fuze-ai-paas/backend/internal/app/hpo"
	"fuze-ai-paas/backend/internal/domain/hpo"
	"fuze-ai-paas/backend/internal/models"
	"fuze-ai-paas/backend/internal/ports"
	"github.com/gin-gonic/gin"
)

const actionTypeStudy = "hpo_study"
const actionTypeTrial = "hpo_trial"

func (h *Handler) hpoOr501(c *gin.Context) {
	if h.hpo == nil {
		c.JSON(http.StatusNotImplemented, gin.H{
			"error": "AutoML/NAS service not enabled (missing HPO storage ports)",
		})
		c.Abort()
	}
}

type createStudyRequest struct {
	ExperimentID     string                  `json:"experiment_id"`
	Name             string                  `json:"name"`
	Algorithm        string                  `json:"algorithm"`
	ObjectiveMetric  string                  `json:"objective_metric"`
	ObjectiveDirection string                `json:"objective_direction"` 
	SearchSpace      []hpoParamSpec          `json:"search_space"`
	MaxTrials        int                     `json:"max_trials"`
	MaxParallel      int                     `json:"max_parallel"`
	MaxDurationSec   int                     `json:"max_duration_sec"`
	EarlyStop        *hpoEarlyStopRequest    `json:"early_stop,omitempty"`
	TrainingTemplate map[string]any          `json:"training_template"`
}

type hpoParamSpec struct {
	Name     string   `json:"name"`
	Type     string   `json:"type"` 
	Min      *float64 `json:"min,omitempty"`
	Max      *float64 `json:"max,omitempty"`
	Step     *float64 `json:"step,omitempty"`
	LogScale bool     `json:"log_scale"`
	Choices  []any    `json:"choices,omitempty"`
}

type hpoEarlyStopRequest struct {
	Enabled  bool    `json:"enabled"`
	Eta      float64 `json:"eta"`
	MinRungs int     `json:"min_rungs"`
}

type reportTrialRequest struct {
	Step  int     `json:"step"`
	Value float64 `json:"value"`
}

func (h *Handler) HPORegisterRoutes(protected *gin.RouterGroup) {
	
	grp := protected.Group("/hpo")
	{
		grp.GET("", h.ListStudies)
		grp.POST("", h.CreateStudy)
		grp.GET("/:id", h.GetStudy)
		grp.DELETE("/:id", h.DeleteStudy)
		grp.POST("/:id/run", h.RunStudy)
		grp.GET("/:id/trials", h.ListTrials)
		grp.GET("/:id/trials/:trialId", h.GetTrial)
		grp.POST("/:id/trials/:trialId/report", h.ReportTrial)
		grp.POST("/:id/trials/:trialId/final", h.ReportTrialFinal)
	}

	gw := protected.Group("/automl")
	{
		gw.POST("/search_space", h.ForwardToGateway("search_space"))
		gw.POST("/:id/suggest", h.ForwardToGateway("suggest"))
		gw.POST("/:id/report", h.ForwardToGateway("report"))
		gw.POST("/:id/trials/:trialId/report", h.ForwardToGateway("trial_report"))
	}
}

func (h *Handler) CreateStudy(c *gin.Context) {
	if h.hpo == nil {
		h.hpoOr501(c)
		return
	}
	var req createStudyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.ObjectiveMetric == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "objective_metric is required"})
		return
	}
	space, err := toDomainSpace(req.SearchSpace)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	spec := hpoappStudySpec(req, space)
	tenantID, ok := h.requireWriteTenant(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing tenant context"})
		return
	}
	m, err := h.hpo.CreateStudy(c.Request.Context(), tenantID, spec)
	if err != nil {
		respondHPOError(c, err)
		return
	}
	h.audit(c, models.ActionCreate, actionTypeStudy, m.ID, m.Name)
	c.JSON(http.StatusCreated, m)
}

func (h *Handler) ListStudies(c *gin.Context) {
	if h.hpo == nil {
		h.hpoOr501(c)
		return
	}
	list, err := h.hpo.ListStudies(c.Request.Context(), h.principalTenant(c))
	if err != nil {
		respondHPOError(c, err)
		return
	}
	c.JSON(http.StatusOK, list)
}

func (h *Handler) GetStudy(c *gin.Context) {
	if h.hpo == nil {
		h.hpoOr501(c)
		return
	}
	id := c.Param("id")
	m, err := h.hpo.GetStudy(c.Request.Context(), h.principalTenant(c), id)
	if err != nil {
		respondHPOError(c, err)
		return
	}
	c.JSON(http.StatusOK, m)
}

func (h *Handler) DeleteStudy(c *gin.Context) {
	if h.hpo == nil {
		h.hpoOr501(c)
		return
	}
	id := c.Param("id")
	tenantID, ok := h.requireWriteTenant(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing tenant context"})
		return
	}
	if err := h.hpo.DeleteStudy(c.Request.Context(), tenantID, id); err != nil {
		respondHPOError(c, err)
		return
	}
	h.audit(c, models.ActionDelete, actionTypeStudy, id, id)
	c.Status(http.StatusNoContent)
}

func (h *Handler) RunStudy(c *gin.Context) {
	if h.hpo == nil {
		h.hpoOr501(c)
		return
	}
	id := c.Param("id")
	tenantID, ok := h.requireWriteTenant(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing tenant context"})
		return
	}
	if err := h.hpo.RunOnce(c.Request.Context(), tenantID, id); err != nil {
		respondHPOError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "scheduled", "study_id": id})
}

func (h *Handler) ListTrials(c *gin.Context) {
	if h.hpo == nil {
		h.hpoOr501(c)
		return
	}
	id := c.Param("id")
	list, err := h.hpo.ListTrials(c.Request.Context(), h.principalTenant(c), id)
	if err != nil {
		respondHPOError(c, err)
		return
	}
	c.JSON(http.StatusOK, list)
}

func (h *Handler) GetTrial(c *gin.Context) {
	if h.hpo == nil {
		h.hpoOr501(c)
		return
	}
	trialID := c.Param("trialId")
	id := c.Param("id")
	t, err := h.hpo.GetTrial(c.Request.Context(), h.principalTenant(c), id, trialID)
	if err != nil {
		respondHPOError(c, err)
		return
	}
	c.JSON(http.StatusOK, t)
}

func (h *Handler) ReportTrial(c *gin.Context) {
	if h.hpo == nil {
		h.hpoOr501(c)
		return
	}
	trialID := c.Param("trialId")
	var req reportTrialRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	tenantID, ok := h.requireWriteTenant(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing tenant context"})
		return
	}
	stop, err := h.hpo.Report(c.Request.Context(), tenantID, trialID, req.Step, req.Value)
	if err != nil {
		respondHPOError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"stop": stop})
}

func (h *Handler) ReportTrialFinal(c *gin.Context) {
	if h.hpo == nil {
		h.hpoOr501(c)
		return
	}
	trialID := c.Param("trialId")
	var req reportTrialRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	tenantID, ok := h.requireWriteTenant(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing tenant context"})
		return
	}
	if err := h.hpo.ReportFinal(c.Request.Context(), tenantID, trialID, req.Value); err != nil {
		respondHPOError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "recorded"})
}

func (h *Handler) ForwardToGateway(kind string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if h.hpoGatewayBase == "" {
			c.JSON(http.StatusNotImplemented, gin.H{
				"error": "AI gateway (HPO_GATEWAY_BASE_URL) not configured",
			})
			return
		}
		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		path := c.FullPath()
		url := h.hpoGatewayBase + path
		req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodPost, url, bytes.NewReader(body))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		req.Header.Set("Content-Type", "application/json")
		if auth := c.GetHeader("Authorization"); auth != "" {
			req.Header.Set("Authorization", auth)
		}
		resp, err := h.hpoClient.Do(req)
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": "gateway unreachable: " + err.Error()})
			return
		}
		defer resp.Body.Close()
		
		const maxGatewayBody = 16 << 20
		out, err := io.ReadAll(io.LimitReader(resp.Body, maxGatewayBody))
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": "failed reading gateway response"})
			return
		}
		if int64(len(out)) >= maxGatewayBody {
			c.JSON(http.StatusBadGateway, gin.H{"error": "gateway response too large"})
			return
		}
		c.Data(resp.StatusCode, "application/json", out)
	}
}

func toDomainSpace(in []hpoParamSpec) (hpo.SearchSpace, error) {
	params := make([]hpo.ParamSpec, 0, len(in))
	for _, p := range in {
		ps := hpo.ParamSpec{
			Name:     p.Name,
			Type:     p.Type,
			LogScale: p.LogScale,
		}
		if p.Min != nil {
			ps.Min = *p.Min
		}
		if p.Max != nil {
			ps.Max = *p.Max
		}
		if p.Step != nil {
			ps.Step = *p.Step
		}
		ps.Choices = p.Choices
		if err := ps.Validate(); err != nil {
			return hpo.SearchSpace{}, err
		}
		params = append(params, ps)
	}
	return hpo.SearchSpace{Params: params}, nil
}

func hpoappStudySpec(req createStudyRequest, space hpo.SearchSpace) hpoapp.StudySpec {
	dir := req.ObjectiveDirection
	if dir == "" {
		dir = hpo.DirectionMaximize
	}
	algo := req.Algorithm
	if algo == "" {
		algo = "tpe"
	}
	spec := hpoapp.StudySpec{
		Name:           req.Name,
		Algorithm:      algo,
		Objective:      hpo.Objective{MetricName: req.ObjectiveMetric, Direction: dir},
		Space:          space,
		MaxTrials:      req.MaxTrials,
		MaxParallel:    req.MaxParallel,
		TrainingTemplate: req.TrainingTemplate,
		ExperimentID:   req.ExperimentID,
	}
	if req.MaxDurationSec > 0 {
		spec.MaxDuration = time.Duration(req.MaxDurationSec) * time.Second
	}
	if req.EarlyStop != nil && req.EarlyStop.Enabled {
		spec.EarlyStop = &hpo.EarlyStopSpec{
			Enabled:  true,
			Eta:      req.EarlyStop.Eta,
			MinRungs: req.EarlyStop.MinRungs,
		}
	}
	return spec
}

func respondHPOError(c *gin.Context, err error) {
	if errors.Is(err, ports.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	if errors.Is(err, ports.ErrQuotaExceeded) {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
}