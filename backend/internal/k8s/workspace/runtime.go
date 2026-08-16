package workspace

import (
	"context"
	"fmt"
	"strings"
	"time"

	"fuze-ai-paas/backend/internal/k8s"
	"fuze-ai-paas/backend/internal/models"
	"fuze-ai-paas/backend/internal/ports"
	"fuze-ai-paas/backend/internal/storage"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func WorkspaceGVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{
		Group:    "apps",
		Version:  "v1",
		Resource: "deployments",
	}
}

type Driver struct {
	client *k8s.Client
	store  *storage.Storage
	
	proxyBaseURL string
}

func NewDriver(client *k8s.Client, store *storage.Storage, proxyBaseURL string) *Driver {
	return &Driver{client: client, store: store, proxyBaseURL: proxyBaseURL}
}

func (d *Driver) Provision(ctx context.Context, ws *models.Workspace) (string, error) {
	obj := BuildWorkspaceManifest(ws)
	if err := Snapshot(obj); err != nil {
		return "", fmt.Errorf("workspace provision rejected by baseline snapshot: %w", err)
	}
	name := obj.GetName()

	if d.client == nil || !d.client.Enabled() {
		
		return name, nil
	}

	_, err := d.client.Dynamic().Resource(WorkspaceGVR()).Namespace(d.client.Namespace()).
		Create(ctx, obj, metav1.CreateOptions{})
	if err != nil {
		
		if _, updateErr := d.client.Dynamic().Resource(WorkspaceGVR()).Namespace(d.client.Namespace()).
			Update(ctx, obj, metav1.UpdateOptions{}); updateErr != nil {
			return "", fmt.Errorf("failed to provision workspace %s: %w", name, updateErr)
		}
		return name, nil
	}
	return name, nil
}

func (d *Driver) Deprovision(ctx context.Context, ws *models.Workspace) error {
	if d.client == nil || !d.client.Enabled() {
		return nil
	}
	name := objectName(ws)
	propagation := metav1.DeletePropagationForeground
	if err := d.client.Dynamic().Resource(WorkspaceGVR()).Namespace(d.client.Namespace()).
		Delete(ctx, name, metav1.DeleteOptions{PropagationPolicy: &propagation}); err != nil {
		return fmt.Errorf("failed to deprovision workspace %s: %w", name, err)
	}
	return nil
}

func (d *Driver) Heartbeat(ctx context.Context, wsID string, at time.Time) error {
	if d.store == nil {
		return nil
	}
	ws, err := d.store.GetWorkspaceByID(wsID)
	if err != nil {
		return fmt.Errorf("heartbeat: lookup %s: %w", wsID, err)
	}
	if ws.LastActiveAt != nil && at.Before(*ws.LastActiveAt) {
		
		return nil
	}
	return d.store.TouchWorkspace(wsID, at)
}

func (d *Driver) URL(ws *models.Workspace) (string, error) {
	if ws == nil {
		return "", fmt.Errorf("workspace is nil")
	}
	return fmt.Sprintf("https://nb.%s.%s", objectName(ws), proxyDomain), nil
}

func (d *Driver) ProxyTarget(ws *models.Workspace) (string, error) {
	if ws == nil {
		return "", fmt.Errorf("workspace is nil")
	}
	name := objectName(ws)
	if d.client != nil && d.client.Enabled() {
		return fmt.Sprintf("http://%s.%s.svc:%d", name, d.client.Namespace(), notebookPort), nil
	}
	if d.proxyBaseURL != "" {
		base := strings.TrimRight(d.proxyBaseURL, "/")
		return fmt.Sprintf("%s/%s", base, name), nil
	}
	return "", fmt.Errorf("workspace proxy target unavailable (no cluster, no proxy base URL)")
}

const proxyDomain = "fuze.ai"

const notebookPort = 8888

var _ ports.WorkspaceRuntime = (*Driver)(nil)