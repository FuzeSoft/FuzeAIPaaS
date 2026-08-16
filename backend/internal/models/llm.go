package models

import "time"

type LLMRoute struct {
	ID       string `gorm:"primaryKey" json:"id"`
	TenantID string `gorm:"uniqueIndex:idx_llm_route_tenant_model" json:"tenant_id"`
	
	Model string `gorm:"uniqueIndex:idx_llm_route_tenant_model" json:"model"`
	
	Strategy string `json:"strategy"`
	
	Backends  string    `json:"backends"`
	CreatedBy string    `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (LLMRoute) TableName() string { return "llm_routes" }

type LLMPrice struct {
	ID    string `gorm:"primaryKey" json:"id"`
	Model string `gorm:"uniqueIndex" json:"model"`
	
	InputPer1K float64 `json:"input_per_1k"`
	
	OutputPer1K float64   `json:"output_per_1k"`
	Currency    string    `json:"currency"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (LLMPrice) TableName() string { return "llm_prices" }

type GPUPrice struct {
	ID              string    `gorm:"primaryKey" json:"id"`
	GPUType         string    `gorm:"uniqueIndex" json:"gpu_type"`
	PricePerGPUHour float64   `json:"price_per_gpu_hour"`
	Currency        string    `json:"currency"`
	UpdatedAt       time.Time `json:"updated_at"`
}

func (GPUPrice) TableName() string { return "gpu_prices" }

type LLMTokenQuota struct {
	TenantID    string    `gorm:"primaryKey" json:"tenant_id"`
	LimitTokens int64     `json:"limit_tokens"`
	UsedTokens  int64     `json:"used_tokens"`
	LimitCost   float64   `json:"limit_cost"`
	UsedCost    float64   `json:"used_cost"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (LLMTokenQuota) TableName() string { return "llm_token_quotas" }

type LLMUsageRecord struct {
	ID               string  `gorm:"primaryKey" json:"id"`
	TenantID         string  `gorm:"index:idx_llm_usage_tenant_time" json:"tenant_id"`
	UserID           string  `gorm:"index" json:"user_id"`
	Model            string  `gorm:"index" json:"model"`
	Backend          string  `json:"backend"`
	PromptTokens     int     `json:"prompt_tokens"`
	CompletionTokens int     `json:"completion_tokens"`
	TotalTokens      int     `json:"total_tokens"`
	Cost             float64 `json:"cost"`
	LatencyMS        int64   `json:"latency_ms"`
	TTFTMS           int64   `json:"ttft_ms"`
	Success          bool    `json:"success"`
	TraceID          string  `gorm:"index" json:"trace_id"`
	
	CreatedAt int64 `gorm:"index:idx_llm_usage_tenant_time" json:"created_at"`
}

func (LLMUsageRecord) TableName() string { return "llm_usage_records" }

type GPUUsageRecord struct {
	ID        string  `gorm:"primaryKey" json:"id"`
	TenantID  string  `gorm:"index:idx_gpu_usage_tenant_time" json:"tenant_id"`
	JobID     string  `gorm:"uniqueIndex" json:"job_id"`
	GPUType   string  `json:"gpu_type"`
	GPUCount  int     `json:"gpu_count"`
	Hours     float64 `json:"hours"`
	Cost      float64 `json:"cost"`
	Currency  string  `json:"currency"`
	Status    string  `json:"status"` 
	CreatedAt int64   `gorm:"index:idx_gpu_usage_tenant_time" json:"created_at"`
}

func (GPUUsageRecord) TableName() string { return "gpu_usage_records" }

type LLMTrace struct {
	ID       string `gorm:"primaryKey" json:"id"`
	TenantID string `gorm:"index" json:"tenant_id"`
	UserID   string `json:"user_id"`
	Model    string `gorm:"index" json:"model"`
	Backend  string `json:"backend"`
	
	Spans string `json:"spans"`
	
	Findings         string  `json:"findings"`
	PromptTokens     int     `json:"prompt_tokens"`
	CompletionTokens int     `json:"completion_tokens"`
	TotalTokens      int     `json:"total_tokens"`
	Cost             float64 `json:"cost"`
	
	TTFTMS  int64 `json:"ttft_ms"`
	TPOTMS  int64 `json:"tpot_ms"`
	TotalMS int64 `json:"total_ms"`
	
	TokensPerSecond float64   `json:"tokens_per_second"`
	Error           string    `json:"error,omitempty"`
	StartedAt       time.Time `gorm:"index" json:"started_at"`
	FinishedAt      time.Time `json:"finished_at"`
}

func (LLMTrace) TableName() string { return "llm_traces" }

type LLMPrompt struct {
	ID       string `gorm:"primaryKey" json:"id"`
	TenantID string `gorm:"uniqueIndex:idx_llm_prompt_tenant_name" json:"tenant_id"`
	Name     string `gorm:"uniqueIndex:idx_llm_prompt_tenant_name" json:"name"`
	
	Versions string `json:"versions"`
	
	ActiveVersion int       `json:"active_version"`
	Description   string    `json:"description,omitempty"`
	CreatedBy     string    `json:"created_by"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

func (LLMPrompt) TableName() string { return "llm_prompts" }

type LLMGuardrailRule struct {
	ID       string `gorm:"primaryKey" json:"id"`
	TenantID string `gorm:"index;uniqueIndex:idx_llm_guardrail_tenant_name" json:"tenant_id"`
	Name     string `gorm:"uniqueIndex:idx_llm_guardrail_tenant_name" json:"name"`
	
	Category string `json:"category"`
	
	Direction string `json:"direction"`
	
	Action string `json:"action"`
	
	Pattern string `json:"pattern,omitempty"`
	
	Keywords string `json:"keywords,omitempty"`
	
	Replacement string    `json:"replacement,omitempty"`
	Enabled     bool      `json:"enabled"`
	CreatedBy   string    `json:"created_by,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (LLMGuardrailRule) TableName() string { return "llm_guardrail_rules" }

type ModelCredential struct {
	ID       string `gorm:"primaryKey" json:"id"`
	TenantID string `gorm:"index;uniqueIndex:idx_model_cred_tenant_backend_name" json:"tenant_id"`
	
	Backend string `gorm:"uniqueIndex:idx_model_cred_tenant_backend_name" json:"backend"`
	
	Name string `gorm:"uniqueIndex:idx_model_cred_tenant_backend_name" json:"name"`
	
	APIKey string `gorm:"-" json:"-"`
	
	APIKeyEnc string    `gorm:"column:api_key_enc;type:text" json:"-"`
	BaseURL   string    `json:"base_url,omitempty"`
	Extra     string    `json:"extra,omitempty"` 
	Enabled   bool      `json:"enabled"`
	CreatedBy string    `json:"created_by,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (ModelCredential) TableName() string { return "model_credentials" }

type LLMKnowledgeBase struct {
	ID       string `gorm:"primaryKey" json:"id"`
	Name     string `json:"name"`
	TenantID string `gorm:"index" json:"tenant_id"`
	
	EmbeddingModel string    `json:"embedding_model"`
	Dimension      int       `json:"dimension"`
	ChunkSize      int       `json:"chunk_size"`
	Overlap        int       `json:"overlap"`
	CreatedBy      string    `json:"created_by"`
	CreatedAt      time.Time `json:"created_at"`
}

func (LLMKnowledgeBase) TableName() string { return "llm_knowledge_bases" }

type LLMDocument struct {
	ID      string `gorm:"primaryKey" json:"id"`
	BaseID  string `gorm:"index" json:"base_id"`
	Title   string `json:"title"`
	Source  string `json:"source,omitempty"`
	Content string `json:"content"`
	
	Segments int `json:"segments"`
	
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

func (LLMDocument) TableName() string { return "llm_documents" }

type LLMAdapter struct {
	ID   string `gorm:"primaryKey" json:"id"`
	Name string `gorm:"index" json:"name"`
	
	BaseModel string `gorm:"index" json:"base_model"`
	Path      string `json:"path"`
	Rank      int    `json:"rank"`
	
	Method string `json:"method"`
	
	SourceJobID string    `gorm:"index" json:"source_job_id,omitempty"`
	TenantID    string    `gorm:"index" json:"tenant_id"`
	CreatedBy   string    `json:"created_by"`
	CreatedAt   time.Time `json:"created_at"`
}

func (LLMAdapter) TableName() string { return "llm_adapters" }

type LLMAdapterMount struct {
	ID        string `gorm:"primaryKey" json:"id"`
	AdapterID string `gorm:"index" json:"adapter_id"`
	ServiceID string `gorm:"index" json:"service_id"`
	
	ServedName string `gorm:"index:idx_mount_tenant_served,unique,composite:tenant_served" json:"served_name"`
	TenantID   string `gorm:"index:idx_mount_tenant_served,unique,composite:tenant_served" json:"tenant_id"`
	
	BaseModel string    `gorm:"index" json:"base_model"`
	CreatedBy string    `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
}

func (LLMAdapterMount) TableName() string { return "llm_adapter_mounts" }