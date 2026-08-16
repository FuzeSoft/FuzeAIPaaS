
package llmgateway

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync/atomic"
	"time"

	"fuze-ai-paas/backend/internal/domain/llm"
	"fuze-ai-paas/backend/internal/ports"
)

type Clock func() time.Time

type IDGen func() string

type RouteTable interface {
	
	PickForTenant(tenantID, model string) ([]llm.Backend, error)
	
	SetHealth(model, backend string, healthy bool) bool
}

type Deps struct {
	
	Completer ports.ChatCompleter
	
	Routes RouteTable
	
	Guard *llm.Guard
	
	Guards GuardProvider
	
	Prices *llm.PriceBook
	
	Quota ports.TokenQuotaRepository
	
	Usage ports.TokenUsageRepository
	
	Traces ports.TraceRepository
	
	Retriever Retriever
	
	Mounts MountResolver
	
	Now Clock
	
	NewID IDGen
	
	Alert func(ctx context.Context, tenantID string, limitCost, usedCost float64) error
}

func (s *Service) GetRetriever() Retriever {
	return s.deps.Retriever
}

type Retriever interface {
	Retrieve(ctx context.Context, baseID, query string, topK int) ([]llm.ScoredSegment, error)
}

type MountResolver interface {
	ResolveServedName(ctx context.Context, tenantID, servedName string) (*ports.AdapterMount, error)
}

func (s *Service) resolveRouteModel(ctx context.Context, tenantID, model string) (string, error) {
	base, adapter := ports.SplitServedName(model)
	if adapter == "" {
		return model, nil
	}
	
	if s.deps.Mounts == nil {
		return model, nil
	}

	mount, err := s.deps.Mounts.ResolveServedName(ctx, tenantID, model)
	if err != nil {
		
		return "", err
	}
	
	if mount.BaseModel != "" {
		return mount.BaseModel, nil
	}
	return base, nil
}

type Service struct {
	deps Deps
	now  Clock
	id   IDGen
}

func NewService(deps Deps) (*Service, error) {
	if deps.Completer == nil {
		return nil, errors.New("llmgateway: completer is required")
	}
	if deps.Routes == nil {
		return nil, errors.New("llmgateway: route table is required")
	}
	now := deps.Now
	if now == nil {
		now = time.Now
	}
	gen := deps.NewID
	if gen == nil {
		gen = defaultIDGen()
	}
	
	if deps.Guards == nil && deps.Guard != nil {
		deps.Guards = staticGuardProvider{guard: deps.Guard}
	}
	return &Service{deps: deps, now: now, id: gen}, nil
}

func (s *Service) guardFor(ctx context.Context, tenantID string) *llm.Guard {
	if s.deps.Guards == nil {
		return nil
	}
	g, err := s.deps.Guards.GuardFor(ctx, tenantID)
	if err != nil {
		log.Printf("llmgateway: 解析租户 %s 的护栏规则失败，本次跳过检查: %v", tenantID, err)
		return nil
	}
	return g
}

type Request struct {
	Chat llm.ChatRequest
	
	TenantID string
	UserID   string
	
	KnowledgeBaseID string
	
	TopK int
}

type Result struct {
	Response llm.ChatResponse
	Trace    *llm.Trace
	
	Citations []llm.ScoredSegment `json:"citations,omitempty"`
}

var (
	
	ErrBlockedByGuard = errors.New("llmgateway: content blocked by guardrail")
	
	ErrAllBackendsFailed = errors.New("llmgateway: all backends failed")
)

const budgetAlertRatio = 0.8

const defaultTopK = 4

func (s *Service) Complete(ctx context.Context, req Request) (*Result, error) {
	start := s.now()
	trace := llm.NewTrace(s.id(), req.Chat.Model, start)
	trace.TenantID = req.TenantID
	trace.UserID = req.UserID

	if err := req.Chat.Validate(); err != nil {
		s.finish(ctx, trace, llm.Usage{}, 0, err)
		return nil, err
	}

	prepared, citations, err := s.prepare(ctx, trace, &req)
	if err != nil {
		s.finish(ctx, trace, llm.Usage{}, 0, err)
		return nil, err
	}

	if err := s.precheckQuota(ctx, req.TenantID, prepared); err != nil {
		s.finish(ctx, trace, llm.Usage{}, 0, err)
		return nil, err
	}

	trace.StartSpan(llm.SpanInference, s.now())
	resp, backend, err := s.completeWithFailover(ctx, req.TenantID, prepared)
	_ = trace.EndSpan(llm.SpanInference, s.now(), err)
	if err != nil {
		s.finish(ctx, trace, llm.Usage{}, 0, err)
		return nil, err
	}
	trace.Backend = backend

	if guard := s.guardFor(ctx, req.TenantID); guard != nil {
		trace.StartSpan(llm.SpanGuardOutput, s.now())
		text := resp.Text()
		res := guard.Check(text, llm.DirectionOutput)
		trace.AddFindings(res.Findings...)
		_ = trace.EndSpan(llm.SpanGuardOutput, s.now(), nil)

		if res.Blocked() {
			
			s.settle(ctx, trace, req, resp.Usage, ErrBlockedByGuard)
			return nil, ErrBlockedByGuard
		}
		if res.Modified() && len(resp.Choices) > 0 {
			resp.Choices[0].Message.Content = res.Content
		}
	}

	s.settle(ctx, trace, req, resp.Usage, nil)
	return &Result{Response: resp, Trace: trace, Citations: citations}, nil
}

func (s *Service) Stream(ctx context.Context, req Request, onChunk func(llm.Chunk) error) (*Result, error) {
	start := s.now()
	trace := llm.NewTrace(s.id(), req.Chat.Model, start)
	trace.TenantID = req.TenantID
	trace.UserID = req.UserID

	if err := req.Chat.Validate(); err != nil {
		s.finish(ctx, trace, llm.Usage{}, 0, err)
		return nil, err
	}

	prepared, citations, err := s.prepare(ctx, trace, &req)
	if err != nil {
		s.finish(ctx, trace, llm.Usage{}, 0, err)
		return nil, err
	}
	if err := s.precheckQuota(ctx, req.TenantID, prepared); err != nil {
		s.finish(ctx, trace, llm.Usage{}, 0, err)
		return nil, err
	}

	routeModel, err := s.resolveRouteModel(ctx, req.TenantID, prepared.Model)
	if err != nil {
		s.finish(ctx, trace, llm.Usage{}, 0, err)
		return nil, err
	}

	backends, err := s.deps.Routes.PickForTenant(req.TenantID, routeModel)
	if err != nil {
		s.finish(ctx, trace, llm.Usage{}, 0, err)
		return nil, err
	}

	trace.StartSpan(llm.SpanInference, s.now())
	
	var (
		usage    llm.Usage
		streamed bool
		lastErr  error
	)
	for _, b := range backends {
		emitted := false
		usage, lastErr = s.deps.Completer.Stream(ctx, b.Endpoint, prepared, func(ch llm.Chunk) error {
			if !emitted {
				emitted = true
				trace.MarkFirstToken(s.now())
			}
			return onChunk(ch)
		})
		if lastErr == nil {
			trace.Backend = b.Name
			streamed = true
			break
		}
		if emitted {
			
			trace.Backend = b.Name
			break
		}
		
		s.markUnhealthy(routeModel, b.Name, lastErr)
	}
	_ = trace.EndSpan(llm.SpanInference, s.now(), lastErr)

	if !streamed {
		err := fmt.Errorf("%w: %v", ErrAllBackendsFailed, lastErr)
		s.finish(ctx, trace, usage, 0, err)
		return nil, err
	}

	if prepared.StreamOptions == nil || prepared.StreamOptions.IncludeUsage {
		u := usage
		if err := onChunk(llm.Chunk{Model: prepared.Model, Usage: &u}); err != nil {
			s.finish(ctx, trace, usage, 0, err)
			return nil, err
		}
	}

	s.settle(ctx, trace, req, usage, nil)
	return &Result{Trace: trace, Citations: citations}, nil
}

func (s *Service) prepare(ctx context.Context, trace *llm.Trace, req *Request) (llm.ChatRequest, []llm.ScoredSegment, error) {
	chat := req.Chat

	if guard := s.guardFor(ctx, req.TenantID); guard != nil {
		trace.StartSpan(llm.SpanGuardInput, s.now())
		var findings []llm.Finding
		msgs := make([]llm.Message, len(chat.Messages))
		copy(msgs, chat.Messages)

		for i, m := range msgs {
			
			if m.Role != llm.RoleUser {
				continue
			}
			res := guard.Check(m.Content, llm.DirectionInput)
			findings = append(findings, res.Findings...)
			if res.Blocked() {
				trace.AddFindings(findings...)
				_ = trace.EndSpan(llm.SpanGuardInput, s.now(), ErrBlockedByGuard)
				return chat, nil, ErrBlockedByGuard
			}
			msgs[i].Content = res.Content
		}
		chat.Messages = msgs
		trace.AddFindings(findings...)
		_ = trace.EndSpan(llm.SpanGuardInput, s.now(), nil)
	}

	var citations []llm.ScoredSegment
	if req.KnowledgeBaseID != "" && s.deps.Retriever != nil {
		trace.StartSpan(llm.SpanRetrieval, s.now())
		topK := req.TopK
		if topK <= 0 {
			topK = defaultTopK
		}
		hits, err := s.deps.Retriever.Retrieve(ctx, req.KnowledgeBaseID, lastUserMessage(chat), topK)
		_ = trace.EndSpan(llm.SpanRetrieval, s.now(), err)
		if err != nil {
			
			log.Printf("[llmgateway] retrieval failed, falling back to plain completion: %v", err)
		} else if len(hits) > 0 {
			citations = hits
			chat.Messages = injectContext(chat.Messages, llm.BuildContext(hits))
		}
	}

	return chat, citations, nil
}

func (s *Service) completeWithFailover(ctx context.Context, tenantID string, req llm.ChatRequest) (llm.ChatResponse, string, error) {
	
	routeModel, err := s.resolveRouteModel(ctx, tenantID, req.Model)
	if err != nil {
		return llm.ChatResponse{}, "", err
	}

	backends, err := s.deps.Routes.PickForTenant(tenantID, routeModel)
	if err != nil {
		return llm.ChatResponse{}, "", err
	}
	var lastErr error
	for _, b := range backends {
		resp, err := s.deps.Completer.Complete(ctx, b.Endpoint, req)
		if err == nil {
			return resp, b.Name, nil
		}
		lastErr = err
		
		s.markUnhealthy(routeModel, b.Name, err)
		
		if ctx.Err() != nil {
			break
		}
	}
	return llm.ChatResponse{}, "", fmt.Errorf("%w: %v", ErrAllBackendsFailed, lastErr)
}

func (s *Service) markUnhealthy(model, backend string, cause error) {
	s.deps.Routes.SetHealth(model, backend, false)
	log.Printf("[llmgateway] backend %s/%s marked unhealthy: %v", model, backend, cause)
}

func (s *Service) precheckQuota(ctx context.Context, tenantID string, req llm.ChatRequest) error {
	if s.deps.Quota == nil || tenantID == "" {
		return nil
	}
	q, err := s.deps.Quota.GetQuota(ctx, tenantID)
	if err != nil {
		
		log.Printf("[llmgateway] quota lookup failed for %s: %v", tenantID, err)
		return nil
	}
	est := int64(llm.EstimateTokens(req.Prompt()))
	if !q.CanConsume(est) {
		return llm.ErrTokenQuotaExceeded
	}
	return nil
}

func (s *Service) settle(ctx context.Context, trace *llm.Trace, req Request, usage llm.Usage, callErr error) {
	usage = usage.Normalize()
	cost := 0.0
	if s.deps.Prices != nil {
		cost = s.deps.Prices.Cost(req.Chat.Model, usage)
	}

	if s.deps.Quota != nil && req.TenantID != "" && usage.TotalTokens > 0 {
		if err := s.deps.Quota.CheckAndConsume(ctx, req.TenantID, int64(usage.TotalTokens), cost); err != nil {
			
			log.Printf("[llmgateway] quota consume failed for %s: %v", req.TenantID, err)
		}
		
		if s.deps.Alert != nil {
			q, gerr := s.deps.Quota.GetQuota(ctx, req.TenantID)
			if gerr == nil && q.LimitCost > 0 {
				ratio := q.UsedCost / q.LimitCost
				if ratio >= budgetAlertRatio || q.UsedCost >= q.LimitCost {
					if aerr := s.deps.Alert(ctx, req.TenantID, q.LimitCost, q.UsedCost); aerr != nil {
						log.Printf("[llmgateway] budget alert failed for %s: %v", req.TenantID, aerr)
					}
				}
			}
		}
	}

	s.finish(ctx, trace, usage, cost, callErr)

	if s.deps.Usage != nil {
		rec := &ports.TokenUsageRecord{
			ID: trace.ID, TenantID: req.TenantID, UserID: req.UserID,
			Model: req.Chat.Model, Backend: trace.Backend,
			PromptTokens: usage.PromptTokens, CompletionTokens: usage.CompletionTokens,
			TotalTokens: usage.TotalTokens, Cost: cost,
			LatencyMS: trace.Latency.Total.Milliseconds(),
			TTFTMS:    trace.Latency.TTFT.Milliseconds(),
			Success:   callErr == nil, TraceID: trace.ID,
			CreatedAt: trace.StartedAt.Unix(),
		}
		if err := s.deps.Usage.RecordUsage(ctx, rec); err != nil {
			log.Printf("[llmgateway] record usage failed: %v", err)
		}
	}
}

func (s *Service) finish(ctx context.Context, trace *llm.Trace, usage llm.Usage, cost float64, err error) {
	trace.Finish(s.now(), usage, cost, err)
	if s.deps.Traces == nil {
		return
	}
	if serr := s.deps.Traces.Save(ctx, trace); serr != nil {
		log.Printf("[llmgateway] save trace failed: %v", serr)
	}
}

func lastUserMessage(req llm.ChatRequest) string {
	for i := len(req.Messages) - 1; i >= 0; i-- {
		if req.Messages[i].Role == llm.RoleUser {
			return req.Messages[i].Content
		}
	}
	return ""
}

func injectContext(msgs []llm.Message, context string) []llm.Message {
	if context == "" {
		return msgs
	}
	prefix := llm.Message{
		Role: llm.RoleSystem,
		Content: "请基于以下参考资料回答问题。若资料中没有相关信息，" +
			"请明确说明「资料中未提及」，不要编造。\n\n参考资料：\n" + context,
	}
	out := make([]llm.Message, 0, len(msgs)+1)
	out = append(out, prefix)
	out = append(out, msgs...)
	return out
}

func defaultIDGen() IDGen {
	var seq int64
	return func() string {
		n := atomic.AddInt64(&seq, 1)
		return fmt.Sprintf("trace-%d-%d", time.Now().UnixNano(), n)
	}
}