package ports

import (
	"context"
	"errors"

	"fuze-ai-paas/backend/internal/domain/llm"
	"fuze-ai-paas/backend/internal/models"
)

type ChatCompleter interface {
	
	Complete(ctx context.Context, endpoint string, req llm.ChatRequest) (llm.ChatResponse, error)
	
	Stream(ctx context.Context, endpoint string, req llm.ChatRequest, onChunk func(llm.Chunk) error) (llm.Usage, error)
}

type Embedder interface {
	
	Embed(ctx context.Context, endpoint string, req llm.EmbeddingRequest) (llm.EmbeddingResponse, error)
}

type VectorStore interface {
	
	Upsert(ctx context.Context, collection string, items []VectorItem) error
	
	Search(ctx context.Context, collection string, vector []float32, topK int) ([]VectorHit, error)
	
	Delete(ctx context.Context, collection, documentID string) error
	
	DropCollection(ctx context.Context, collection string) error
}

type VectorItem struct {
	
	ID string
	
	DocumentID string
	
	Title string
	
	Segment llm.Segment
	
	Vector []float32
}

type VectorHit struct {
	Item  VectorItem
	Score float64
}

type Reranker interface {
	
	Rerank(ctx context.Context, query string, hits []llm.ScoredSegment) ([]llm.ScoredSegment, error)
}

type PromptRepository interface {
	Create(ctx context.Context, p *llm.Prompt, tenantID, createdBy string) error
	Get(ctx context.Context, tenantID, name string) (*llm.Prompt, error)
	List(ctx context.Context, tenantID string) ([]*llm.Prompt, error)
	Update(ctx context.Context, tenantID string, p *llm.Prompt) error
	Delete(ctx context.Context, tenantID, name string) error
}

var ErrGuardrailRuleNotFound = errors.New("guardrail rule not found")

var ErrGuardrailInvalidRule = errors.New("invalid guardrail rule")

var ErrAlertRuleNotFound = errors.New("alert rule not found")

var ErrAlertRuleInvalid = errors.New("invalid alert rule")

var ErrAlertSilenceNotFound = errors.New("alert silence not found")

var ErrAlertSilenceInvalid = errors.New("invalid alert silence")

type GuardrailRepository interface {
	Resolve(ctx context.Context, tenantID string) ([]llm.Rule, error)
	List(ctx context.Context, tenantID string) ([]models.LLMGuardrailRule, error)
	Upsert(ctx context.Context, rule *models.LLMGuardrailRule) error
	Delete(ctx context.Context, tenantID, id string) error
}

type KnowledgeRepository interface {
	CreateBase(ctx context.Context, kb *KnowledgeBase) error
	GetBase(ctx context.Context, id string) (*KnowledgeBase, error)
	ListBases(ctx context.Context, tenantID string) ([]*KnowledgeBase, error)
	DeleteBase(ctx context.Context, id string) error

	AddDocument(ctx context.Context, doc *KnowledgeDocument) error
	GetDocument(ctx context.Context, id string) (*KnowledgeDocument, error)
	ListDocuments(ctx context.Context, baseID string) ([]*KnowledgeDocument, error)
	DeleteDocument(ctx context.Context, id string) error
}

type KnowledgeBase struct {
	ID       string
	Name     string
	TenantID string
	
	EmbeddingModel string
	
	Dimension int
	ChunkSize int
	Overlap   int
	CreatedBy string
}

type KnowledgeDocument struct {
	ID       string
	BaseID   string
	Title    string
	Source   string
	Content  string
	Segments int
	Status   string
}

type TokenUsageRepository interface {
	
	RecordUsage(ctx context.Context, rec *TokenUsageRecord) error
	
	SumUsage(ctx context.Context, tenantID string, since, until int64) (llm.Usage, float64, error)
	
	ListUsage(ctx context.Context, tenantID string, limit int) ([]*TokenUsageRecord, error)
}

type TokenUsageRecord struct {
	ID               string
	TenantID         string
	UserID           string
	Model            string
	Backend          string
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	Cost             float64
	
	LatencyMS int64
	
	TTFTMS  int64
	Success bool
	TraceID string
	
	CreatedAt int64
}

type CostRepository interface {
	
	TokenQuotaRepository
	
	RecordGPUCost(ctx context.Context, rec *models.GPUUsageRecord) error
	
	SumGPUUsage(ctx context.Context, tenantID string, since, until int64) (hours float64, cost float64, err error)
}

type TokenQuotaRepository interface {
	GetQuota(ctx context.Context, tenantID string) (llm.TokenQuota, error)
	SetQuota(ctx context.Context, q llm.TokenQuota) error
	
	CheckAndConsume(ctx context.Context, tenantID string, tokens int64, cost float64) error
	ListQuotas(ctx context.Context) ([]llm.TokenQuota, error)
}

type TraceRepository interface {
	Save(ctx context.Context, t *llm.Trace) error
	Get(ctx context.Context, id string) (*llm.Trace, error)
	List(ctx context.Context, tenantID string, limit int) ([]*llm.Trace, error)
}

type RouteRepository interface {
	Save(ctx context.Context, tenantID string, r llm.Route) error
	List(ctx context.Context, tenantID string) ([]llm.Route, error)
	Delete(ctx context.Context, tenantID, model string) error
}