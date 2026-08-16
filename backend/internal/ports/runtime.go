package ports

import (
	"fuze-ai-paas/backend/internal/domain/inference"
)

type RuntimeRegistry interface {
	
	For(clusterID string, kind inference.RuntimeKind, client ClusterClientPort) (inference.RuntimeClient, error)
}