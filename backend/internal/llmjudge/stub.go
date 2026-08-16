package llmjudge

import (
	"context"
	"crypto/sha1"
	"encoding/binary"

	"fuze-ai-paas/backend/internal/ports"
)

type Stub struct {
	modelName string
}

func NewStub(modelName string) *Stub {
	if modelName == "" {
		modelName = "stub"
	}
	return &Stub{modelName: modelName}
}

func (s *Stub) Model() string { return s.modelName }

func (s *Stub) Judge(_ context.Context, req ports.JudgeRequest) (ports.JudgeResponse, error) {
	scores := map[string]float64{}
	for _, d := range req.Dimensions {
		h := sha1.Sum([]byte(req.Task + "|" + d.Name))
		v := float64(binary.BigEndian.Uint32(h[:4])%200) / 1000.0 
		scores[d.Name] = 0.4 + v                                   
	}
	var weightSum, weighted float64
	for _, d := range req.Dimensions {
		weighted += scores[d.Name] * d.Weight
		weightSum += d.Weight
	}
	overall := 0.5
	if weightSum > 0 {
		overall = weighted / weightSum
	}
	return ports.JudgeResponse{
		Scores:  scores,
		Overall: overall,
		Comment: "stub: llm not configured",
	}, nil
}