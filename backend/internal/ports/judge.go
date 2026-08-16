package ports

import "context"

type JudgeDimension struct {
	Name        string  `json:"name"`
	Weight      float64 `json:"weight"`
	Description string  `json:"description,omitempty"`
}

type JudgeRequest struct {
	Task        string          `json:"task,omitempty"`
	Dataset     string          `json:"dataset,omitempty"`
	Dimensions  []JudgeDimension `json:"dimensions"`
	ModelOutput string          `json:"model_output,omitempty"` 
	Reference   string          `json:"reference,omitempty"`    
}

type JudgeResponse struct {
	Scores  map[string]float64 `json:"scores"`
	Overall float64            `json:"overall"`
	Comment string             `json:"comment,omitempty"`
}

type JudgeLLM interface {
	
	Judge(ctx context.Context, req JudgeRequest) (JudgeResponse, error)
	
	Model() string
}