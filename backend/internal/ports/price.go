package ports

import (
	"context"

	"fuze-ai-paas/backend/internal/models"
)

type PriceRepository interface {
	
	SaveLLMPrice(ctx context.Context, p models.LLMPrice) error
	
	GetLLMPrice(ctx context.Context, model string) (models.LLMPrice, error)
	
	ListLLMPrices(ctx context.Context) ([]models.LLMPrice, error)
	
	DeleteLLMPrice(ctx context.Context, model string) error

	SaveGPUPrice(ctx context.Context, p models.GPUPrice) error
	
	GetGPUPrice(ctx context.Context, gpuType string) (models.GPUPrice, error)
	
	ListGPUPrices(ctx context.Context) ([]models.GPUPrice, error)
	
	DeleteGPUPrice(ctx context.Context, gpuType string) error
}