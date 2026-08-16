
package llm

import (
	"errors"
	"strings"
)

const (
	RoleSystem    = "system"
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleTool      = "tool"
)

var (
	
	ErrNoMessages = errors.New("llm: messages must not be empty")
	
	ErrInvalidRole = errors.New("llm: invalid message role")
	
	ErrEmptyModel = errors.New("llm: model must not be empty")
	
	ErrInvalidTemperature = errors.New("llm: temperature must be within [0,2]")
	
	ErrInvalidTopP = errors.New("llm: top_p must be within (0,1]")
	
	ErrInvalidMaxTokens = errors.New("llm: max_tokens must not be negative")
)

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
	
	Name string `json:"name,omitempty"`
}

func ValidRole(role string) bool {
	switch role {
	case RoleSystem, RoleUser, RoleAssistant, RoleTool:
		return true
	}
	return false
}

type ChatRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Temperature float64   `json:"temperature,omitempty"`
	TopP        float64   `json:"top_p,omitempty"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
	Stream      bool     `json:"stream,omitempty"`
	Stop        []string `json:"stop,omitempty"`
	
	StreamOptions *StreamOptions `json:"stream_options,omitempty"`
	
	User string `json:"user,omitempty"`
}

type StreamOptions struct {
	
	IncludeUsage bool `json:"include_usage"`
}

func (r ChatRequest) Validate() error {
	if strings.TrimSpace(r.Model) == "" {
		return ErrEmptyModel
	}
	if len(r.Messages) == 0 {
		return ErrNoMessages
	}
	for _, m := range r.Messages {
		if !ValidRole(m.Role) {
			return ErrInvalidRole
		}
	}
	if r.Temperature != 0 && (r.Temperature < 0 || r.Temperature > 2) {
		return ErrInvalidTemperature
	}
	if r.TopP != 0 && (r.TopP <= 0 || r.TopP > 1) {
		return ErrInvalidTopP
	}
	if r.MaxTokens < 0 {
		return ErrInvalidMaxTokens
	}
	return nil
}

func (r ChatRequest) Prompt() string {
	var b strings.Builder
	for i, m := range r.Messages {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(m.Role)
		b.WriteString(": ")
		b.WriteString(m.Content)
	}
	return b.String()
}

type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

func (u Usage) Add(other Usage) Usage {
	return Usage{
		PromptTokens:     u.PromptTokens + other.PromptTokens,
		CompletionTokens: u.CompletionTokens + other.CompletionTokens,
		TotalTokens:      u.TotalTokens + other.TotalTokens,
	}
}

func (u Usage) Normalize() Usage {
	if u.TotalTokens == 0 {
		u.TotalTokens = u.PromptTokens + u.CompletionTokens
	}
	return u
}

type Choice struct {
	Index        int     `json:"index"`
	Message      Message `json:"message"`
	FinishReason string  `json:"finish_reason,omitempty"`
}

type ChatResponse struct {
	ID      string   `json:"id"`
	Model   string   `json:"model"`
	Choices []Choice `json:"choices"`
	Usage   Usage    `json:"usage"`
	
	Created int64 `json:"created"`
}

func (r ChatResponse) Text() string {
	if len(r.Choices) == 0 {
		return ""
	}
	return r.Choices[0].Message.Content
}

type Chunk struct {
	ID    string `json:"id"`
	Model string `json:"model"`
	
	Delta string `json:"delta"`
	
	FinishReason string `json:"finish_reason,omitempty"`
	
	Usage *Usage `json:"usage,omitempty"`
}

type EmbeddingRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
	User  string   `json:"user,omitempty"`
}

func (r EmbeddingRequest) Validate() error {
	if strings.TrimSpace(r.Model) == "" {
		return ErrEmptyModel
	}
	if len(r.Input) == 0 {
		return ErrNoMessages
	}
	return nil
}

type Embedding struct {
	Index  int       `json:"index"`
	Vector []float32 `json:"embedding"`
}

type EmbeddingResponse struct {
	Model string      `json:"model"`
	Data  []Embedding `json:"data"`
	Usage Usage       `json:"usage"`
}

func EstimateTokens(s string) int {
	if s == "" {
		return 0
	}
	cjk, other := 0, 0
	for _, r := range s {
		if isCJK(r) {
			cjk++
			continue
		}
		other += len(string(r))
	}
	est := cjk + other/4
	if est == 0 {
		est = 1
	}
	return est
}

func isCJK(r rune) bool {
	switch {
	case r >= 0x4E00 && r <= 0x9FFF: 
		return true
	case r >= 0x3400 && r <= 0x4DBF: 
		return true
	case r >= 0x3040 && r <= 0x30FF: 
		return true
	case r >= 0xAC00 && r <= 0xD7AF: 
		return true
	}
	return false
}