package api

import (
	"net/http"

	evaluationapp "fuze-ai-paas/backend/internal/app/evaluation"
	"fuze-ai-paas/backend/internal/ports"
	"github.com/gin-gonic/gin"
)

func (h *Handler) ListEvaluations(c *gin.Context) {
	tenantID := ""
	if !h.isPlatformAdmin(c) {
		tenantID = h.principalTenant(c)
	}
	list, err := h.evaluation.List(c.Request.Context(), tenantID)
	if err != nil {
		respondEvalError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"evaluations": list})
}

func (h *Handler) CreateEvaluation(c *gin.Context) {
	var body struct {
		Name         string `json:"name"`
		Task         string `json:"task"`
		Dataset      string `json:"dataset"`
		ExperimentID string `json:"experiment_id"`
		RunID        string `json:"run_id"`
		ModelID      string `json:"model_id"`
		Criteria     string `json:"criteria"`
		
		JudgeMode string `json:"judge_mode"`
		
		Dimensions string `json:"dimensions"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	tenantID, ok := h.requireWriteTenant(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing tenant context"})
		return
	}
	created, err := h.evaluation.Create(c.Request.Context(), evaluationapp.CreateInput{
		Name:         body.Name,
		Task:         body.Task,
		Dataset:      body.Dataset,
		ExperimentID: body.ExperimentID,
		RunID:        body.RunID,
		ModelID:      body.ModelID,
		Criteria:     body.Criteria,
		TenantID:     tenantID,
		CreatedBy:    h.claimsOf(c).UserID,
		JudgeMode:    body.JudgeMode,
		Dimensions:   body.Dimensions,
	})
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, created)
}

func (h *Handler) GetEvaluation(c *gin.Context) {
	e, err := h.evaluation.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		respondEvalError(c, err)
		return
	}
	
	if !h.canAccessTenant(e.TenantID, c) {
		c.JSON(http.StatusNotFound, gin.H{"error": "evaluation not found"})
		return
	}
	c.JSON(http.StatusOK, e)
}

func (h *Handler) RecordEvaluationResult(c *gin.Context) {
	id := c.Param("id")
	
	owning, err := h.evaluation.Get(c.Request.Context(), id)
	if err != nil {
		respondEvalError(c, err)
		return
	}
	if !h.canAccessTenant(owning.TenantID, c) {
		c.JSON(http.StatusNotFound, gin.H{"error": "evaluation not found"})
		return
	}
	var body struct {
		Metrics map[string]float64 `json:"metrics"`
		Score   float64            `json:"score"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	if err := h.evaluation.RecordResult(c.Request.Context(), id, body.Metrics, body.Score); err != nil {
		respondEvalError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) FailEvaluation(c *gin.Context) {
	id := c.Param("id")
	
	owning, err := h.evaluation.Get(c.Request.Context(), id)
	if err != nil {
		respondEvalError(c, err)
		return
	}
	if !h.canAccessTenant(owning.TenantID, c) {
		c.JSON(http.StatusNotFound, gin.H{"error": "evaluation not found"})
		return
	}
	var body struct {
		Reason string `json:"reason"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.evaluation.Fail(c.Request.Context(), id, body.Reason); err != nil {
		respondEvalError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) DeleteEvaluation(c *gin.Context) {
	id := c.Param("id")
	
	owning, err := h.evaluation.Get(c.Request.Context(), id)
	if err != nil {
		respondEvalError(c, err)
		return
	}
	if !h.canAccessTenant(owning.TenantID, c) {
		c.JSON(http.StatusNotFound, gin.H{"error": "evaluation not found"})
		return
	}
	if err := h.evaluation.Delete(c.Request.Context(), id); err != nil {
		respondEvalError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": id})
}

func (h *Handler) ListExperimentEvaluations(c *gin.Context) {
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
	list, err := h.evaluation.ListByExperiment(c.Request.Context(), expID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"evaluations": list})
}

func respondEvalError(c *gin.Context, err error) {
	if err == ports.ErrNotFound {
		c.JSON(http.StatusNotFound, gin.H{"error": "evaluation not found"})
		return
	}
	c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
}

func (h *Handler) ListEvaluationReviews(c *gin.Context) {
	id := c.Param("id")
	owning, err := h.evaluation.Get(c.Request.Context(), id)
	if err != nil {
		respondEvalError(c, err)
		return
	}
	if !h.canAccessTenant(owning.TenantID, c) {
		c.JSON(http.StatusNotFound, gin.H{"error": "evaluation not found"})
		return
	}
	list, err := h.evaluation.ListReviews(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"reviews": list})
}

func (h *Handler) SubmitEvaluationReview(c *gin.Context) {
	id := c.Param("id")
	owning, err := h.evaluation.Get(c.Request.Context(), id)
	if err != nil {
		respondEvalError(c, err)
		return
	}
	if !h.canAccessTenant(owning.TenantID, c) {
		c.JSON(http.StatusNotFound, gin.H{"error": "evaluation not found"})
		return
	}
	var body struct {
		Scores  string `json:"scores"`
		Comment string `json:"comment"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	claims := h.claimsOf(c)
	review, err := h.evaluation.SubmitReview(c.Request.Context(), id, evaluationapp.ReviewInput{
		JudgeID: claims.UserID,
		Scores:  body.Scores,
		Comment: body.Comment,
	})
	if err != nil {
		respondEvalError(c, err)
		return
	}
	c.JSON(http.StatusCreated, review)
}

func (h *Handler) RunEvaluationLLMJudge(c *gin.Context) {
	id := c.Param("id")
	owning, err := h.evaluation.Get(c.Request.Context(), id)
	if err != nil {
		respondEvalError(c, err)
		return
	}
	if !h.canAccessTenant(owning.TenantID, c) {
		c.JSON(http.StatusNotFound, gin.H{"error": "evaluation not found"})
		return
	}
	var body struct {
		ModelOutput string `json:"model_output"`
		Reference   string `json:"reference"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	review, err := h.evaluation.RunLLMJudge(c.Request.Context(), id, body.ModelOutput, body.Reference)
	if err != nil {
		respondEvalError(c, err)
		return
	}
	c.JSON(http.StatusCreated, review)
}

func (h *Handler) GetEvaluationReport(c *gin.Context) {
	id := c.Param("id")
	owning, err := h.evaluation.Get(c.Request.Context(), id)
	if err != nil {
		respondEvalError(c, err)
		return
	}
	if !h.canAccessTenant(owning.TenantID, c) {
		c.JSON(http.StatusNotFound, gin.H{"error": "evaluation not found"})
		return
	}
	report, err := h.evaluation.GetReport(c.Request.Context(), id)
	if err != nil {
		respondEvalError(c, err)
		return
	}
	c.JSON(http.StatusOK, report)
}

func (h *Handler) FinalizeEvaluationReport(c *gin.Context) {
	id := c.Param("id")
	owning, err := h.evaluation.Get(c.Request.Context(), id)
	if err != nil {
		respondEvalError(c, err)
		return
	}
	if !h.canAccessTenant(owning.TenantID, c) {
		c.JSON(http.StatusNotFound, gin.H{"error": "evaluation not found"})
		return
	}
	report, err := h.evaluation.FinalizeReport(c.Request.Context(), id)
	if err != nil {
		respondEvalError(c, err)
		return
	}
	c.JSON(http.StatusOK, report)
}