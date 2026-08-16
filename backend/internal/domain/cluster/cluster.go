
package cluster

import (
	"time"

	"fuze-ai-paas/backend/internal/domain/event"
	"fuze-ai-paas/backend/internal/domain/gpu"
)

const (
	StatusRegistered = "registered" 
	StatusHealthy    = "healthy"    
	StatusUnhealthy  = "unhealthy"  
)

type Cluster struct {
	ID         string
	Name       string
	Status     string
	NodeCount  int
	TotalGPUs  int
	UsedGPUs   int
	Version    string
	LastSyncAt time.Time
}

func New(id, name string) *Cluster {
	return &Cluster{ID: id, Name: name, Status: StatusRegistered}
}

func (c *Cluster) Discover(devices []gpu.GPUDevice) event.ClusterDiscovered {
	nodeCount := len(devices)
	total := 0
	used := 0
	for i := range devices {
		total += devices[i].Total
		used += devices[i].Used
	}
	c.NodeCount = nodeCount
	c.TotalGPUs = total
	c.UsedGPUs = used
	c.Status = StatusHealthy
	c.LastSyncAt = time.Now().UTC()
	return event.NewClusterDiscovered(c.ID, c.Name, event.ClusterStats{
		NodeCount: nodeCount,
		TotalGPUs: total,
		UsedGPUs:  used,
		Status:    c.Status,
		Version:   c.Version,
	})
}