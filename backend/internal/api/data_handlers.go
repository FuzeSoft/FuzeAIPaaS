package api

import (
	"encoding/json"
	"net/http"

	"fuze-ai-paas/backend/internal/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func (h *Handler) requireData(c *gin.Context) bool {
	if h.data == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "data processing not enabled (missing adapter)"})
		return false
	}
	return true
}

type createPipelineRequest struct {
	Name        string                `json:"name"`
	Description string                `json:"description"`
	DatasetName string                `json:"dataset_name"`
	MountPath   string                `json:"mount_path"`
	Priority    int                   `json:"priority"`
	QueueName   string                `json:"queue_name"`
	ClusterID   string                `json:"cluster_id"`
	Steps       []createStepRequest   `json:"steps"`
}

type createStepRequest struct {
	Name      string   `json:"name"`
	Kind      string   `json:"kind"` 
	Operator  string   `json:"operator"`
	DependsOn []string `json:"depends_on"`
	InputPath string   `json:"input_path"`
	OutputPath string  `json:"output_path"`
	Params    string   `json:"params"`
	Image     string   `json:"image"`
	Command   string   `json:"command"`
	GPUs      int      `json:"gpus"`
	Memory    int      `json:"memory"`
}

func (h *Handler) CreateDataPipeline(c *gin.Context) {
	if !h.requireData(c) {
		return
	}
	var req createPipelineRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.MountPath == "" {
		req.MountPath = "/mnt/data"
	}
	
	tenant, ok := h.requireWriteTenant(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing tenant context"})
		return
	}
	pipeline := &models.DataPipeline{
		ID:          "pipe-" + uuid.NewString(),
		TenantID:    tenant,
		Name:        req.Name,
		Description: req.Description,
		DatasetName: req.DatasetName,
		MountPath:   req.MountPath,
		Status:      models.PipelineStatusDraft,
		Priority:    req.Priority,
		QueueName:   req.QueueName,
		ClusterID:   req.ClusterID,
	}
	steps := make([]models.PipelineStep, 0, len(req.Steps))
	for i, s := range req.Steps {
		dep, _ := json.Marshal(s.DependsOn)
		steps[i] = models.PipelineStep{
			ID:         "step-" + uuid.NewString(),
			Name:       s.Name,
			Kind:       models.StepKind(s.Kind),
			Operator:   s.Operator,
			DependsOn:  string(dep),
			InputPath:  s.InputPath,
			OutputPath: s.OutputPath,
			Params:     s.Params,
			Image:      s.Image,
			Command:    s.Command,
			GPUs:       s.GPUs,
			Memory:     s.Memory,
		}
	}
	if err := h.data.CreatePipeline(pipeline, steps); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	h.audit(c, "create", "data-pipeline", pipeline.ID, req.Name)
	c.JSON(http.StatusCreated, pipeline)
}

func (h *Handler) ListDataPipelines(c *gin.Context) {
	if !h.requireData(c) {
		return
	}
	tenant := h.tenantScope(c)
	list, err := h.data.ListPipelines(tenant)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"pipelines": list})
}

func (h *Handler) GetDataPipeline(c *gin.Context) {
	if !h.requireData(c) {
		return
	}
	id := c.Param("id")
	p, steps, err := h.data.GetPipeline(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "pipeline not found"})
		return
	}
	if !h.canAccessTenant(p.TenantID, c) {
		c.JSON(http.StatusNotFound, gin.H{"error": "pipeline not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"pipeline": p, "steps": steps})
}

func (h *Handler) SubmitDataPipeline(c *gin.Context) {
	if !h.requireData(c) {
		return
	}
	id := c.Param("id")
	
	p, _, err := h.data.GetPipeline(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "pipeline not found"})
		return
	}
	if !h.canAccessTenant(p.TenantID, c) {
		c.JSON(http.StatusNotFound, gin.H{"error": "pipeline not found"})
		return
	}
	if err := h.data.SubmitPipeline(id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	h.audit(c, "submit", "data-pipeline", id, "")
	c.JSON(http.StatusOK, gin.H{"id": id, "status": "submitted"})
}

func (h *Handler) CancelDataPipeline(c *gin.Context) {
	if !h.requireData(c) {
		return
	}
	id := c.Param("id")
	
	p, _, err := h.data.GetPipeline(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "pipeline not found"})
		return
	}
	if !h.canAccessTenant(p.TenantID, c) {
		c.JSON(http.StatusNotFound, gin.H{"error": "pipeline not found"})
		return
	}
	if err := h.data.CancelPipeline(id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	h.audit(c, "cancel", "data-pipeline", id, "")
	c.JSON(http.StatusOK, gin.H{"id": id, "status": "cancelled"})
}

type createAnnotationRequest struct {
	Name         string `json:"name"`
	DatasetName  string `json:"dataset_name"`
	DataGlob     string `json:"data_glob"`
	TaskType     string `json:"task_type"`
	Categories   string `json:"categories"`
	Assignee     string `json:"assignee"`
	OutputFormat string `json:"output_format"`
}

func (h *Handler) CreateAnnotation(c *gin.Context) {
	if !h.requireData(c) {
		return
	}
	var req createAnnotationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.OutputFormat == "" {
		req.OutputFormat = "coco"
	}
	
	tenant, ok := h.requireWriteTenant(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing tenant context"})
		return
	}
	a := &models.AnnotationTask{
		ID:           "ann-" + uuid.NewString(),
		TenantID:     tenant,
		Name:         req.Name,
		DatasetName:  req.DatasetName,
		DataGlob:     req.DataGlob,
		TaskType:     req.TaskType,
		Categories:   req.Categories,
		Assignee:     req.Assignee,
		Status:       models.AnnotationStatusOpen,
		OutputFormat: req.OutputFormat,
	}
	if err := h.data.CreateAnnotation(a); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	h.audit(c, "create", "annotation", a.ID, req.Name)
	c.JSON(http.StatusCreated, a)
}

func (h *Handler) ListAnnotations(c *gin.Context) {
	if !h.requireData(c) {
		return
	}
	list, err := h.data.ListAnnotations(h.tenantScope(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"annotations": list})
}

func (h *Handler) GetAnnotation(c *gin.Context) {
	if !h.requireData(c) {
		return
	}
	a, err := h.data.GetAnnotation(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "annotation not found"})
		return
	}
	if !h.canAccessTenant(a.TenantID, c) {
		c.JSON(http.StatusNotFound, gin.H{"error": "annotation not found"})
		return
	}
	c.JSON(http.StatusOK, a)
}

type exportAnnotationRequest struct {
	SrcFormat  string `json:"src_format"` 
	InputPath  string `json:"input_path"` 
	OutputPath string `json:"output_path"`
}

func (h *Handler) ExportAnnotation(c *gin.Context) {
	if !h.requireData(c) {
		return
	}
	id := c.Param("id")
	
	ann, err := h.data.GetAnnotation(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "annotation not found"})
		return
	}
	if !h.canAccessTenant(ann.TenantID, c) {
		c.JSON(http.StatusNotFound, gin.H{"error": "annotation not found"})
		return
	}
	var req exportAnnotationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.data.ExportAnnotation(c.Request.Context(), id, req.SrcFormat, req.InputPath, req.OutputPath); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	a, _ := h.data.GetAnnotation(id)
	h.audit(c, "export", "annotation", id, a.ExportedURI)
	c.JSON(http.StatusOK, gin.H{"id": id, "exported_uri": a.ExportedURI, "status": string(a.Status)})
}