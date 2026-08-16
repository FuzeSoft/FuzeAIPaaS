package adapter

import (
	"fuze-ai-paas/backend/internal/domain/model"
	"fuze-ai-paas/backend/internal/models"
)

func ModelFromModel(m *models.Model) *model.Model {
	if m == nil {
		return nil
	}
	return &model.Model{
		ID:          m.ID,
		Name:        m.Name,
		Description: m.Description,
		Framework:   model.Framework(m.Framework),
		Owner:       m.Owner,
		CreatedAt:   m.CreatedAt,
		UpdatedAt:   m.UpdatedAt,
	}
}

func ModelSyncToModel(agg *model.Model, m *models.Model) {
	if agg == nil || m == nil {
		return
	}
	m.Name = agg.Name
	m.Description = agg.Description
	m.Framework = string(agg.Framework)
	m.Owner = agg.Owner
	m.UpdatedAt = agg.UpdatedAt
}

func ModelVersionFromModel(m *models.ModelVersion) *model.ModelVersion {
	if m == nil {
		return nil
	}
	return &model.ModelVersion{
		ID:         m.ID,
		ModelID:    m.ModelID,
		Version:    m.Version,
		StorageURI: m.StorageURI,
		Image:      m.Image,
		SizeBytes:  m.SizeBytes,
		Hash:       m.Hash,
		CreatedAt:  m.CreatedAt,
	}
}

func ModelVersionSyncToModel(agg *model.ModelVersion, m *models.ModelVersion) {
	if agg == nil || m == nil {
		return
	}
	m.ModelID = agg.ModelID
	m.Version = agg.Version
	m.StorageURI = agg.StorageURI
	m.Image = agg.Image
	m.SizeBytes = agg.SizeBytes
	m.Hash = agg.Hash
}