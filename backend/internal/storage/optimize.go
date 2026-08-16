package storage

import (
	"context"
	"errors"

	"fuze-ai-paas/backend/internal/domain/optimize"
	"fuze-ai-paas/backend/internal/models"

	"gorm.io/gorm"
)

type compressionRepo struct {
	db *gorm.DB
}

func NewCompressionRepository(db *gorm.DB) optimize.CompressionRepository {
	return &compressionRepo{db: db}
}

func (s *Storage) Compression() optimize.CompressionRepository {
	return NewCompressionRepository(s.db)
}

func isOptNotFound(err error) bool { return errors.Is(err, gorm.ErrRecordNotFound) }

func (r *compressionRepo) Create(ctx context.Context, t *optimize.CompressionTask) error {
	return r.db.WithContext(ctx).Create(taskToModel(t)).Error
}

func (r *compressionRepo) Get(ctx context.Context, id string) (*optimize.CompressionTask, error) {
	var m models.CompressionTask
	if err := r.db.WithContext(ctx).First(&m, "id = ?", id).Error; err != nil {
		if isOptNotFound(err) {
			return nil, optimize.ErrNotFound
		}
		return nil, err
	}
	return taskFromModel(&m), nil
}

func (r *compressionRepo) ListByTenant(ctx context.Context, tenantID string) ([]*optimize.CompressionTask, error) {
	var list []models.CompressionTask
	q := r.db.WithContext(ctx)
	if tenantID != "" {
		q = q.Where("tenant_id = ?", tenantID)
	}
	if err := q.Order("created_at DESC").Find(&list).Error; err != nil {
		return nil, err
	}
	out := make([]*optimize.CompressionTask, 0, len(list))
	for i := range list {
		out = append(out, taskFromModel(&list[i]))
	}
	return out, nil
}

func (r *compressionRepo) Update(ctx context.Context, t *optimize.CompressionTask) error {
	m := taskToModel(t)
	m.UpdatedAt = now()
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing models.CompressionTask
		if err := tx.Select("id").First(&existing, "id = ?", m.ID).Error; err != nil {
			if isOptNotFound(err) {
				return optimize.ErrNotFound
			}
			return err
		}
		return tx.Model(&models.CompressionTask{}).
			Where("id = ?", m.ID).
			Select(
				"TenantID", "Name", "Type", "Backend", "ConfigJSON",
				"ModelVersionID", "Status", "JobID", "GateThreshold",
				"OrigAccuracy", "GatePass", "FailReason",
				"CompressedSizeBytes", "LatencyMs", "Accuracy",
				"ArtifactURI", "CompressionRatio", "Speedup",
				"UpdatedAt",
			).
			Updates(m).Error
	})
}

func (r *compressionRepo) Delete(ctx context.Context, id string) error {
	res := r.db.WithContext(ctx).Where("id = ?", id).Delete(&models.CompressionTask{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return optimize.ErrNotFound
	}
	return nil
}

func taskToModel(t *optimize.CompressionTask) *models.CompressionTask {
	return &models.CompressionTask{
		ID:                 t.ID,
		TenantID:           t.TenantID,
		Name:               t.Name,
		Type:               string(t.Type),
		Backend:            string(t.Backend),
		ConfigJSON:         t.ConfigJSON,
		ModelVersionID:     t.ModelVersionID,
		Status:             string(t.Status),
		JobID:              t.JobID,
		GateThreshold:      t.GateThreshold,
		OrigAccuracy:       t.OrigAccuracy,
		GatePass:           t.GatePass,
		FailReason:         t.FailReason,
		CompressedSizeBytes: t.CompressedSizeBytes,
		LatencyMs:          t.LatencyMs,
		Accuracy:           t.Accuracy,
		ArtifactURI:        t.ArtifactURI,
		CompressionRatio:   t.CompressionRatio,
		Speedup:            t.Speedup,
	}
}

func taskFromModel(m *models.CompressionTask) *optimize.CompressionTask {
	return &optimize.CompressionTask{
		ID:                 m.ID,
		TenantID:           m.TenantID,
		Name:               m.Name,
		Type:               optimize.CompressionType(m.Type),
		Backend:            optimize.CompressionBackend(m.Backend),
		ConfigJSON:         m.ConfigJSON,
		ModelVersionID:     m.ModelVersionID,
		Status:             optimize.CompressionStatus(m.Status),
		JobID:              m.JobID,
		GateThreshold:      m.GateThreshold,
		OrigAccuracy:       m.OrigAccuracy,
		GatePass:           m.GatePass,
		FailReason:         m.FailReason,
		CompressedSizeBytes: m.CompressedSizeBytes,
		LatencyMs:          m.LatencyMs,
		Accuracy:           m.Accuracy,
		ArtifactURI:        m.ArtifactURI,
		CompressionRatio:   m.CompressionRatio,
		Speedup:            m.Speedup,
	}
}