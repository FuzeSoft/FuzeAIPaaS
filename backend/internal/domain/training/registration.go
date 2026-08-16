package training

import (
	"errors"
	"strings"
)

type ModelRegistration struct {
	Enabled    bool   `json:"enabled"`
	ModelID    string `json:"model_id"`
	VersionTag string `json:"version_tag"`
}

func (r *ModelRegistration) Normalize() {
	r.ModelID = strings.TrimSpace(r.ModelID)
	r.VersionTag = strings.TrimSpace(r.VersionTag)
}

func (r ModelRegistration) Validate() error {
	if !r.Enabled {
		return nil
	}
	if strings.TrimSpace(r.ModelID) == "" {
		return errors.New("register_model.model_id is required when registration is enabled")
	}
	if strings.TrimSpace(r.VersionTag) == "" {
		return errors.New("register_model.version_tag is required when registration is enabled")
	}
	return nil
}