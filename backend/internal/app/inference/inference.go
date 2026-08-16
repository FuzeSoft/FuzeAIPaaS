
package inference

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"fuze-ai-paas/backend/internal/adapter"
	domaininference "fuze-ai-paas/backend/internal/domain/inference"
	"fuze-ai-paas/backend/internal/models"
	"fuze-ai-paas/backend/internal/ports"
)

const clusterOpTimeout = 30 * time.Second

const maxFailureReasonLen = 500

func truncateReason(reason string) string {
	if len(reason) <= maxFailureReasonLen {
		return reason
	}
	return reason[:maxFailureReasonLen] + "..."
}

type Service struct {
	jobRepo    ports.InferenceRepository
	clusterMgr ports.ClusterRegistry
	runtimeReg ports.RuntimeRegistry
}

func NewService(jobRepo ports.InferenceRepository, clusterMgr ports.ClusterRegistry, runtimeReg ports.RuntimeRegistry) *Service {
	return &Service{jobRepo: jobRepo, clusterMgr: clusterMgr, runtimeReg: runtimeReg}
}

func (svc *Service) clusterManaged(clusterID string) bool {
	if svc.clusterMgr == nil {
		return false
	}
	_, err := svc.clusterMgr.Get(clusterID)
	return !errors.Is(err, ports.ErrClusterNotRegistered)
}

func (svc *Service) Deploy(s *models.InferenceService) error {
	if s == nil {
		return fmt.Errorf("inference service is nil")
	}
	ctx, cancel := context.WithTimeout(context.Background(), clusterOpTimeout)
	defer cancel()
	agg := adapter.InferenceFromModel(s)

	if svc.clusterManaged(s.ClusterID) {
		kc, err := svc.clusterMgr.K8sClient(s.ClusterID)
		if err == nil && kc != nil && kc.Enabled() {
			rt, rerr := svc.runtimeReg.For(s.ClusterID, agg.Runtime, kc)
			if rerr != nil {
				
				s.Status = models.InferenceStatusFailed
				s.FailureReason = truncateReason(fmt.Sprintf("runtime %s unavailable: %v", agg.Runtime, rerr))
				_ = svc.jobRepo.UpdateInferenceRuntimeStatus(s)
				return rerr
			}
			name, derr := rt.Deploy(ctx, agg)
			if derr != nil {
				s.Status = models.InferenceStatusFailed
				s.FailureReason = truncateReason(fmt.Sprintf("deploy failed: %v", derr))
				_ = svc.jobRepo.UpdateInferenceRuntimeStatus(s)
				return derr
			}
			agg.RuntimeName = name
			agg.Status = domaininference.InferenceStatusPending
			adapter.InferenceSyncToModel(agg, s)
			
			return svc.jobRepo.UpdateInferenceRuntimeStatus(s)
		}
		
		s.Status = models.InferenceStatusPending
		reason := fmt.Sprintf("cluster %s configured but not usable for deploy", s.ClusterID)
		if err != nil {
			reason = fmt.Sprintf("cluster %s configured but unreachable: %v", s.ClusterID, err)
		} else if kc == nil {
			reason = fmt.Sprintf("cluster %s has no k8s client", s.ClusterID)
		} else if !kc.Enabled() {
			reason = fmt.Sprintf("cluster %s is disabled", s.ClusterID)
		}
		s.FailureReason = reason
		_ = svc.jobRepo.UpdateInferenceRuntimeStatus(s)
		log.Printf("[Inference] %s, service %s kept pending", reason, s.Name)
		return nil
	}

	s.Status = models.InferenceStatusReady
	s.KServeName = s.Name
	
	agg.RuntimeName = s.Name
	s.ReadyReplicas = s.MinReplicas
	if s.URL == "" {
		ns := ports.DefaultNamespace
		if svc.clusterMgr != nil {
			if client, err := svc.clusterMgr.Get(s.ClusterID); err == nil && client != nil && client.Enabled() {
				ns = client.Namespace()
			}
		}
		s.URL = "http://" + s.Name + "." + ns + ".example.com/v1/models/" + s.Name + ":predict"
	}
	return svc.jobRepo.UpdateInferenceRuntimeStatus(s)
}

func (svc *Service) Delete(s *models.InferenceService) error {
	if s == nil {
		return fmt.Errorf("inference service is nil")
	}
	ctx, cancel := context.WithTimeout(context.Background(), clusterOpTimeout)
	defer cancel()
	_ = svc.Undeploy(ctx, s)
	return svc.jobRepo.DeleteInferenceService(s.ID)
}

func (svc *Service) Undeploy(ctx context.Context, s *models.InferenceService) error {
	if s == nil {
		return fmt.Errorf("inference service is nil")
	}
	if s.KServeName == "" || svc.clusterMgr == nil {
		return nil
	}
	if kc, err := svc.clusterMgr.K8sClient(s.ClusterID); err == nil && kc != nil && kc.Enabled() {
		agg := adapter.InferenceFromModel(s)
		if rt, rerr := svc.runtimeReg.For(s.ClusterID, agg.Runtime, kc); rerr == nil {
			if derr := rt.Undeploy(ctx, s.KServeName); derr != nil {
				log.Printf("[Inference] Failed to undeploy %s: %v", s.KServeName, derr)
			}
		}
	} else if client, err := svc.clusterMgr.Get(s.ClusterID); err == nil && client != nil && client.Enabled() {
		if derr := client.DeleteInferenceService(ctx, s.KServeName); derr != nil {
			log.Printf("[Inference] Failed to delete InferenceService %s: %v", s.KServeName, derr)
		}
	}
	return nil
}

func (svc *Service) runtimeFor(s *models.InferenceService) (domaininference.RuntimeClient, bool) {
	agg := adapter.InferenceFromModel(s)
	if svc.clusterMgr == nil {
		return nil, false
	}
	kc, err := svc.clusterMgr.K8sClient(s.ClusterID)
	if err != nil || kc == nil || !kc.Enabled() {
		return nil, false
	}
	rt, rerr := svc.runtimeReg.For(s.ClusterID, agg.Runtime, kc)
	if rerr != nil {
		log.Printf("[Inference] Runtime client unavailable for svc %s: %v", s.Name, rerr)
		return nil, false
	}
	return rt, true
}

func (svc *Service) Reconcile(ctx context.Context) {
	list, err := svc.jobRepo.GetInferenceServices()
	if err != nil {
		log.Printf("[Inference] Reconcile: list services failed: %v", err)
		return
	}
	for i := range list {
		select {
		case <-ctx.Done():
			return
		default:
		}
		svc.reconcileOne(ctx, &list[i])
	}
}

func (svc *Service) reconcileOne(ctx context.Context, s *models.InferenceService) {
	
	agg := adapter.InferenceFromModel(s)
	if agg.NeedsDeploy() {
		if err := svc.Deploy(s); err != nil {
			log.Printf("[Inference] Reconcile: deploy %s failed: %v", s.ID, err)
			return
		}
		agg = adapter.InferenceFromModel(s) 
	}

	clusterManaged := svc.clusterManaged(s.ClusterID)
	rt, runtimeAvailable := svc.runtimeFor(s)
	if agg.ShouldUseRuntime(clusterManaged, runtimeAvailable) {
		
		if agg.NeedsScalePush() {
			if err := rt.Scale(ctx, agg.RuntimeName, agg.TargetReplicas); err != nil {
				log.Printf("[Inference] Reconcile: scale %s failed: %v", s.ID, err)
			}
		}
		
		if agg.NeedsCanaryPush() {
			if err := rt.RolloutCanary(ctx, agg.RuntimeName, agg.CanaryWeight); err != nil {
				
				if !errors.Is(err, domaininference.ErrCanaryUnsupported) {
					log.Printf("[Inference] Reconcile: canary %s failed: %v", s.ID, err)
				}
			}
		}
		
		ready, found, failed, replicas, url, serr := rt.Status(ctx, agg.RuntimeName)
		if serr != nil {
			log.Printf("[Inference] Reconcile: status %s failed: %v", s.ID, serr)
			return
		}
		agg.ApplyRuntimeStatus(ready, found, failed, replicas, url)
		
		if agg.Status == domaininference.InferenceStatusFailed && s.FailureReason == "" {
			s.FailureReason = "runtime reported failure (Ready/Progressing condition False)"
		} else if agg.Status != domaininference.InferenceStatusFailed {
			s.FailureReason = "" 
		}
		adapter.InferenceSyncToModel(agg, s)
		
		if err := svc.jobRepo.UpdateInferenceRuntimeStatus(s); err != nil {
			log.Printf("[Inference] Reconcile: persist %s failed: %v", s.ID, err)
		}
		return
	}

	if clusterManaged {
		return
	}
	
	if agg.MockConvergeObservation() {
		
		adapter.InferenceSyncToModel(agg, s)
		if err := svc.jobRepo.UpdateInferenceRuntimeStatus(s); err != nil {
			log.Printf("[Inference] Reconcile: persist %s failed: %v", s.ID, err)
		}
	}
}