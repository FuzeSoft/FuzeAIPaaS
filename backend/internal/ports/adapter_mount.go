package ports

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

var ErrAdapterMounted = errors.New("adapter is currently mounted")

var ErrAdapterNotMounted = errors.New("adapter not mounted on service")

var ErrMountConflict = errors.New("served name already in use")

var ErrIncompatibleBase = errors.New("adapter base model mismatch")

var ErrSourceJobNotFound = errors.New("source job not found in tenant")

const servedNameSep = ":"

type AdapterMount struct {
	ID        string `json:"id"`
	AdapterID string `json:"adapter_id"`
	ServiceID string `json:"service_id"`
	
	ServedName string `json:"served_name"`
	
	BaseModel string `json:"base_model"`
	
	AdapterName string `json:"-"`
	TenantID    string `json:"tenant_id"`
	CreatedBy   string `json:"created_by"`
	
	CreatedAt int64 `json:"created_at"`
}

func (m *AdapterMount) Normalize() {
	m.AdapterID = strings.TrimSpace(m.AdapterID)
	m.ServiceID = strings.TrimSpace(m.ServiceID)
	m.ServedName = strings.TrimSpace(m.ServedName)
	m.BaseModel = strings.TrimSpace(m.BaseModel)
	m.AdapterName = strings.TrimSpace(m.AdapterName)
	m.TenantID = strings.TrimSpace(m.TenantID)

	if m.ServedName == "" && m.BaseModel != "" && m.AdapterName != "" {
		m.ServedName = JoinServedName(m.BaseModel, m.AdapterName)
	}
}

func (m AdapterMount) Validate() error {
	if m.AdapterID == "" {
		return fmt.Errorf("%w: adapter_id required", ErrAdapterInvalid)
	}
	if m.ServiceID == "" {
		return fmt.Errorf("%w: service_id required", ErrAdapterInvalid)
	}
	if m.ServedName == "" {
		return fmt.Errorf("%w: served_name required", ErrAdapterInvalid)
	}
	if m.BaseModel == "" {
		return fmt.Errorf("%w: base_model required", ErrAdapterInvalid)
	}
	if m.TenantID == "" {
		return fmt.Errorf("%w: tenant_id required", ErrAdapterInvalid)
	}
	
	base, _ := SplitServedName(m.ServedName)
	if base != m.BaseModel {
		return fmt.Errorf("%w: served_name prefix %q does not match base_model %q",
			ErrAdapterInvalid, base, m.BaseModel)
	}
	return nil
}

func JoinServedName(base, adapter string) string {
	return base + servedNameSep + adapter
}

func SplitServedName(served string) (base, adapter string) {
	served = strings.TrimSpace(served)
	idx := strings.Index(served, servedNameSep)
	if idx < 0 {
		return served, ""
	}
	return served[:idx], served[idx+len(servedNameSep):]
}

type JobExistenceChecker interface {
	JobExistsForTenant(ctx context.Context, tenantID, jobID string) (bool, error)
}

func ValidateSourceJob(ctx context.Context, chk JobExistenceChecker, a FineTuneAdapter) error {
	jobID := strings.TrimSpace(a.SourceJobID)
	if jobID == "" {
		return nil
	}
	
	if chk == nil {
		return nil
	}

	exists, err := chk.JobExistsForTenant(ctx, a.TenantID, jobID)
	if err != nil {
		
		return fmt.Errorf("check source job %q: %w", jobID, err)
	}
	if !exists {
		return fmt.Errorf("%w: %q", ErrSourceJobNotFound, jobID)
	}
	return nil
}

type AdapterMountRepository interface {
	
	Mount(ctx context.Context, m *AdapterMount) error
	
	Unmount(ctx context.Context, tenantID, adapterID, serviceID string) error
	
	ListByAdapter(ctx context.Context, tenantID, adapterID string) ([]*AdapterMount, error)
	
	ListByService(ctx context.Context, tenantID, serviceID string) ([]*AdapterMount, error)
	
	ResolveServedName(ctx context.Context, tenantID, servedName string) (*AdapterMount, error)
}