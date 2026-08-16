package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"fuze-ai-paas/backend/internal/app/llmgateway"
	"fuze-ai-paas/backend/internal/domain/llm"
	"fuze-ai-paas/backend/internal/ports"
	"github.com/gin-gonic/gin"
)

func (h *Handler) ListRoutes(c *gin.Context) {
	if h.llmRoute == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "route repository not configured"})
		return
	}
	routes, err := h.llmRoute.List(c.Request.Context(), h.tenantScope(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"routes": routes})
}

func (h *Handler) UpsertRoute(c *gin.Context) {
	if h.llmRoute == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "route repository not configured"})
		return
	}
	var rt llm.Route
	if err := c.ShouldBindJSON(&rt); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid route body"})
		return
	}
	if rt.Model == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "model is required"})
		return
	}
	for i := range rt.Backends {
		if rt.Backends[i].Name == "" {
			rt.Backends[i].Name = rt.Backends[i].Endpoint
		}
		rt.Backends[i].Healthy = true 
	}
	tenant := h.tenantScope(c)
	if tenant == "" {
		tenant = h.principalTenant(c)
	}
	if err := h.llmRoute.Save(c.Request.Context(), tenant, rt); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"model": rt.Model, "backends": len(rt.Backends)})
}

func (h *Handler) DeleteRoute(c *gin.Context) {
	if h.llmRoute == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "route repository not configured"})
		return
	}
	model := c.Param("model")
	tenant := h.tenantScope(c)
	if tenant == "" {
		tenant = h.principalTenant(c)
	}
	if err := h.llmRoute.Delete(c.Request.Context(), tenant, model); err != nil {
		respondLLMError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": model})
}

func (h *Handler) LLMGetQuota(c *gin.Context) {
	if h.llmQuota == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "quota repository not configured"})
		return
	}
	q, err := h.llmQuota.GetQuota(c.Request.Context(), h.principalTenant(c))
	if err != nil {
		respondLLMError(c, err)
		return
	}
	c.JSON(http.StatusOK, q)
}

func (h *Handler) LLMSetQuota(c *gin.Context) {
	if h.llmQuota == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "quota repository not configured"})
		return
	}
	var q llm.TokenQuota
	if err := c.ShouldBindJSON(&q); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid quota body"})
		return
	}
	tenantID, ok := h.requireWriteTenant(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing tenant context"})
		return
	}
	q.TenantID = tenantID
	if err := h.llmQuota.SetQuota(c.Request.Context(), q); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, q)
}

func (h *Handler) SumUsage(c *gin.Context) {
	if h.llmUsage == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "usage repository not configured"})
		return
	}
	var since, until int64
	parseInt64Query(c.Query("since"), &since)
	parseInt64Query(c.Query("until"), &until)
	usage, cost, err := h.llmUsage.SumUsage(c.Request.Context(), h.principalTenant(c), since, until)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"usage": usage, "cost": cost})
}

func (h *Handler) ListUsage(c *gin.Context) {
	if h.llmUsage == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "usage repository not configured"})
		return
	}
	limit := queryLimit(c, 100)
	recs, err := h.llmUsage.ListUsage(c.Request.Context(), h.principalTenant(c), limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"records": recs})
}

func (h *Handler) ListTraces(c *gin.Context) {
	if h.llmTrace == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "trace repository not configured"})
		return
	}
	limit := queryLimit(c, 100)
	traces, err := h.llmTrace.List(c.Request.Context(), h.tenantScope(c), limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"traces": traces})
}

func (h *Handler) GetTrace(c *gin.Context) {
	if h.llmTrace == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "trace repository not configured"})
		return
	}
	t, err := h.llmTrace.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		respondLLMError(c, err)
		return
	}
	if !h.canAccessTenant(t.TenantID, c) {
		c.JSON(http.StatusNotFound, gin.H{"error": "trace not found"})
		return
	}
	c.JSON(http.StatusOK, t)
}

func (h *Handler) ListPrompts(c *gin.Context) {
	if h.llmPrompt == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "prompt repository not configured"})
		return
	}
	list, err := h.llmPrompt.List(c.Request.Context(), h.tenantScope(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"prompts": list})
}

func (h *Handler) CreatePrompt(c *gin.Context) {
	if h.llmPrompt == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "prompt repository not configured"})
		return
	}
	var body struct {
		Name    string `json:"name"`
		Content string `json:"content"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid prompt body: name required"})
		return
	}
	p := &llm.Prompt{
		Name: body.Name,
		Versions: []llm.PromptVersion{
			{Version: 1, Content: body.Content},
		},
		ActiveVersion: 1,
	}
	tenantID, ok := h.requireWriteTenant(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing tenant context"})
		return
	}
	if err := h.llmPrompt.Create(c.Request.Context(), p, tenantID, h.claimsOf(c).UserID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, p)
}

func (h *Handler) GetPrompt(c *gin.Context) {
	if h.llmPrompt == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "prompt repository not configured"})
		return
	}
	p, err := h.llmPrompt.Get(c.Request.Context(), h.principalTenant(c), c.Param("name"))
	if err != nil {
		respondLLMError(c, err)
		return
	}
	c.JSON(http.StatusOK, p)
}

func (h *Handler) AddPromptVersion(c *gin.Context) {
	if h.llmPrompt == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "prompt repository not configured"})
		return
	}
	var body struct {
		Content  string `json:"content"`
		Activate bool   `json:"activate"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.Content == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid version body: content required"})
		return
	}
	tenantID, ok := h.requireWriteTenant(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing tenant context"})
		return
	}
	existing, err := h.llmPrompt.Get(c.Request.Context(), tenantID, c.Param("name"))
	if err != nil {
		respondLLMError(c, err)
		return
	}
	next := 1
	for _, v := range existing.Versions {
		if v.Version >= next {
			next = v.Version + 1
		}
	}
	existing.Versions = append(existing.Versions, llm.PromptVersion{
		Version: next,
		Content: body.Content,
	})
	if body.Activate {
		existing.ActiveVersion = next
	}
	if err := h.llmPrompt.Update(c.Request.Context(), h.principalTenant(c), existing); err != nil {
		respondLLMError(c, err)
		return
	}
	c.JSON(http.StatusOK, existing)
}

func (h *Handler) ActivatePrompt(c *gin.Context) {
	if h.llmPrompt == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "prompt repository not configured"})
		return
	}
	var body struct {
		Version int `json:"version"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	tenantID, ok := h.requireWriteTenant(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing tenant context"})
		return
	}
	existing, err := h.llmPrompt.Get(c.Request.Context(), tenantID, c.Param("name"))
	if err != nil {
		respondLLMError(c, err)
		return
	}
	found := false
	for _, v := range existing.Versions {
		if v.Version == body.Version {
			found = true
			break
		}
	}
	if !found {
		c.JSON(http.StatusBadRequest, gin.H{"error": "version not found"})
		return
	}
	existing.ActiveVersion = body.Version
	if err := h.llmPrompt.Update(c.Request.Context(), h.principalTenant(c), existing); err != nil {
		respondLLMError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"name": existing.Name, "active_version": body.Version})
}

func (h *Handler) DeletePrompt(c *gin.Context) {
	if h.llmPrompt == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "prompt repository not configured"})
		return
	}
	tenantID, ok := h.requireWriteTenant(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing tenant context"})
		return
	}
	if err := h.llmPrompt.Delete(c.Request.Context(), tenantID, c.Param("name")); err != nil {
		respondLLMError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": c.Param("name")})
}

func (h *Handler) ListKnowledgeBases(c *gin.Context) {
	if h.llmKnowledge == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "knowledge repository not configured"})
		return
	}
	list, err := h.llmKnowledge.ListBases(c.Request.Context(), h.tenantScope(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"bases": list})
}

func (h *Handler) CreateKnowledgeBase(c *gin.Context) {
	if h.llmKnowledge == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "knowledge repository not configured"})
		return
	}
	var kb ports.KnowledgeBase
	if err := c.ShouldBindJSON(&kb); err != nil || kb.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid knowledge base body: name required"})
		return
	}
	tenantID, ok := h.requireWriteTenant(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing tenant context"})
		return
	}
	kb.TenantID = tenantID
	kb.CreatedBy = h.claimsOf(c).UserID
	if kb.ChunkSize <= 0 {
		kb.ChunkSize = 512
	}
	if kb.Overlap < 0 {
		kb.Overlap = 0
	}
	
	if err := (llm.ChunkOptions{Size: kb.ChunkSize, Overlap: kb.Overlap}.Validate()); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.llmKnowledge.CreateBase(c.Request.Context(), &kb); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, kb)
}

func (h *Handler) GetKnowledgeBase(c *gin.Context) {
	if h.llmKnowledge == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "knowledge repository not configured"})
		return
	}
	kb, err := h.llmKnowledge.GetBase(c.Request.Context(), c.Param("id"))
	if err != nil {
		respondLLMError(c, err)
		return
	}
	if !h.canAccessTenant(kb.TenantID, c) {
		c.JSON(http.StatusNotFound, gin.H{"error": "knowledge base not found"})
		return
	}
	c.JSON(http.StatusOK, kb)
}

func (h *Handler) DeleteKnowledgeBase(c *gin.Context) {
	if h.llmKnowledge == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "knowledge repository not configured"})
		return
	}
	baseID := c.Param("id")
	
	kb, err := h.llmKnowledge.GetBase(c.Request.Context(), baseID)
	if err != nil {
		respondLLMError(c, err)
		return
	}
	if !h.canAccessTenant(kb.TenantID, c) {
		c.JSON(http.StatusNotFound, gin.H{"error": "knowledge base not found"})
		return
	}
	if err := h.llmKnowledge.DeleteBase(c.Request.Context(), baseID); err != nil {
		respondLLMError(c, err)
		return
	}
	
	if h.vectorStore != nil {
		if err := h.vectorStore.DropCollection(c.Request.Context(), baseID); err != nil {
			c.JSON(http.StatusOK, gin.H{"deleted": baseID, "vector_drop_warning": err.Error()})
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"deleted": baseID})
}

func (h *Handler) AddKnowledgeDocument(c *gin.Context) {
	if h.llmKnowledge == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "knowledge repository not configured"})
		return
	}
	baseID := c.Param("id")
	kb, err := h.llmKnowledge.GetBase(c.Request.Context(), baseID)
	if err != nil {
		respondLLMError(c, err)
		return
	}
	if !h.canAccessTenant(kb.TenantID, c) {
		c.JSON(http.StatusNotFound, gin.H{"error": "knowledge base not found"})
		return
	}
	var body struct {
		Title   string `json:"title"`
		Source  string `json:"source"`
		Content string `json:"content"`
	}
	if err = c.ShouldBindJSON(&body); err != nil || body.Content == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid document body: content required"})
		return
	}
	
	segs, err := llm.SplitText(body.Content, llm.ChunkOptions{
		Size:       kb.ChunkSize,
		Overlap:    kb.Overlap,
		Separators: []string{"\n"},
	})
	if err != nil {
		
		c.JSON(http.StatusBadRequest, gin.H{"error": "split document failed: " + err.Error()})
		return
	}
	doc := &ports.KnowledgeDocument{
		BaseID:   baseID,
		Title:    body.Title,
		Source:   body.Source,
		Content:  body.Content,
		Segments: len(segs),
		Status:   "indexed", 
	}
	if err := h.llmKnowledge.AddDocument(c.Request.Context(), doc); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	
	if h.vectorStore != nil && h.embedder != nil {
		if err := h.indexDocumentVectors(c.Request.Context(), baseID, doc, segs); err != nil {
			
			c.JSON(http.StatusOK, gin.H{"document": doc, "vector_index_warning": err.Error()})
			return
		}
		doc.Status = "indexed"
	}
	c.JSON(http.StatusOK, doc)
}

func (h *Handler) indexDocumentVectors(ctx context.Context, baseID string, doc *ports.KnowledgeDocument, segs []llm.Segment) error {
	texts := make([]string, len(segs))
	for i, s := range segs {
		texts[i] = s.Content
	}
	resp, err := h.embedder.Embed(ctx, "", llm.EmbeddingRequest{Model: "default", Input: texts})
	if err != nil {
		return fmt.Errorf("embed segments: %w", err)
	}
	items := make([]ports.VectorItem, 0, len(segs))
	for i, s := range segs {
		var vec []float32
		if i < len(resp.Data) {
			vec = resp.Data[i].Vector
		}
		items = append(items, ports.VectorItem{
			ID:         fmt.Sprintf("%s#%d", doc.ID, i),
			DocumentID: doc.ID,
			Title:      doc.Title,
			Segment:    s,
			Vector:     vec,
		})
	}
	if err := h.vectorStore.Upsert(ctx, baseID, items); err != nil {
		return fmt.Errorf("upsert vectors: %w", err)
	}
	return nil
}

func (h *Handler) ListKnowledgeDocuments(c *gin.Context) {
	if h.llmKnowledge == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "knowledge repository not configured"})
		return
	}
	baseID := c.Param("id")
	
	kb, err := h.llmKnowledge.GetBase(c.Request.Context(), baseID)
	if err != nil {
		respondLLMError(c, err)
		return
	}
	if !h.canAccessTenant(kb.TenantID, c) {
		c.JSON(http.StatusNotFound, gin.H{"error": "knowledge base not found"})
		return
	}
	docs, err := h.llmKnowledge.ListDocuments(c.Request.Context(), baseID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"documents": docs})
}

func (h *Handler) DeleteKnowledgeDocument(c *gin.Context) {
	if h.llmKnowledge == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "knowledge repository not configured"})
		return
	}
	baseID := c.Param("id")
	docID := c.Param("docId")
	
	kb, err := h.llmKnowledge.GetBase(c.Request.Context(), baseID)
	if err != nil {
		respondLLMError(c, err)
		return
	}
	if !h.canAccessTenant(kb.TenantID, c) {
		c.JSON(http.StatusNotFound, gin.H{"error": "knowledge base not found"})
		return
	}
	if err := h.llmKnowledge.DeleteDocument(c.Request.Context(), docID); err != nil {
		respondLLMError(c, err)
		return
	}
	
	if h.vectorStore != nil {
		if err := h.vectorStore.Delete(c.Request.Context(), c.Param("id"), docID); err != nil {
			c.JSON(http.StatusOK, gin.H{"deleted": docID, "vector_delete_warning": err.Error()})
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"deleted": docID})
}

func (h *Handler) LLMChat(c *gin.Context) {
	if h.llmgw == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "llm gateway backend not configured (adapter pending)",
		})
		return
	}
	var req llm.ChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid chat request"})
		return
	}
	
	userID := h.claimsOf(c).UserID
	req.User = userID

	if req.Stream {
		h.streamLLMChat(c, req, userID)
		return
	}

	res, err := h.llmgw.Complete(c.Request.Context(), llmgateway.Request{
		Chat:     req,
		TenantID: h.principalTenant(c),
		UserID:   userID,
	})
	if err != nil {
		respondLLMError(c, err)
		return
	}
	c.JSON(http.StatusOK, res.Response)
}

func (h *Handler) streamLLMChat(c *gin.Context, req llm.ChatRequest, userID string) {
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		respondLLMError(c, errors.New("streaming unsupported: response writer is not flushable"))
		return
	}

	var written bool
	
	commitHeaders := func() {
		if written {
			return
		}
		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")
		c.Header("X-Accel-Buffering", "no") 
		written = true
	}

	_, err := h.llmgw.Stream(c.Request.Context(), llmgateway.Request{
		Chat:     req,
		TenantID: h.principalTenant(c),
		UserID:   userID,
	}, func(ch llm.Chunk) error {
		data, e := json.Marshal(ch)
		if e != nil {
			return e
		}
		commitHeaders()
		
		if _, e := c.Writer.Write([]byte("data: ")); e != nil {
			return e
		}
		if _, e := c.Writer.Write(data); e != nil {
			return e
		}
		if _, e := c.Writer.Write([]byte("\n\n")); e != nil {
			return e
		}
		flusher.Flush()
		return nil
	})
	if err != nil {
		if !written {
			
			respondLLMError(c, err)
			return
		}
		
		data, _ := json.Marshal(gin.H{"error": err.Error()})
		c.Writer.Write([]byte("data: "))
		c.Writer.Write(data)
		c.Writer.Write([]byte("\n\n"))
		flusher.Flush()
		return
	}
	
	commitHeaders()
	c.Writer.Write([]byte("data: [DONE]\n\n"))
	flusher.Flush()
}

func (h *Handler) OpenAIChat(c *gin.Context) {
	h.LLMChat(c)
}

func respondLLMError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, llm.ErrModelNotFound):
		
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	case errors.Is(err, llm.ErrNoHealthyBackend):
		
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		return
	case errors.Is(err, llm.ErrTokenQuotaExceeded):
		
		c.JSON(http.StatusTooManyRequests, gin.H{"error": err.Error()})
		return
	case errors.Is(err, ports.ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "resource not found"})
		return
	default:
		
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	}
}

func queryLimit(c *gin.Context, max int) int {
	s := c.Query("limit")
	if s == "" {
		return max
	}
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return max
	}
	if n > max {
		return max
	}
	return n
}

func parseInt64Query(s string, out *int64) {
	if s == "" || out == nil {
		return
	}
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		*out = n
	}
}