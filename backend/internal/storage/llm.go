package storage

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"sync/atomic"
	"time"

	"fuze-ai-paas/backend/internal/domain/llm"
	"fuze-ai-paas/backend/internal/models"
	"fuze-ai-paas/backend/internal/ports"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var genIDCounter int64

func genID() string {
	n := atomic.AddInt64(&genIDCounter, 1)
	return now().UTC().Format("20060102150405.000000000") + "-" + strconv.FormatInt(n, 10)
}

type llmRouteRepo struct{ db *gorm.DB }

func NewRouteRepository(db *gorm.DB) ports.RouteRepository { return &llmRouteRepo{db: db} }
func (s *Storage) Route() ports.RouteRepository            { return NewRouteRepository(s.db) }

func (r *llmRouteRepo) Save(ctx context.Context, tenantID string, rt llm.Route) error {
	backends, err := json.Marshal(rt.Backends)
	if err != nil {
		return errInvalidJSON("route backends", err)
	}
	row := models.LLMRoute{
		ID:        genID(),
		TenantID:  tenantID,
		Model:     rt.Model,
		Strategy:  rt.Strategy,
		Backends:  string(backends),
		UpdatedAt: now(),
	}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "tenant_id"}, {Name: "model"}},
		DoUpdates: clause.AssignmentColumns([]string{"strategy", "backends", "updated_at"}),
	}).Create(&row).Error
}

func (r *llmRouteRepo) List(ctx context.Context, tenantID string) ([]llm.Route, error) {
	var rows []models.LLMRoute
	q := r.db.WithContext(ctx)
	if tenantID != "" {
		q = q.Where("tenant_id = ?", tenantID)
	}
	if err := q.Order("model ASC").Limit(defaultListLimit).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]llm.Route, 0, len(rows))
	for _, row := range rows {
		rt, err := decodeRoute(row)
		if err != nil {
			return nil, err
		}
		out = append(out, rt)
	}
	return out, nil
}

func (r *llmRouteRepo) Delete(ctx context.Context, tenantID, model string) error {
	res := r.db.WithContext(ctx).Where("tenant_id = ? AND model = ?", tenantID, model).Delete(&models.LLMRoute{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ports.ErrNotFound
	}
	return nil
}

func decodeRoute(row models.LLMRoute) (llm.Route, error) {
	var backends []llm.Backend
	if row.Backends != "" {
		if err := json.Unmarshal([]byte(row.Backends), &backends); err != nil {
			return llm.Route{}, errInvalidJSON("route backends", err)
		}
	}
	return llm.Route{Model: row.Model, Strategy: row.Strategy, Backends: backends}, nil
}

type llmQuotaRepo struct{ db *gorm.DB }

func NewTokenQuotaRepository(db *gorm.DB) ports.TokenQuotaRepository {
	return &llmQuotaRepo{db: db}
}
func (s *Storage) TokenQuota() ports.TokenQuotaRepository { return NewTokenQuotaRepository(s.db) }

func (r *llmQuotaRepo) GetQuota(ctx context.Context, tenantID string) (llm.TokenQuota, error) {
	var q models.LLMTokenQuota
	err := r.db.WithContext(ctx).First(&q, "tenant_id = ?", tenantID).Error
	if isNotFoundErr(err) {
		return llm.TokenQuota{}, ports.ErrNotFound
	}
	if err != nil {
		return llm.TokenQuota{}, err
	}
	return llm.TokenQuota{
		TenantID:    q.TenantID,
		LimitTokens: q.LimitTokens,
		UsedTokens:  q.UsedTokens,
		LimitCost:   q.LimitCost,
		UsedCost:    q.UsedCost,
	}, nil
}

func (r *llmQuotaRepo) SetQuota(ctx context.Context, q llm.TokenQuota) error {
	row := models.LLMTokenQuota{
		TenantID:    q.TenantID,
		LimitTokens: q.LimitTokens,
		UsedTokens:  q.UsedTokens,
		LimitCost:   q.LimitCost,
		UsedCost:    q.UsedCost,
		UpdatedAt:   now(),
	}
	
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "tenant_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"limit_tokens", "limit_cost", "updated_at"}),
	}).Create(&row).Error
}

func (r *llmQuotaRepo) CheckAndConsume(ctx context.Context, tenantID string, tokens int64, cost float64) error {
	return retryTx(r.db, func(tx *gorm.DB) error {
		var q models.LLMTokenQuota
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&q, "tenant_id = ?", tenantID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				
				return nil
			}
			return err
		}
		if q.LimitTokens > 0 && q.UsedTokens+int64(tokens) > q.LimitTokens {
			return llm.ErrTokenQuotaExceeded
		}
		if q.LimitCost > 0 && q.UsedCost+cost > q.LimitCost {
			return llm.ErrTokenQuotaExceeded
		}
		q.UsedTokens += int64(tokens)
		q.UsedCost += cost
		q.UpdatedAt = now()
		return tx.Save(&q).Error
	})
}

func (r *llmQuotaRepo) ListQuotas(ctx context.Context) ([]llm.TokenQuota, error) {
	var rows []models.LLMTokenQuota
	if err := r.db.WithContext(ctx).Order("tenant_id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]llm.TokenQuota, 0, len(rows))
	for _, q := range rows {
		out = append(out, llm.TokenQuota{
			TenantID:    q.TenantID,
			LimitTokens: q.LimitTokens,
			UsedTokens:  q.UsedTokens,
			LimitCost:   q.LimitCost,
			UsedCost:    q.UsedCost,
		})
	}
	return out, nil
}

type llmUsageRepo struct{ db *gorm.DB }

func NewTokenUsageRepository(db *gorm.DB) ports.TokenUsageRepository {
	return &llmUsageRepo{db: db}
}
func (s *Storage) TokenUsage() ports.TokenUsageRepository { return NewTokenUsageRepository(s.db) }

func (r *llmUsageRepo) RecordUsage(ctx context.Context, rec *ports.TokenUsageRecord) error {
	row := models.LLMUsageRecord{
		ID:               generateID(),
		TenantID:         rec.TenantID,
		UserID:           rec.UserID,
		Model:            rec.Model,
		Backend:          rec.Backend,
		PromptTokens:     rec.PromptTokens,
		CompletionTokens: rec.CompletionTokens,
		TotalTokens:      rec.TotalTokens,
		Cost:             rec.Cost,
		LatencyMS:        rec.LatencyMS,
		TTFTMS:           rec.TTFTMS,
		Success:          rec.Success,
		TraceID:          rec.TraceID,
		CreatedAt:        rec.CreatedAt,
	}
	if row.CreatedAt == 0 {
		row.CreatedAt = now().Unix()
	}
	return r.db.WithContext(ctx).Create(&row).Error
}

func (r *llmUsageRepo) SumUsage(ctx context.Context, tenantID string, since, until int64) (llm.Usage, float64, error) {
	row := struct {
		Tokens int64
		Cost   float64
	}{}
	q := r.db.WithContext(ctx).Model(&models.LLMUsageRecord{}).
		Select("COALESCE(SUM(total_tokens),0) AS tokens, COALESCE(SUM(cost),0) AS cost")
	if tenantID != "" {
		q = q.Where("tenant_id = ?", tenantID)
	}
	if since > 0 {
		q = q.Where("created_at >= ?", since)
	}
	if until > 0 {
		q = q.Where("created_at <= ?", until)
	}
	if err := q.Scan(&row).Error; err != nil {
		return llm.Usage{}, 0, err
	}
	return llm.Usage{TotalTokens: int(row.Tokens)}, row.Cost, nil
}

func (r *llmUsageRepo) ListUsage(ctx context.Context, tenantID string, limit int) ([]*ports.TokenUsageRecord, error) {
	var rows []models.LLMUsageRecord
	q := r.db.WithContext(ctx).Model(&models.LLMUsageRecord{})
	if tenantID != "" {
		q = q.Where("tenant_id = ?", tenantID)
	}
	if limit <= 0 {
		limit = 100
	}
	if err := q.Order("created_at DESC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]*ports.TokenUsageRecord, 0, len(rows))
	for i := range rows {
		out = append(out, toUsageRecord(&rows[i]))
	}
	return out, nil
}

func toUsageRecord(m *models.LLMUsageRecord) *ports.TokenUsageRecord {
	return &ports.TokenUsageRecord{
		ID:               m.ID,
		TenantID:         m.TenantID,
		UserID:           m.UserID,
		Model:            m.Model,
		Backend:          m.Backend,
		PromptTokens:     m.PromptTokens,
		CompletionTokens: m.CompletionTokens,
		TotalTokens:      m.TotalTokens,
		Cost:             m.Cost,
		LatencyMS:        m.LatencyMS,
		TTFTMS:           m.TTFTMS,
		Success:          m.Success,
		TraceID:          m.TraceID,
		CreatedAt:        m.CreatedAt,
	}
}

type llmTraceRepo struct{ db *gorm.DB }

func NewTraceRepository(db *gorm.DB) ports.TraceRepository { return &llmTraceRepo{db: db} }
func (s *Storage) Trace() ports.TraceRepository            { return NewTraceRepository(s.db) }

func (r *llmTraceRepo) Save(ctx context.Context, t *llm.Trace) error {
	spans, err := json.Marshal(t.Spans)
	if err != nil {
		return errInvalidJSON("trace spans", err)
	}
	findings, err := json.Marshal(t.Findings)
	if err != nil {
		return errInvalidJSON("trace findings", err)
	}
	row := models.LLMTrace{
		ID:               t.ID,
		TenantID:         t.TenantID,
		UserID:           t.UserID,
		Model:            t.Model,
		Backend:          t.Backend,
		Spans:            string(spans),
		Findings:         string(findings),
		PromptTokens:     t.Usage.PromptTokens,
		CompletionTokens: t.Usage.CompletionTokens,
		TotalTokens:      t.Usage.TotalTokens,
		Cost:             t.Cost,
		TTFTMS:           int64(t.Latency.TTFT / time.Millisecond),
		TPOTMS:           int64(t.Latency.TPOT / time.Millisecond),
		TotalMS:          int64(t.Latency.Total / time.Millisecond),
		TokensPerSecond:  t.Latency.TokensPerSecond,
		Error:            t.Error,
		StartedAt:        t.StartedAt,
		FinishedAt:       t.FinishedAt,
	}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"backend", "spans", "findings", "prompt_tokens", "completion_tokens",
			"total_tokens", "cost", "ttftms", "tpotms", "total_ms",
			"tokens_per_second", "error", "finished_at",
		}),
	}).Create(&row).Error
}

func (r *llmTraceRepo) Get(ctx context.Context, id string) (*llm.Trace, error) {
	var row models.LLMTrace
	err := r.db.WithContext(ctx).First(&row, "id = ?", id).Error
	if isNotFoundErr(err) {
		return nil, ports.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return decodeTrace(row)
}

func (r *llmTraceRepo) List(ctx context.Context, tenantID string, limit int) ([]*llm.Trace, error) {
	var rows []models.LLMTrace
	q := r.db.WithContext(ctx).Model(&models.LLMTrace{})
	if tenantID != "" {
		q = q.Where("tenant_id = ?", tenantID)
	}
	if limit <= 0 {
		limit = 100
	}
	if err := q.Order("started_at DESC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]*llm.Trace, 0, len(rows))
	for _, row := range rows {
		t, err := decodeTrace(row)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, nil
}

func decodeTrace(row models.LLMTrace) (*llm.Trace, error) {
	var spans []llm.Span
	if row.Spans != "" {
		if err := json.Unmarshal([]byte(row.Spans), &spans); err != nil {
			return nil, errInvalidJSON("trace spans", err)
		}
	}
	var findings []llm.Finding
	if row.Findings != "" {
		if err := json.Unmarshal([]byte(row.Findings), &findings); err != nil {
			return nil, errInvalidJSON("trace findings", err)
		}
	}
	return &llm.Trace{
		ID:         row.ID,
		TenantID:   row.TenantID,
		UserID:     row.UserID,
		Model:      row.Model,
		Backend:    row.Backend,
		Spans:      spans,
		Usage:      llm.Usage{PromptTokens: row.PromptTokens, CompletionTokens: row.CompletionTokens, TotalTokens: row.TotalTokens},
		Cost:       row.Cost,
		Findings:   findings,
		Error:      row.Error,
		StartedAt:  row.StartedAt,
		FinishedAt: row.FinishedAt,
		Latency:    llm.LatencyStats{TTFT: time.Duration(row.TTFTMS) * time.Millisecond, TPOT: time.Duration(row.TPOTMS) * time.Millisecond, Total: time.Duration(row.TotalMS) * time.Millisecond, TokensPerSecond: row.TokensPerSecond},
	}, nil
}

type llmPromptRepo struct{ db *gorm.DB }

func NewPromptRepository(db *gorm.DB) ports.PromptRepository { return &llmPromptRepo{db: db} }
func (s *Storage) Prompt() ports.PromptRepository            { return NewPromptRepository(s.db) }

func (r *llmPromptRepo) Create(ctx context.Context, p *llm.Prompt, tenantID, createdBy string) error {
	versions, err := json.Marshal(p.Versions)
	if err != nil {
		return errInvalidJSON("prompt versions", err)
	}
	row := models.LLMPrompt{
		ID:            generateID(),
		TenantID:      tenantID,
		Name:          p.Name,
		Versions:      string(versions),
		ActiveVersion: p.ActiveVersion,
		CreatedBy:     createdBy,
		CreatedAt:     now(),
		UpdatedAt:     now(),
	}
	return r.db.WithContext(ctx).Create(&row).Error
}

func (r *llmPromptRepo) Get(ctx context.Context, tenantID, name string) (*llm.Prompt, error) {
	var row models.LLMPrompt
	err := r.db.WithContext(ctx).First(&row, "tenant_id = ? AND name = ?", tenantID, name).Error
	if isNotFoundErr(err) {
		return nil, ports.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return decodePrompt(row)
}

func (r *llmPromptRepo) List(ctx context.Context, tenantID string) ([]*llm.Prompt, error) {
	var rows []models.LLMPrompt
	q := r.db.WithContext(ctx)
	if tenantID != "" {
		q = q.Where("tenant_id = ?", tenantID)
	}
	if err := q.Order("created_at DESC").Limit(defaultListLimit).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]*llm.Prompt, 0, len(rows))
	for _, row := range rows {
		p, err := decodePrompt(row)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

func (r *llmPromptRepo) Update(ctx context.Context, tenantID string, p *llm.Prompt) error {
	versions, err := json.Marshal(p.Versions)
	if err != nil {
		return errInvalidJSON("prompt versions", err)
	}
	res := r.db.WithContext(ctx).Model(&models.LLMPrompt{}).
		Where("tenant_id = ? AND name = ?", tenantID, p.Name).
		Updates(map[string]any{
			"versions":       string(versions),
			"active_version": p.ActiveVersion,
			"updated_at":     now(),
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ports.ErrNotFound
	}
	return nil
}

func (r *llmPromptRepo) Delete(ctx context.Context, tenantID, name string) error {
	res := r.db.WithContext(ctx).Where("tenant_id = ? AND name = ?", tenantID, name).Delete(&models.LLMPrompt{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ports.ErrNotFound
	}
	return nil
}

func decodePrompt(row models.LLMPrompt) (*llm.Prompt, error) {
	var versions []llm.PromptVersion
	if row.Versions != "" {
		if err := json.Unmarshal([]byte(row.Versions), &versions); err != nil {
			return nil, errInvalidJSON("prompt versions", err)
		}
	}
	return &llm.Prompt{
		Name:          row.Name,
		Versions:      versions,
		ActiveVersion: row.ActiveVersion,
	}, nil
}

type llmKnowledgeRepo struct{ db *gorm.DB }

func NewKnowledgeRepository(db *gorm.DB) ports.KnowledgeRepository {
	return &llmKnowledgeRepo{db: db}
}
func (s *Storage) Knowledge() ports.KnowledgeRepository { return NewKnowledgeRepository(s.db) }

func (r *llmKnowledgeRepo) CreateBase(ctx context.Context, kb *ports.KnowledgeBase) error {
	row := models.LLMKnowledgeBase{
		ID:             generateID(),
		Name:           kb.Name,
		TenantID:       kb.TenantID,
		EmbeddingModel: kb.EmbeddingModel,
		Dimension:      kb.Dimension,
		ChunkSize:      kb.ChunkSize,
		Overlap:        kb.Overlap,
		CreatedBy:      kb.CreatedBy,
		CreatedAt:      now(),
	}
	kb.ID = row.ID
	return r.db.WithContext(ctx).Create(&row).Error
}

func (r *llmKnowledgeRepo) GetBase(ctx context.Context, id string) (*ports.KnowledgeBase, error) {
	var row models.LLMKnowledgeBase
	err := r.db.WithContext(ctx).First(&row, "id = ?", id).Error
	if isNotFoundErr(err) {
		return nil, ports.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &ports.KnowledgeBase{
		ID:             row.ID,
		Name:           row.Name,
		TenantID:       row.TenantID,
		EmbeddingModel: row.EmbeddingModel,
		Dimension:      row.Dimension,
		ChunkSize:      row.ChunkSize,
		Overlap:        row.Overlap,
		CreatedBy:      row.CreatedBy,
	}, nil
}

func (r *llmKnowledgeRepo) ListBases(ctx context.Context, tenantID string) ([]*ports.KnowledgeBase, error) {
	var rows []models.LLMKnowledgeBase
	q := r.db.WithContext(ctx)
	if tenantID != "" {
		q = q.Where("tenant_id = ?", tenantID)
	}
	if err := q.Order("created_at DESC").Limit(defaultListLimit).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]*ports.KnowledgeBase, 0, len(rows))
	for i := range rows {
		out = append(out, &ports.KnowledgeBase{
			ID:             rows[i].ID,
			Name:           rows[i].Name,
			TenantID:       rows[i].TenantID,
			EmbeddingModel: rows[i].EmbeddingModel,
			Dimension:      rows[i].Dimension,
			ChunkSize:      rows[i].ChunkSize,
			Overlap:        rows[i].Overlap,
			CreatedBy:      rows[i].CreatedBy,
		})
	}
	return out, nil
}

func (r *llmKnowledgeRepo) DeleteBase(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("base_id = ?", id).Delete(&models.LLMDocument{}).Error; err != nil {
			return err
		}
		res := tx.Where("id = ?", id).Delete(&models.LLMKnowledgeBase{})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ports.ErrNotFound
		}
		return nil
	})
}

func (r *llmKnowledgeRepo) AddDocument(ctx context.Context, doc *ports.KnowledgeDocument) error {
	row := models.LLMDocument{
		ID:        generateID(),
		BaseID:    doc.BaseID,
		Title:     doc.Title,
		Source:    doc.Source,
		Content:   doc.Content,
		Segments:  doc.Segments,
		Status:    doc.Status,
		CreatedAt: now(),
	}
	doc.ID = row.ID
	return r.db.WithContext(ctx).Create(&row).Error
}

func (r *llmKnowledgeRepo) GetDocument(ctx context.Context, id string) (*ports.KnowledgeDocument, error) {
	var row models.LLMDocument
	err := r.db.WithContext(ctx).First(&row, "id = ?", id).Error
	if isNotFoundErr(err) {
		return nil, ports.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &ports.KnowledgeDocument{
		ID:       row.ID,
		BaseID:   row.BaseID,
		Title:    row.Title,
		Source:   row.Source,
		Content:  row.Content,
		Segments: row.Segments,
		Status:   row.Status,
	}, nil
}

func (r *llmKnowledgeRepo) ListDocuments(ctx context.Context, baseID string) ([]*ports.KnowledgeDocument, error) {
	var rows []models.LLMDocument
	if err := r.db.WithContext(ctx).Where("base_id = ?", baseID).
		Order("created_at ASC").Limit(defaultListLimit).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]*ports.KnowledgeDocument, 0, len(rows))
	for i := range rows {
		out = append(out, &ports.KnowledgeDocument{
			ID:       rows[i].ID,
			BaseID:   rows[i].BaseID,
			Title:    rows[i].Title,
			Source:   rows[i].Source,
			Content:  rows[i].Content,
			Segments: rows[i].Segments,
			Status:   rows[i].Status,
		})
	}
	return out, nil
}

func (r *llmKnowledgeRepo) DeleteDocument(ctx context.Context, id string) error {
	res := r.db.WithContext(ctx).Where("id = ?", id).Delete(&models.LLMDocument{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ports.ErrNotFound
	}
	return nil
}

func retryTx(db *gorm.DB, fn func(tx *gorm.DB) error) error {
	const maxAttempts = 5
	var lastErr error
	for i := 0; i < maxAttempts; i++ {
		if err := db.Transaction(fn); err != nil {
			lastErr = err
			
			if errors.Is(err, gorm.ErrRecordNotFound) || errors.Is(err, llm.ErrTokenQuotaExceeded) {
				return err
			}
			
			if !isTransientLockError(err) {
				return err
			}
			time.Sleep(time.Duration(i+1) * 10 * time.Millisecond)
			continue
		}
		return nil
	}
	return lastErr
}

func errInvalidJSON(what string, err error) error {
	return errors.New("llm storage: invalid json for " + what + ": " + err.Error())
}