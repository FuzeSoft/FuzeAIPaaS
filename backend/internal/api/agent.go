package api

import (
	"errors"
	"log"
	"net/http"

	"fuze-ai-paas/backend/internal/domain/agent"
	"fuze-ai-paas/backend/internal/models"
	"fuze-ai-paas/backend/internal/ports"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func (h *Handler) requireAgent(c *gin.Context) bool {
	if h.agent == nil {
		c.JSON(http.StatusNotImplemented, gin.H{
			"error": "agent orchestration not enabled (missing adapter)",
		})
		return false
	}
	return true
}

type createAgentRequest struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	DAG         agent.DAG `json:"dag"`
}

type compileAgentResponse struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

type startRunRequest struct {
	Input string `json:"input"`
}

type resumeRunRequest struct {
	Decision string `json:"decision"`
}

func (h *Handler) ListAgents(c *gin.Context) {
	if !h.requireAgent(c) {
		return
	}
	tenantID := h.principalTenant(c)
	list, err := h.agent.List(c.Request.Context(), tenantID, c.Query("name"), c.Query("status"))
	if err != nil {
		log.Printf("[agent] ListAgents failed for tenant %s: %v", tenantID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"agents": list})
}

func (h *Handler) CreateAgent(c *gin.Context) {
	if !h.requireAgent(c) {
		return
	}
	var req createAgentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}
	id := req.ID
	if id == "" {
		id = "ag-" + uuid.NewString()
	}
	tenantID, ok := h.requireWriteTenant(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing tenant context"})
		return
	}
	userID := h.claimsOf(c).UserID
	a, err := agent.NewAgent(id, tenantID, req.Name, req.DAG, userID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	a.Description = req.Description
	if err := h.agent.SaveAgent(c.Request.Context(), a); err != nil {
		if errors.Is(err, ports.ErrAgentConflict) {
			c.JSON(http.StatusConflict, gin.H{"error": "agent name conflict"})
			return
		}
		log.Printf("[agent] SaveAgent failed for tenant %s agent %s: %v", tenantID, a.ID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.audit(c, models.ActionCreate, models.ResAgent, a.ID, a.Name)

	c.JSON(http.StatusCreated, gin.H{"agent": a})
}

func (h *Handler) GetAgent(c *gin.Context) {
	if !h.requireAgent(c) {
		return
	}
	tenantID := h.principalTenant(c)
	a, err := h.agent.Get(c.Request.Context(), tenantID, c.Param("id"))
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, ports.ErrAgentNotFound) {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"agent": a})
}

func (h *Handler) CompileAgent(c *gin.Context) {
	if !h.requireAgent(c) {
		return
	}
	tenantID, ok := h.requireWriteTenant(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing tenant context"})
		return
	}
	a, err := h.agent.Compile(c.Request.Context(), tenantID, c.Param("id"))
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, ports.ErrAgentNotFound) {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, compileAgentResponse{ID: a.ID, Status: a.Status})
}

func (h *Handler) DeleteAgent(c *gin.Context) {
	if !h.requireAgent(c) {
		return
	}
	tenantID, ok := h.requireWriteTenant(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing tenant context"})
		return
	}
	if err := h.agent.Delete(c.Request.Context(), tenantID, c.Param("id")); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, ports.ErrAgentNotFound) {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}

	h.audit(c, models.ActionDelete, models.ResAgent, c.Param("id"), "")

	c.Status(http.StatusNoContent)
}

func (h *Handler) StartRun(c *gin.Context) {
	if !h.requireAgent(c) {
		return
	}
	var req startRunRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	tenantID, ok := h.requireWriteTenant(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing tenant context"})
		return
	}
	userID := h.claimsOf(c).UserID
	run, err := h.agent.StartRun(c.Request.Context(), tenantID, c.Param("id"), req.Input, userID)
	if err != nil {
		status := http.StatusInternalServerError
		switch {
		case errors.Is(err, ports.ErrAgentNotFound):
			status = http.StatusNotFound
		case errors.Is(err, agent.ErrAgentNotCompiled):
			status = http.StatusConflict
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"run": run})
}

func (h *Handler) ListAgentRuns(c *gin.Context) {
	if !h.requireAgent(c) {
		return
	}
	tenantID := h.principalTenant(c)
	runs, err := h.agent.ListRuns(c.Request.Context(), tenantID, c.Param("id"), c.Query("status"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"runs": runs})
}

func (h *Handler) GetRun(c *gin.Context) {
	if !h.requireAgent(c) {
		return
	}
	tenantID := h.principalTenant(c)
	run, err := h.agent.GetRun(c.Request.Context(), tenantID, c.Param("runId"))
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, ports.ErrRunNotFound) {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"run": run})
}

func (h *Handler) ResumeRun(c *gin.Context) {
	if !h.requireAgent(c) {
		return
	}
	var req resumeRunRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	tenantID, ok := h.requireWriteTenant(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing tenant context"})
		return
	}
	run, err := h.agent.ResumeRun(c.Request.Context(), tenantID, c.Param("runId"), req.Decision)
	if err != nil {
		status := http.StatusInternalServerError
		switch {
		case errors.Is(err, ports.ErrRunNotFound), errors.Is(err, agent.ErrRunNotPaused):
			status = http.StatusConflict
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"run": run})
}