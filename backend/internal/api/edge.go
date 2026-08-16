package api

import (
	"errors"
	"net/http"

	"fuze-ai-paas/backend/internal/app/edge"
	domainedge "fuze-ai-paas/backend/internal/domain/edge"
	"github.com/gin-gonic/gin"
)

func (h *Handler) registerEdgeRoutes(rg *gin.RouterGroup) {
	edgeG := rg.Group("/edge-nodes")
	{
		edgeG.POST("", h.edgeRegisterNode)
		edgeG.GET("", h.edgeListNodes)
		edgeG.GET("/:id", h.edgeGetNode)
		edgeG.DELETE("/:id", h.edgeDeregisterNode)
		edgeG.POST("/:id/heartbeat", h.edgeNodeHeartbeat)
	}

	depG := rg.Group("/edge-deployments")
	{
		depG.POST("", h.edgeDeploy)
		depG.GET("", h.edgeListDeployments)
		depG.GET("/:id", h.edgeGetDeployment)
		depG.POST("/:id/canary/promote", h.edgePromoteCanary)
		depG.POST("/:id/rollback", h.edgeRollback)
		depG.PUT("/:id/guard", h.edgeSetGuard)
		depG.POST("/:id/drift/check", h.edgeRunDrift)
		depG.POST("/:id/drift/sample", h.edgeSubmitSample)
		depG.GET("/:id/drift", h.edgeLatestDrift)
		depG.POST("/:id/baseline", h.edgeSetBaseline)
		depG.POST("/:id/label-feedback", h.edgeSubmitLabelFeedback)
	}
}

func (h *Handler) requireEdge() *edge.Service {
	if h.edge == nil {
		return nil
	}
	return h.edge
}

func (h *Handler) edgeTenant(c *gin.Context) (string, bool) {
	t, ok := h.principalTenantStrict(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing tenant context"})
		return "", false
	}
	return t, true
}

func (h *Handler) edgeRegisterNode(c *gin.Context) {
	svc := h.requireEdge()
	if svc == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "edge deployment not configured"})
		return
	}
	var body struct {
		ID            string            `json:"id"`
		Name          string            `json:"name"`
		Mode          string            `json:"mode"`
		Endpoint      string            `json:"endpoint"`
		Region        string            `json:"region"`
		Labels        map[string]string `json:"labels"`
		CACertPEM     string            `json:"caCertPem"`
		ClientCertPEM string            `json:"clientCertPem"`
		ClientKeyPEM  string            `json:"clientKeyPem"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	tenantID, ok := h.edgeTenant(c)
	if !ok {
		return
	}
	mode := domainedge.NodeMode(body.Mode)
	if mode != domainedge.NodeModeAgent && mode != domainedge.NodeModeKubeEdge {
		mode = domainedge.NodeModeAgent
	}
	node, err := svc.RegisterNode(c.Request.Context(), tenantID, edge.RegisterNodeInput{
		ID:            body.ID,
		Name:          body.Name,
		Mode:          mode,
		Endpoint:      body.Endpoint,
		Region:        body.Region,
		Labels:        body.Labels,
		CACertPEM:     body.CACertPEM,
		ClientCertPEM: body.ClientCertPEM,
		ClientKeyPEM:  body.ClientKeyPEM,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, node)
}

func (h *Handler) edgeListNodes(c *gin.Context) {
	svc := h.requireEdge()
	if svc == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "edge deployment not configured"})
		return
	}
	nodes, err := svc.ListNodes(c.Request.Context(), h.principalTenant(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"nodes": nodes})
}

func (h *Handler) edgeGetNode(c *gin.Context) {
	svc := h.requireEdge()
	if svc == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "edge deployment not configured"})
		return
	}
	node, err := svc.GetNode(c.Request.Context(), h.principalTenant(c), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "edge node not found"})
		return
	}
	c.JSON(http.StatusOK, node)
}

func (h *Handler) edgeDeregisterNode(c *gin.Context) {
	svc := h.requireEdge()
	if svc == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "edge deployment not configured"})
		return
	}
	tenantID, ok := h.edgeTenant(c)
	if !ok {
		return
	}
	if err := svc.DeregisterNode(c.Request.Context(), tenantID, c.Param("id")); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "edge node not found"})
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) edgeNodeHeartbeat(c *gin.Context) {
	svc := h.requireEdge()
	if svc == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "edge deployment not configured"})
		return
	}
	tenantID, ok := h.edgeTenant(c)
	if !ok {
		return
	}
	node, err := svc.Heartbeat(c.Request.Context(), tenantID, c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "edge node not found"})
		return
	}
	c.JSON(http.StatusOK, node)
}

func (h *Handler) edgeDeploy(c *gin.Context) {
	svc := h.requireEdge()
	if svc == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "edge deployment not configured"})
		return
	}
	var body struct {
		NodeID        string                  `json:"nodeId"`
		ModelID       string                  `json:"modelId"`
		Version       string                  `json:"version"`
		Image         string                  `json:"image"`
		Replicas      int                     `json:"replicas"`
		Command       []string                `json:"command"`
		Args          []string                `json:"args"`
		Env           map[string]string       `json:"env"`
		GPUs          int                     `json:"gpus"`
		CPU           string                  `json:"cpu"`
		Memory        string                  `json:"memory"`
		CanaryWeight  int                     `json:"canaryWeight"`
		AutoRollback  bool                    `json:"autoRollback"`
		DriftGuard    bool                    `json:"driftGuard"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if body.NodeID == "" || body.ModelID == "" || body.Version == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "nodeId, modelId and version are required"})
		return
	}
	if body.Replicas <= 0 {
		body.Replicas = 1
	}
	tenantID, ok := h.edgeTenant(c)
	if !ok {
		return
	}
	spec := domainedge.EdgeDeploySpec{
		Image:    body.Image,
		Command:  body.Command,
		Args:     body.Args,
		Env:      body.Env,
		Replicas: body.Replicas,
		GPUs:     body.GPUs,
		CPU:      body.CPU,
		Memory:   body.Memory,
	}
	d, err := svc.Deploy(c.Request.Context(), tenantID, body.NodeID, body.ModelID, body.Version, spec, body.CanaryWeight, body.AutoRollback, body.DriftGuard)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, d)
}

func (h *Handler) edgeListDeployments(c *gin.Context) {
	svc := h.requireEdge()
	if svc == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "edge deployment not configured"})
		return
	}
	ds, err := svc.ListDeployments(c.Request.Context(), h.principalTenant(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"deployments": ds})
}

func (h *Handler) edgeGetDeployment(c *gin.Context) {
	svc := h.requireEdge()
	if svc == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "edge deployment not configured"})
		return
	}
	d, err := svc.GetDeployment(c.Request.Context(), h.principalTenant(c), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "edge deployment not found"})
		return
	}
	c.JSON(http.StatusOK, d)
}

func (h *Handler) edgePromoteCanary(c *gin.Context) {
	svc := h.requireEdge()
	if svc == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "edge deployment not configured"})
		return
	}
	var body struct {
		Step int `json:"step"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if body.Step < 1 || body.Step > 100 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "step must be between 1 and 100"})
		return
	}
	tenantID, ok := h.edgeTenant(c)
	if !ok {
		return
	}
	d, err := svc.PromoteCanary(c.Request.Context(), tenantID, c.Param("id"), body.Step)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "edge deployment not found"})
		return
	}
	c.JSON(http.StatusOK, d)
}

func (h *Handler) edgeRollback(c *gin.Context) {
	svc := h.requireEdge()
	if svc == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "edge deployment not configured"})
		return
	}
	var body struct {
		Reason string `json:"reason"`
	}
	_ = c.ShouldBindJSON(&body)
	reason := body.Reason
	if reason == "" {
		reason = "manual rollback"
	}
	tenantID, ok := h.edgeTenant(c)
	if !ok {
		return
	}
	d, err := svc.Rollback(c.Request.Context(), tenantID, c.Param("id"), reason)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "edge deployment not found"})
		return
	}
	c.JSON(http.StatusOK, d)
}

func (h *Handler) edgeSetGuard(c *gin.Context) {
	svc := h.requireEdge()
	if svc == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "edge deployment not configured"})
		return
	}
	var body struct {
		DriftGuard   bool `json:"driftGuard"`
		AutoRollback bool `json:"autoRollback"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	tenantID, ok := h.edgeTenant(c)
	if !ok {
		return
	}
	d, err := svc.SetGuard(c.Request.Context(), tenantID, c.Param("id"), body.DriftGuard, body.AutoRollback)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "edge deployment not found"})
		return
	}
	c.JSON(http.StatusOK, d)
}

func (h *Handler) edgeSetBaseline(c *gin.Context) {
	svc := h.requireEdge()
	if svc == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "edge deployment not configured"})
		return
	}
	var b domainedge.DriftBaseline
	if err := c.ShouldBindJSON(&b); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	tenantID, ok := h.edgeTenant(c)
	if !ok {
		return
	}
	if err := svc.SetBaseline(c.Request.Context(), tenantID, c.Param("id"), &b); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) edgeRunDrift(c *gin.Context) {
	svc := h.requireEdge()
	if svc == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "edge deployment not configured"})
		return
	}
	tenantID, ok := h.edgeTenant(c)
	if !ok {
		return
	}
	rep, err := svc.EvaluateDrift(c.Request.Context(), tenantID, c.Param("id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, rep)
}

func (h *Handler) edgeSubmitSample(c *gin.Context) {
	svc := h.requireEdge()
	if svc == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "edge deployment not configured"})
		return
	}
	var sample domainedge.DriftSample
	if err := c.ShouldBindJSON(&sample); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	tenantID, ok := h.edgeTenant(c)
	if !ok {
		return
	}
	rep, err := svc.SubmitSample(c.Request.Context(), tenantID, c.Param("id"), &sample)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, rep)
}

func (h *Handler) edgeSubmitLabelFeedback(c *gin.Context) {
	svc := h.requireEdge()
	if svc == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "edge deployment not configured"})
		return
	}
	var body struct {
		Label     string `json:"label"`
		RequestID string `json:"requestId,omitempty"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if body.Label == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "label required"})
		return
	}
	tenantID, ok := h.edgeTenant(c)
	if !ok {
		return
	}
	if err := svc.SubmitLabelFeedback(c.Request.Context(), tenantID, c.Param("id"), body.Label, body.RequestID); err != nil {
		if errors.Is(err, domainedge.ErrLabelFeedbackNotConfigured) {
			c.JSON(http.StatusNotImplemented, gin.H{"error": "label feedback not configured"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) edgeLatestDrift(c *gin.Context) {
	svc := h.requireEdge()
	if svc == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "edge deployment not configured"})
		return
	}
	rep, err := svc.LatestDrift(c.Request.Context(), h.principalTenant(c), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "no drift report"})
		return
	}
	c.JSON(http.StatusOK, rep)
}