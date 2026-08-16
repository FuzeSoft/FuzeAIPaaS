package k8s

import (
	"fmt"
	"sync"

	"fuze-ai-paas/backend/internal/models"
	"fuze-ai-paas/backend/internal/ports"
)

type (
	
	ClusterClientPort = ports.ClusterClientPort
	
	ClusterRegistry = ports.ClusterRegistry
	
	LogQuery = ports.LogQuery
	
	PodRef = ports.PodRef
	
	JobLogs = ports.JobLogs
)

var (
	ErrPodNotFound          = ports.ErrPodNotFound
	ErrClusterNotRegistered = ports.ErrClusterNotRegistered
)

type ClusterManager struct {
	mu      sync.RWMutex
	clients map[string]*Client
}

func NewClusterManager() *ClusterManager {
	return &ClusterManager{
		clients: make(map[string]*Client),
	}
}

func (m *ClusterManager) Register(cluster *models.Cluster) error {
	var c *Client
	if cluster.KubeConfig != "" {
		client, err := NewClientFromKubeConfig(cluster.KubeConfig, cluster.Namespace)
		if err != nil {
			return err
		}
		c = client
	} else {
		c = NewClient(cluster.Namespace)
	}

	m.mu.Lock()
	m.clients[cluster.ID] = c
	m.mu.Unlock()
	return nil
}

func (m *ClusterManager) Unregister(clusterID string) {
	m.mu.Lock()
	delete(m.clients, clusterID)
	m.mu.Unlock()
}

func (m *ClusterManager) Get(clusterID string) (ClusterClientPort, error) {
	m.mu.RLock()
	c, ok := m.clients[clusterID]
	m.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrClusterNotRegistered, clusterID)
	}
	return c, nil
}

func (m *ClusterManager) K8sClient(clusterID string) (ports.ClusterClientPort, error) {
	m.mu.RLock()
	c, ok := m.clients[clusterID]
	m.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrClusterNotRegistered, clusterID)
	}
	return c, nil
}

func (m *ClusterManager) List() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ids := make([]string, 0, len(m.clients))
	for id := range m.clients {
		ids = append(ids, id)
	}
	return ids
}

func (m *ClusterManager) LoadAll(clusters []models.Cluster) []error {
	var errs []error
	for i := range clusters {
		cl := &clusters[i]
		if !cl.Enabled {
			continue
		}
		if err := m.Register(cl); err != nil {
			errs = append(errs, fmt.Errorf("cluster %s register failed: %w", cl.ID, err))
		}
	}
	return errs
}

var _ ClusterRegistry = (*ClusterManager)(nil)
var _ ClusterClientPort = (*Client)(nil)