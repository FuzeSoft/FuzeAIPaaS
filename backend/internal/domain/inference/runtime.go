package inference

import "context"

type RuntimeClient interface {
	
	Deploy(ctx context.Context, svc *InferenceService) (runtimeName string, err error)
	
	Undeploy(ctx context.Context, runtimeName string) error
	
	Status(ctx context.Context, runtimeName string) (ready, found, failed bool, replicas int, url string, err error)
	
	Scale(ctx context.Context, runtimeName string, replicas int) error
	
	RolloutCanary(ctx context.Context, runtimeName string, weight int) error
}