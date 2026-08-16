
package workspace

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"fuze-ai-paas/backend/internal/domain/event"
	"fuze-ai-paas/backend/internal/domain/workspace"
	"fuze-ai-paas/backend/internal/models"
	"fuze-ai-paas/backend/internal/notify"
	"fuze-ai-paas/backend/internal/ports"
)

var (
	ErrForbidden       = errors.New("operation forbidden: not owner or admin")
	ErrIllegalState    = errors.New("illegal workspace state for this operation")
	ErrImageNotAllowed = errors.New("image not in allowed whitelist")
)

type ImagePolicy struct {
	Allowed []string
}

func DefaultImagePolicy() ImagePolicy {
	return ImagePolicy{}
}

func (p ImagePolicy) Allow(image string) bool {
	if len(p.Allowed) == 0 {
		return true
	}
	for _, a := range p.Allowed {
		if a == image {
			return true
		}
	}
	return false
}

type Service struct {
	repo    ports.WorkspaceRepository
	quota   ports.QuotaRepository
	runtime ports.WorkspaceRuntime
	images  ImagePolicy

	statusFn func(ws *models.Workspace) (ready, found, failed bool)
	
	activeFn func(ws *models.Workspace) (bool, error)
	
	notifier notify.EventSink
}

type Option func(*Service)

func WithNotifier(sink notify.EventSink) Option {
	return func(s *Service) { s.notifier = sink }
}

func NewService(
	repo ports.WorkspaceRepository,
	quota ports.QuotaRepository,
	runtime ports.WorkspaceRuntime,
	images ImagePolicy,
	opts ...Option,
) *Service {
	s := &Service{
		repo:     repo,
		quota:    quota,
		runtime:  runtime,
		images:   images,
		statusFn: func(ws *models.Workspace) (bool, bool, bool) { return false, false, false },
		activeFn: func(ws *models.Workspace) (bool, error) { return false, nil },
	}
	for _, o := range opts {
		o(s)
	}
	return s
}

func (s *Service) Create(ctx context.Context, ws *models.Workspace) error {
	if ws == nil {
		return fmt.Errorf("workspace is nil")
	}
	
	if !s.images.Allow(ws.Image) {
		return ErrImageNotAllowed
	}

	memGB := parseMemGB(ws.MemoryRequest)
	if err := s.quota.CheckAndReserve(ws.TenantID, ws.GPUCount, memGB, 1); err != nil {
		return err 
	}
	if err := s.repo.CreateWorkspace(ws); err != nil {
		_ = s.quota.Release(ws.TenantID, ws.GPUCount, memGB, 1)
		return err
	}

	if err := s.provision(ctx, ws); err != nil {
		log.Printf("[Workspace] provision %s failed: %v", ws.ID, err)
	}
	return nil
}

func (s *Service) provision(ctx context.Context, ws *models.Workspace) error {
	if ws.Status != models.WorkspaceStatusPending {
		return nil
	}
	ws.Status = models.WorkspaceStatusStarting
	if err := s.repo.UpdateWorkspace(ws); err != nil {
		return err
	}
	name, err := s.runtime.Provision(ctx, ws)
	if err != nil {
		ws.Status = models.WorkspaceStatusFailed
		ws.FailureMsg = truncateReason(err.Error())
		_ = s.repo.UpdateWorkspace(ws)
		return err
	}
	ws.RuntimeName = name
	return s.repo.UpdateWorkspace(ws)
}

func (s *Service) Start(ctx context.Context, tenantID, actor string, isAdmin bool, id string) error {
	ws, err := s.repo.GetWorkspaceForTenant(tenantID, id)
	if err != nil {
		return err
	}
	if !s.authorized(ws, actor, isAdmin) {
		return ErrForbidden
	}
	if !toDomain(ws).CanStart() {
		return ErrIllegalState
	}
	ws.Status = models.WorkspaceStatusStarting
	if err := s.repo.UpdateWorkspace(ws); err != nil {
		return err
	}
	if _, err := s.runtime.Provision(ctx, ws); err != nil {
		ws.Status = models.WorkspaceStatusFailed
		ws.FailureMsg = truncateReason(err.Error())
		_ = s.repo.UpdateWorkspace(ws)
		return err
	}
	return nil
}

func (s *Service) Stop(ctx context.Context, tenantID, actor string, isAdmin bool, id string) error {
	ws, err := s.repo.GetWorkspaceForTenant(tenantID, id)
	if err != nil {
		return err
	}
	if !s.authorized(ws, actor, isAdmin) {
		return ErrForbidden
	}
	if !toDomain(ws).CanStop() {
		return ErrIllegalState
	}
	if err := s.runtime.Deprovision(ctx, ws); err != nil {
		return err
	}
	now := time.Now()
	ws.Status = models.WorkspaceStatusStopped
	ws.StoppedAt = &now
	return s.repo.UpdateWorkspace(ws)
}

func (s *Service) Delete(ctx context.Context, tenantID, actor string, isAdmin bool, id string) error {
	ws, err := s.repo.GetWorkspaceForTenant(tenantID, id)
	if err != nil {
		return err
	}
	if !s.authorized(ws, actor, isAdmin) {
		return ErrForbidden
	}
	
	if ws.Status == models.WorkspaceStatusRunning {
		return ErrIllegalState
	}
	if err := s.runtime.Deprovision(ctx, ws); err != nil {
		log.Printf("[Workspace] deprovision %s failed: %v", id, err)
	}
	
	memGB := parseMemGB(ws.MemoryRequest)
	return s.repo.DeleteWorkspaceAndReleaseQuota(id, ws.TenantID, ws.GPUCount, memGB)
}

func (s *Service) authorized(ws *models.Workspace, actor string, isAdmin bool) bool {
	return isAdmin || ws.OwnerID == actor
}

func (s *Service) Reconcile(ctx context.Context) {
	list, err := s.repo.ListWorkspaces("", ports.WorkspaceFilter{})
	if err != nil {
		log.Printf("[Workspace] reconcile list failed: %v", err)
		return
	}
	for i := range list {
		select {
		case <-ctx.Done():
			return
		default:
		}
		s.reconcileOne(ctx, &list[i])
	}
}

func (s *Service) reconcileOne(ctx context.Context, ws *models.Workspace) {
	switch ws.Status {
	case models.WorkspaceStatusStarting:
		ready, found, _ := s.statusFn(ws)
		if found && ready {
			now := time.Now()
			ws.Status = models.WorkspaceStatusRunning
			ws.StartedAt = &now
			if err := s.repo.UpdateWorkspace(ws); err != nil {
				log.Printf("[Workspace] reconcile persist running %s failed: %v", ws.ID, err)
			}
		}
	case models.WorkspaceStatusRunning:
		_, found, failed := s.statusFn(ws)
		if !found {
			
			now := time.Now()
			ws.Status = models.WorkspaceStatusFailed
			ws.StoppedAt = &now
			if ws.FailureMsg == "" {
				ws.FailureMsg = "runtime resource disappeared during running"
			}
			if err := s.repo.UpdateWorkspace(ws); err != nil {
				log.Printf("[Workspace] reconcile persist failed %s failed: %v", ws.ID, err)
			}
		} else if failed {
			now := time.Now()
			ws.Status = models.WorkspaceStatusFailed
			ws.StoppedAt = &now
			if err := s.repo.UpdateWorkspace(ws); err != nil {
				log.Printf("[Workspace] reconcile persist failed %s failed: %v", ws.ID, err)
			}
		}
	}
}

func (s *Service) ReclaimIdle(ctx context.Context, now time.Time) ([]string, error) {
	candidates, err := s.repo.ListReclaimable(now)
	if err != nil {
		return nil, fmt.Errorf("list reclaimable: %w", err)
	}
	reclaimed := make([]string, 0, len(candidates))
	for i := range candidates {
		select {
		case <-ctx.Done():
			return reclaimed, ctx.Err()
		default:
		}
		ws := &candidates[i]
		active, aerr := s.activeFn(ws)
		if aerr != nil {
			log.Printf("[Workspace] reclaim active-check %s failed: %v (treat as active, skip)", ws.ID, aerr)
			continue
		}
		if active {
			
			if terr := s.repo.TouchWorkspace(ws.ID, now); terr != nil {
				log.Printf("[Workspace] reclaim touch %s failed: %v", ws.ID, terr)
			}
			continue
		}
		
		if s.notifier != nil {
			if nerr := s.notifier.Notify(ctx, event.NewWorkspaceReclaimed(event.WorkspaceInfo{
				ID:           ws.ID,
				TenantID:     ws.TenantID,
				OwnerID:      ws.OwnerID,
				Name:         ws.Name,
				IdleTimeout:  ws.IdleTimeout,
				LastActiveAt: ws.LastActiveAt,
			})); nerr != nil {
				log.Printf("[Workspace] reclaim notify %s failed: %v", ws.ID, nerr)
			}
		}
		if err := s.runtime.Deprovision(ctx, ws); err != nil {
			log.Printf("[Workspace] reclaim deprovision %s failed: %v", ws.ID, err)
		}
		ws.Status = models.WorkspaceStatusStopping
		if uerr := s.repo.UpdateWorkspace(ws); uerr != nil {
			log.Printf("[Workspace] reclaim persist %s failed: %v", ws.ID, uerr)
			continue
		}
		reclaimed = append(reclaimed, ws.ID)
	}
	return reclaimed, nil
}

func parseMemGB(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	mult := 1
	switch {
	case strings.HasSuffix(s, "Gi"):
		s = strings.TrimSuffix(s, "Gi")
	case strings.HasSuffix(s, "Mi"):
		s = strings.TrimSuffix(s, "Mi")
		mult = 0 
	case strings.HasSuffix(s, "G"):
		s = strings.TrimSuffix(s, "G")
	case strings.HasSuffix(s, "M"):
		s = strings.TrimSuffix(s, "M")
		mult = 0
	}
	var v int
	if _, err := fmt.Sscanf(s, "%d", &v); err != nil {
		return 0
	}
	return v * mult
}

const maxReasonLen = 500

func truncateReason(reason string) string {
	if len(reason) <= maxReasonLen {
		return reason
	}
	return reason[:maxReasonLen] + "..."
}

func toDomain(ws *models.Workspace) *workspace.Workspace {
	return &workspace.Workspace{
		ID:           ws.ID,
		TenantID:     ws.TenantID,
		OwnerID:      ws.OwnerID,
		Name:         ws.Name,
		Kind:         string(ws.Kind),
		Image:        ws.Image,
		Status:       string(ws.Status),
		RuntimeName:  ws.RuntimeName,
		IdleTimeout:  ws.IdleTimeout,
		LastActiveAt: ws.LastActiveAt,
		StartedAt:    ws.StartedAt,
		StoppedAt:    ws.StoppedAt,
		Resources: workspace.ResourceSpec{
			CPU:     parseCPU(ws.CPURequest),
			Memory:  parseMemGB(ws.MemoryRequest),
			GPU:     ws.GPUCount,
			GPUType: ws.GPUModel,
		},
	}
}

func parseCPU(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	var v int
	if _, err := fmt.Sscanf(s, "%d", &v); err != nil {
		return 0
	}
	return v
}