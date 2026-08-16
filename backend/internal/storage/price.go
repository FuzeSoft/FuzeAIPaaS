package storage

import (
	"context"

	"fuze-ai-paas/backend/internal/domain/llm"
	"fuze-ai-paas/backend/internal/models"
	"fuze-ai-paas/backend/internal/ports"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type priceRepo struct {
	db *gorm.DB
}

func NewPriceRepository(db *gorm.DB) ports.PriceRepository {
	return &priceRepo{db: db}
}

func (r *priceRepo) SaveLLMPrice(ctx context.Context, p models.LLMPrice) error {
	if p.ID == "" {
		p.ID = genID()
	}
	p.UpdatedAt = now()
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "model"}},
		DoUpdates: clause.AssignmentColumns([]string{"input_per1_k", "output_per1_k", "currency", "updated_at"}),
	}).Create(&p).Error
}

func (r *priceRepo) GetLLMPrice(ctx context.Context, model string) (models.LLMPrice, error) {
	var row models.LLMPrice
	err := r.db.WithContext(ctx).First(&row, "model = ?", model).Error
	if isNotFoundErr(err) {
		return models.LLMPrice{}, ports.ErrNotFound
	}
	if err != nil {
		return models.LLMPrice{}, err
	}
	return row, nil
}

func (r *priceRepo) ListLLMPrices(ctx context.Context) ([]models.LLMPrice, error) {
	var rows []models.LLMPrice
	if err := r.db.WithContext(ctx).Order("model ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *priceRepo) DeleteLLMPrice(ctx context.Context, model string) error {
	res := r.db.WithContext(ctx).Where("model = ?", model).Delete(&models.LLMPrice{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ports.ErrNotFound
	}
	return nil
}

func (r *priceRepo) SaveGPUPrice(ctx context.Context, p models.GPUPrice) error {
	if p.ID == "" {
		p.ID = genID()
	}
	p.UpdatedAt = now()
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "gpu_type"}},
		DoUpdates: clause.AssignmentColumns([]string{"price_per_gpu_hour", "currency", "updated_at"}),
	}).Create(&p).Error
}

func (r *priceRepo) GetGPUPrice(ctx context.Context, gpuType string) (models.GPUPrice, error) {
	var row models.GPUPrice
	err := r.db.WithContext(ctx).First(&row, "gpu_type = ?", gpuType).Error
	if isNotFoundErr(err) {
		
		if gpuType != "" {
			return r.GetGPUPrice(ctx, "")
		}
		return models.GPUPrice{}, ports.ErrNotFound
	}
	if err != nil {
		return models.GPUPrice{}, err
	}
	return row, nil
}

func (r *priceRepo) ListGPUPrices(ctx context.Context) ([]models.GPUPrice, error) {
	var rows []models.GPUPrice
	if err := r.db.WithContext(ctx).Order("gpu_type ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *priceRepo) DeleteGPUPrice(ctx context.Context, gpuType string) error {
	res := r.db.WithContext(ctx).Where("gpu_type = ?", gpuType).Delete(&models.GPUPrice{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ports.ErrNotFound
	}
	return nil
}

func BuildPriceBook(ctx context.Context, repo ports.PriceRepository, fallback *llm.PriceBook) (*llm.PriceBook, error) {
	book := llm.NewPriceBook()
	if fallback != nil {
		
		copyFallback(book, fallback)
	}

	llmRows, err := repo.ListLLMPrices(ctx)
	if err != nil {
		return nil, err
	}
	for _, row := range llmRows {
		_ = book.Set(llm.Price{
			Model:       row.Model,
			InputPer1K:  row.InputPer1K,
			OutputPer1K: row.OutputPer1K,
			Currency:    row.Currency,
		})
	}

	gpuRows, err := repo.ListGPUPrices(ctx)
	if err != nil {
		return nil, err
	}
	for _, row := range gpuRows {
		_ = book.SetGPUPrice(row.GPUType, row.PricePerGPUHour, row.Currency)
	}

	return book, nil
}

func copyFallback(dst, src *llm.PriceBook) {
	fb, _ := src.Lookup("")
	_ = dst.SetFallback(fb)
}