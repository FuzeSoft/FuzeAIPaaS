package model

import "time"

type ModelVersion struct {
	ID         string
	ModelID    string
	Version    string 
	StorageURI string 
	Image      string 
	SizeBytes  int64
	Hash       string
	CreatedAt  time.Time
}

type Reference struct {
	ModelID    string
	ModelName  string
	Version    string
	StorageURI string
	Image      string
}

func (v ModelVersion) Reference(modelName string) Reference {
	return Reference{
		ModelID:    v.ModelID,
		ModelName:  modelName,
		Version:    v.Version,
		StorageURI: v.StorageURI,
		Image:      v.Image,
	}
}