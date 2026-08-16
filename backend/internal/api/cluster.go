package api

import (
	"context"
	"net/http"

	"fuze-ai-paas/backend/internal/k8s"
	"fuze-ai-paas/backend/internal/models"

	"github.com/gin-gonic/gin"
)

func (h *Handler) GetClusters(c *gin.Context) {
	clusters, err := h.clusterRepo.GetClusters()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, clusters)
}

func (h *Handler) GetQueues(c *gin.Context) {
	queues, err := h.clusterRepo.GetQueues()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, queues)
}

func (h *Handler) RegisterCluster(c *gin.Context) {
	var cluster models.Cluster
	if err := c.ShouldBindJSON(&cluster); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if cluster.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cluster name is required"})
		return
	}
	if cluster.Namespace == "" {
		cluster.Namespace = k8s.DefaultNamespace
	}
	cluster.Enabled = true
	cluster.Status = models.ClusterStatusRegistered

	if cluster.KubeConfig != "" {
		if err := guardKubeconfigServers(cluster.KubeConfig); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "cluster endpoint rejected: " + err.Error()})
			return
		}
	}

	if err := h.clusterRepo.CreateCluster(&cluster); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to persist cluster"})
		return
	}

	if err := h.clusterMgr.Register(&cluster); err != nil {
		cluster.Status = models.ClusterStatusUnhealthy
		cluster.SyncError = "kubeconfig connection failed"
		_ = h.clusterRepo.UpdateClusterStats(cluster.ID, cluster)
		c.JSON(http.StatusOK, gin.H{
			"cluster": cluster,
			"warning": "registered but kubeconfig connection failed",
		})
		return
	}

	if err := h.scheduler.RefreshCluster(context.Background(), cluster.ID); err != nil {
		
		cluster.Status = models.ClusterStatusUnhealthy
		cluster.SyncError = err.Error()
		_ = h.clusterRepo.UpdateClusterStats(cluster.ID, cluster)
	}

	c.JSON(http.StatusCreated, cluster)
}

func (h *Handler) GetCluster(c *gin.Context) {
	id := c.Param("id")
	cluster, err := h.clusterRepo.GetCluster(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Cluster not found"})
		return
	}
	c.JSON(http.StatusOK, cluster)
}

func (h *Handler) UpdateCluster(c *gin.Context) {
	id := c.Param("id")
	var cluster models.Cluster
	if err := c.ShouldBindJSON(&cluster); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	cluster.ID = id
	
	if cluster.KubeConfig != "" {
		if gerr := guardKubeconfigServers(cluster.KubeConfig); gerr != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "cluster endpoint rejected: " + gerr.Error()})
			return
		}
	}
	if err := h.clusterRepo.UpdateCluster(&cluster); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, cluster)
}

func (h *Handler) DiscoverCluster(c *gin.Context) {
	id := c.Param("id")
	if err := h.scheduler.RefreshCluster(context.Background(), id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	cluster, err := h.clusterRepo.GetCluster(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, cluster)
}

func (h *Handler) TestCluster(c *gin.Context) {
	id := c.Param("id")
	cluster, err := h.clusterRepo.GetCluster(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Cluster not found"})
		return
	}

	client, err := h.clusterMgr.Get(id)
	if err != nil {
		
		if cluster.KubeConfig != "" {
			if gerr := guardKubeconfigServers(cluster.KubeConfig); gerr != nil {
				c.JSON(http.StatusBadRequest, gin.H{"connected": false, "error": "cluster endpoint rejected: " + gerr.Error()})
				return
			}
		}
		if rerr := h.clusterMgr.Register(cluster); rerr != nil {
			c.JSON(http.StatusBadRequest, gin.H{"connected": false, "error": "cluster registration failed"})
			return
		}
		client, _ = h.clusterMgr.Get(id)
	}
	if client == nil || !client.Enabled() {
		c.JSON(http.StatusOK, gin.H{"connected": false, "error": "k8s client unavailable (mock mode)"})
		return
	}

	version, verr := client.ServerVersion(context.Background())
	if verr != nil {
		c.JSON(http.StatusOK, gin.H{"connected": false, "error": "cluster unreachable or authentication failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"connected": true, "version": version, "namespace": client.Namespace()})
}

func (h *Handler) DeleteCluster(c *gin.Context) {
	id := c.Param("id")
	h.clusterMgr.Unregister(id)
	if err := h.clusterRepo.DeleteCluster(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusNoContent, nil)
}

func (h *Handler) GetClusterResources(c *gin.Context) {
	id := c.Param("id")
	resources, err := h.clusterRepo.GetResourcesByCluster(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, resources)
}