
package edge

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"fuze-ai-paas/backend/internal/domain/edge"
)

const edgeNodeHostnameLabel = "kubernetes.io/hostname"

const (
	edgeDeploymentLabel = "fuze.ai/edge-deployment-id"
	edgeTenantLabel     = "fuze.ai/tenant-id"
	edgeManagedByLabel  = "managed-by"
)

type MockRuntime struct {
	mu        sync.Mutex
	statuses  map[string]edge.EdgeRuntimeStatus 
	nodes     map[string]edge.EdgeNodeHealth    
	onRollback func(deploymentID, toVersion string)
}

func NewMockRuntime() *MockRuntime {
	return &MockRuntime{
		statuses: map[string]edge.EdgeRuntimeStatus{},
		nodes:    map[string]edge.EdgeNodeHealth{},
	}
}

func (m *MockRuntime) PushDeployment(_ context.Context, _ *edge.EdgeNode, d *edge.EdgeDeployment) (edge.EdgePushResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ready := d.CanaryWeight == 0 
	m.statuses[d.ID] = edge.EdgeRuntimeStatus{Found: true, Ready: ready, Replicas: d.DesiredSpec.Replicas, URL: "mock://" + d.ID}
	return edge.EdgePushResult{Accepted: true, Message: "mock accepted", RuntimeID: "mock-" + d.ID}, nil
}

func (m *MockRuntime) Status(_ context.Context, _ *edge.EdgeNode, d *edge.EdgeDeployment) (edge.EdgeRuntimeStatus, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	st, ok := m.statuses[d.ID]
	if !ok {
		return edge.EdgeRuntimeStatus{Found: false}, nil
	}
	return st, nil
}

func (m *MockRuntime) Rollback(_ context.Context, _ *edge.EdgeNode, d *edge.EdgeDeployment, toVersion string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.onRollback != nil {
		m.onRollback(d.ID, toVersion)
	}
	m.statuses[d.ID] = edge.EdgeRuntimeStatus{Found: true, Ready: true, Replicas: d.DesiredSpec.Replicas, URL: "mock://" + d.ID}
	return nil
}

func (m *MockRuntime) Heartbeat(_ context.Context, n *edge.EdgeNode) (edge.EdgeNodeHealth, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	h := edge.EdgeNodeHealth{Online: true, LoadPercent: 10}
	m.nodes[n.ID] = h
	return h, nil
}

func (m *MockRuntime) SetStatus(deploymentID string, st edge.EdgeRuntimeStatus) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.statuses[deploymentID] = st
}

type agentPushRequest struct {
	DeploymentID string          `json:"deploymentId"`
	ModelID      string          `json:"modelId"`
	Version      string          `json:"version"`
	Spec         edge.EdgeDeploySpec `json:"spec"`
	CanaryVersion string         `json:"canaryVersion,omitempty"`
	CanaryWeight  int            `json:"canaryWeight,omitempty"`
	ActiveVersion string         `json:"activeVersion,omitempty"`
}

type AgentRuntime struct {
	client *http.Client
	
	timeout time.Duration
}

func NewAgentRuntime(caPEM, clientPEM, clientKey string) (*AgentRuntime, error) {
	cert, err := tls.X509KeyPair([]byte(clientPEM), []byte(clientKey))
	if err != nil {
		return nil, fmt.Errorf("edge: load client cert: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM([]byte(caPEM)) {
		return nil, fmt.Errorf("edge: invalid CA cert")
	}
	return &AgentRuntime{
		client: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					Certificates: []tls.Certificate{cert},
					RootCAs:      pool,
					MinVersion:   tls.VersionTLS12,
				},
			},
		},
	}, nil
}

func (a *AgentRuntime) PushDeployment(ctx context.Context, node *edge.EdgeNode, d *edge.EdgeDeployment) (edge.EdgePushResult, error) {
	body, _ := json.Marshal(agentPushRequest{
		DeploymentID:  d.ID,
		ModelID:       d.ModelID,
		Version:       d.Version,
		Spec:          d.DesiredSpec,
		CanaryVersion: d.CanaryVersion,
		CanaryWeight:  d.CanaryWeight,
		ActiveVersion: d.ActiveVersion,
	})
	var out edge.EdgePushResult
	if err := a.post(ctx, node.Endpoint+"/v1/deploy", body, &out); err != nil {
		return edge.EdgePushResult{Accepted: false, Message: err.Error()}, err
	}
	return out, nil
}

func (a *AgentRuntime) Status(ctx context.Context, node *edge.EdgeNode, d *edge.EdgeDeployment) (edge.EdgeRuntimeStatus, error) {
	var out edge.EdgeRuntimeStatus
	if err := a.get(ctx, node.Endpoint+"/v1/deploy/"+d.ID+"/status", &out); err != nil {
		return edge.EdgeRuntimeStatus{Found: false}, err
	}
	return out, nil
}

func (a *AgentRuntime) Rollback(ctx context.Context, node *edge.EdgeNode, d *edge.EdgeDeployment, toVersion string) error {
	body, _ := json.Marshal(map[string]string{"deploymentId": d.ID, "toVersion": toVersion})
	return a.post(ctx, node.Endpoint+"/v1/deploy/"+d.ID+"/rollback", body, nil)
}

func (a *AgentRuntime) Heartbeat(ctx context.Context, node *edge.EdgeNode) (edge.EdgeNodeHealth, error) {
	var out edge.EdgeNodeHealth
	if err := a.get(ctx, node.Endpoint+"/v1/health", &out); err != nil {
		return edge.EdgeNodeHealth{Online: false, Message: err.Error()}, err
	}
	return out, nil
}

func (a *AgentRuntime) post(ctx context.Context, url string, body []byte, out interface{}) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	return a.do(req, out)
}

func (a *AgentRuntime) get(ctx context.Context, url string, out interface{}) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	return a.do(req, out)
}

func (a *AgentRuntime) do(req *http.Request, out interface{}) error {
	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return fmt.Errorf("edge agent: reading response: %w", err)
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("edge agent: status %d: %s", resp.StatusCode, string(data))
	}
	if out != nil && len(data) > 0 {
		return json.Unmarshal(data, out)
	}
	return nil
}

type MockAgentServer struct {
	mu         sync.Mutex
	accepted   map[string]agentPushRequest
	statuses   map[string]edge.EdgeRuntimeStatus
	rolledBack []string
	handler    http.Handler
	server     *http.Server
}

func NewMockAgentServer() *MockAgentServer {
	m := &MockAgentServer{
		accepted: map[string]agentPushRequest{},
		statuses: map[string]edge.EdgeRuntimeStatus{},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/health", m.hHealth)
	mux.HandleFunc("/v1/deploy", m.hDeploy)
	mux.HandleFunc("/v1/deploy/", m.hDeployItem)
	m.handler = mux
	return m
}

func (m *MockAgentServer) Handler() http.Handler { return m.handler }

func (m *MockAgentServer) hHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, edge.EdgeNodeHealth{Online: true, LoadPercent: 12})
}

func (m *MockAgentServer) hDeploy(w http.ResponseWriter, r *http.Request) {
	var req agentPushRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	m.mu.Lock()
	m.accepted[req.DeploymentID] = req
	m.statuses[req.DeploymentID] = edge.EdgeRuntimeStatus{Found: true, Ready: req.CanaryWeight == 0, Replicas: req.Spec.Replicas, URL: "agent://" + req.DeploymentID}
	m.mu.Unlock()
	writeJSON(w, edge.EdgePushResult{Accepted: true, Message: "accepted", RuntimeID: "agent-" + req.DeploymentID})
}

func (m *MockAgentServer) hDeployItem(w http.ResponseWriter, r *http.Request) {
	
	did := deployIDOf(r.URL.Path)
	if did == "" {
		http.NotFound(w, r)
		return
	}
	switch {
	case matchSuffix(r.URL.Path, "/status"):
		m.mu.Lock()
		st := m.statuses[did]
		m.mu.Unlock()
		if !st.Found {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, st)
		return
	case matchSuffix(r.URL.Path, "/rollback"):
		m.mu.Lock()
		m.rolledBack = append(m.rolledBack, did)
		m.statuses[did] = edge.EdgeRuntimeStatus{Found: true, Ready: true}
		m.mu.Unlock()
		writeJSON(w, map[string]string{"ok": "true"})
		return
	}
	http.NotFound(w, r)
}

func deployIDOf(p string) string {
	const prefix = "/v1/deploy/"
	if len(p) <= len(prefix) || p[:len(prefix)] != prefix {
		return ""
	}
	rest := p[len(prefix):]
	if i := indexOf(rest, "/"); i >= 0 {
		return rest[:i]
	}
	return rest
}

func (m *MockAgentServer) RolledBack() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]string, len(m.rolledBack))
	copy(out, m.rolledBack)
	return out
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

func matchSuffix(s, suf string) bool { return len(s) >= len(suf) && s[len(s)-len(suf):] == suf }

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

type kubeObjectMeta struct {
	Name            string            `json:"name"`
	Namespace       string            `json:"namespace"`
	ResourceVersion string            `json:"resourceVersion,omitempty"`
	Labels          map[string]string `json:"labels,omitempty"`
}

type kubeDeploymentStatus struct {
	Replicas          int `json:"replicas"`
	ReadyReplicas     int `json:"readyReplicas"`
	AvailableReplicas int `json:"availableReplicas"`
}

type kubeNodeCondition struct {
	Type               string `json:"type"`
	Status             string `json:"status"`
	LastHeartbeatTime  string `json:"lastHeartbeatTime"`
	LastTransitionTime string `json:"lastTransitionTime"`
}
type kubeNodeStatus struct {
	Conditions []kubeNodeCondition `json:"conditions"`
}

type KubeEdgeRuntime struct {
	
	cloudHub string
	
	token string
	
	namespace string
	
	client *http.Client
	
	timeout time.Duration
	
	now func() time.Time
}

type KubeEdgeConfig struct {
	
	CloudHub string
	
	Token string
	
	Namespace string
	
	CACertPEM string
	
	InsecureSkipVerify bool
}

func NewKubeEdgeRuntime(cfg KubeEdgeConfig) (*KubeEdgeRuntime, error) {
	if cfg.CloudHub == "" {
		return nil, fmt.Errorf("edge kubeedge: CloudHub base empty")
	}
	if cfg.Token == "" {
		return nil, fmt.Errorf("edge kubeedge: token empty")
	}
	ns := cfg.Namespace
	if ns == "" {
		ns = "edge"
	}
	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12}
	if cfg.InsecureSkipVerify {
		tlsCfg.InsecureSkipVerify = true
	} else if cfg.CACertPEM != "" {
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM([]byte(cfg.CACertPEM)) {
			return nil, fmt.Errorf("edge kubeedge: invalid CA cert")
		}
		tlsCfg.RootCAs = pool
	}
	return &KubeEdgeRuntime{
		cloudHub: strings.TrimRight(cfg.CloudHub, "/"),
		token:    cfg.Token,
		namespace: ns,
		client: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{TLSClientConfig: tlsCfg},
		},
		timeout: 30 * time.Second,
		now:     time.Now,
	}, nil
}

func kubeNodeName(node *edge.EdgeNode) string {
	if h, ok := node.Labels[edgeNodeHostnameLabel]; ok && h != "" {
		return h
	}
	if u, err := url.Parse(node.Endpoint); err == nil && u.Host != "" {
		return u.Hostname()
	}
	return node.ID
}

func kubeDeploymentName(deploymentID string) string {
	name := strings.ToLower(deploymentID)
	name = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-':
			return r
		default:
			return '-'
		}
	}, name)
	if len(name) > 63 {
		name = name[:63]
	}
	if name == "" {
		name = "edge"
	}
	return name
}

func effectiveImage(d *edge.EdgeDeployment) string {
	if d.CanaryWeight > 0 && d.CanaryVersion != "" {
		
		return d.DesiredSpec.Image
	}
	if d.DesiredSpec.Image != "" {
		return d.DesiredSpec.Image
	}
	return d.Version
}

func buildEdgeDeployment(namespace string, node *edge.EdgeNode, d *edge.EdgeDeployment) map[string]interface{} {
	name := kubeDeploymentName(d.ID)
	labels := map[string]interface{}{
		edgeDeploymentLabel: d.ID,
		edgeTenantLabel:     d.TenantID,
		edgeManagedByLabel:  "fuze-edge",
	}
	if d.TenantID != "" {
		labels["fuze.ai/tenant-id"] = d.TenantID
	}
	replicas := d.DesiredSpec.Replicas
	if replicas <= 0 {
		replicas = 1
	}
	image := effectiveImage(d)

	container := map[string]interface{}{
		"name":  "inference",
		"image": image,
		"resources": resourceRequirements(d.DesiredSpec),
		"securityContext": map[string]interface{}{
			"runAsNonRoot":             true,
			"runAsUser":                int64(1000),
			"privileged":               false,
			"allowPrivilegeEscalation": false,
			"readOnlyRootFilesystem":   true,
			"capabilities":             map[string]interface{}{"drop": []interface{}{"ALL"}},
		},
	}
	if hc := d.DesiredSpec.HealthCheck; hc != nil && hc.Port > 0 {
		probe := map[string]interface{}{
			"httpGet": map[string]interface{}{
				"path": orDefault(hc.Path, "/healthz"),
				"port": int64(hc.Port),
			},
			"initialDelaySeconds": int64(hc.InitialDelaySeconds),
			"periodSeconds":       int64(hc.PeriodSeconds),
		}
		container["livenessProbe"] = probe
		container["readinessProbe"] = probe
	}

	return map[string]interface{}{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata": map[string]interface{}{
			"name":      name,
			"namespace": namespace,
			"labels":    labels,
		},
		"spec": map[string]interface{}{
			"replicas": int64(replicas),
			"selector": map[string]interface{}{
				"matchLabels": map[string]interface{}{edgeDeploymentLabel: d.ID},
			},
			"template": map[string]interface{}{
				"metadata": map[string]interface{}{
					"labels": map[string]interface{}{edgeDeploymentLabel: d.ID},
				},
				"spec": map[string]interface{}{
					"nodeSelector": map[string]interface{}{
						edgeNodeHostnameLabel: kubeNodeName(node),
					},
					"containers": []interface{}{container},
				},
			},
		},
	}
}

func resourceRequirements(spec edge.EdgeDeploySpec) map[string]interface{} {
	limits := map[string]interface{}{}
	requests := map[string]interface{}{}
	if spec.CPU != "" {
		limits["cpu"] = spec.CPU
		requests["cpu"] = spec.CPU
	}
	if spec.Memory != "" {
		limits["memory"] = spec.Memory
		requests["memory"] = spec.Memory
	}
	if spec.GPUs > 0 {
		limits["nvidia.com/gpu"] = int64(spec.GPUs)
		requests["nvidia.com/gpu"] = int64(spec.GPUs)
	}
	return map[string]interface{}{"limits": limits, "requests": requests}
}

func (k *KubeEdgeRuntime) PushDeployment(ctx context.Context, node *edge.EdgeNode, d *edge.EdgeDeployment) (edge.EdgePushResult, error) {
	obj := buildEdgeDeployment(k.namespace, node, d)
	name := obj["metadata"].(map[string]interface{})["name"].(string)
	body, err := json.Marshal(obj)
	if err != nil {
		return edge.EdgePushResult{Accepted: false, Message: err.Error()}, err
	}
	
	createPath := fmt.Sprintf("/apis/apps/v1/namespaces/%s/deployments", k.namespace)
	if err := k.rest(ctx, http.MethodPost, createPath, body, nil); err != nil {
		if !strings.Contains(err.Error(), "409") && !strings.Contains(strings.ToLower(err.Error()), "already exists") {
			return edge.EdgePushResult{Accepted: false, Message: err.Error()}, err
		}
		
		var existing kubeObjectMeta
		getPath := fmt.Sprintf("/apis/apps/v1/namespaces/%s/deployments/%s", k.namespace, name)
		if gerr := k.rest(ctx, http.MethodGet, getPath, nil, &existing); gerr != nil {
			return edge.EdgePushResult{Accepted: false, Message: gerr.Error()}, gerr
		}
		obj["metadata"].(map[string]interface{})["resourceVersion"] = existing.ResourceVersion
		putBody, _ := json.Marshal(obj)
		if perr := k.rest(ctx, http.MethodPut, getPath, putBody, nil); perr != nil {
			return edge.EdgePushResult{Accepted: false, Message: perr.Error()}, perr
		}
	}
	return edge.EdgePushResult{Accepted: true, Message: "kubeedge applied", RuntimeID: name}, nil
}

func (k *KubeEdgeRuntime) Status(ctx context.Context, _ *edge.EdgeNode, d *edge.EdgeDeployment) (edge.EdgeRuntimeStatus, error) {
	name := kubeDeploymentName(d.ID)
	getPath := fmt.Sprintf("/apis/apps/v1/namespaces/%s/deployments/%s", k.namespace, name)
	var dep struct {
		Metadata kubeObjectMeta       `json:"metadata"`
		Spec     map[string]interface{} `json:"spec"`
		Status   kubeDeploymentStatus  `json:"status"`
	}
	if err := k.rest(ctx, http.MethodGet, getPath, nil, &dep); err != nil {
		if strings.Contains(err.Error(), "404") {
			return edge.EdgeRuntimeStatus{Found: false}, nil
		}
		return edge.EdgeRuntimeStatus{Found: false}, err
	}
	replicas := dep.Status.Replicas
	ready := dep.Status.ReadyReplicas
	res := edge.EdgeRuntimeStatus{
		Found:    true,
		Ready:    replicas > 0 && ready >= replicas,
		Failed:   replicas > 0 && dep.Status.AvailableReplicas == 0,
		Replicas: ready,
		URL:      fmt.Sprintf("%s/namespaces/%s/deployments/%s", k.cloudHub, k.namespace, name),
	}
	return res, nil
}

func (k *KubeEdgeRuntime) Rollback(ctx context.Context, node *edge.EdgeNode, d *edge.EdgeDeployment, toVersion string) error {
	
	rb := *d
	rb.DesiredSpec.Image = toVersion
	if rb.DesiredSpec.Image == "" {
		rb.DesiredSpec.Image = d.ActiveVersion
	}
	obj := buildEdgeDeployment(k.namespace, node, &rb)
	name := obj["metadata"].(map[string]interface{})["name"].(string)
	getPath := fmt.Sprintf("/apis/apps/v1/namespaces/%s/deployments/%s", k.namespace, name)
	var existing kubeObjectMeta
	if err := k.rest(ctx, http.MethodGet, getPath, nil, &existing); err != nil {
		return err
	}
	obj["metadata"].(map[string]interface{})["resourceVersion"] = existing.ResourceVersion
	body, _ := json.Marshal(obj)
	return k.rest(ctx, http.MethodPut, getPath, body, nil)
}

func (k *KubeEdgeRuntime) Heartbeat(ctx context.Context, node *edge.EdgeNode) (edge.EdgeNodeHealth, error) {
	name := kubeNodeName(node)
	path := fmt.Sprintf("/api/v1/nodes/%s/status", name)
	var ns struct {
		Status kubeNodeStatus `json:"status"`
	}
	if err := k.rest(ctx, http.MethodGet, path, nil, &ns); err != nil {
		return edge.EdgeNodeHealth{Online: false, Message: err.Error()}, err
	}
	now := k.now()
	for _, c := range ns.Status.Conditions {
		if c.Type == "Ready" {
			online := c.Status == "True"
			var hb time.Time
			if t, err := time.Parse(time.RFC3339, c.LastHeartbeatTime); err == nil {
				hb = t
			}
			
			if hb.IsZero() || now.Sub(hb) > 5*time.Minute {
				online = false
			}
			msg := ""
			if !online {
				msg = "node not Ready / heartbeat stale"
			}
			return edge.EdgeNodeHealth{Online: online, Message: msg}, nil
		}
	}
	return edge.EdgeNodeHealth{Online: false, Message: "no Ready condition"}, nil
}

func (k *KubeEdgeRuntime) rest(ctx context.Context, method, path string, body []byte, out interface{}) error {
	var rdr io.Reader
	if len(body) > 0 {
		rdr = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, k.cloudHub+path, rdr)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+k.token)
	if len(body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	resp, err := k.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return fmt.Errorf("edge kubeedge: reading response: %w", err)
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("edge kubeedge: %s %s -> %d: %s", method, path, resp.StatusCode, string(data))
	}
	if out != nil && len(data) > 0 {
		return json.Unmarshal(data, out)
	}
	return nil
}

type cloudHubDeployment struct {
	Meta   kubeObjectMeta       `json:"metadata"`
	Spec   map[string]interface{} `json:"spec"`
	Status kubeDeploymentStatus  `json:"status"`
}

type MockCloudHubServer struct {
	mu          sync.Mutex
	deployments map[string]*cloudHubDeployment
	nodes       map[string]kubeNodeStatus
	handler     http.Handler
}

func NewMockCloudHubServer() *MockCloudHubServer {
	m := &MockCloudHubServer{
		deployments: map[string]*cloudHubDeployment{},
		nodes:       map[string]kubeNodeStatus{},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/apis/apps/v1/namespaces/", m.hDeployments)
	mux.HandleFunc("/api/v1/nodes/", m.hNodes)
	m.handler = mux
	return m
}

func (m *MockCloudHubServer) Handler() http.Handler { return m.handler }

func (m *MockCloudHubServer) SetNodeStatus(nodeName string, st kubeNodeStatus) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nodes[nodeName] = st
}

func (m *MockCloudHubServer) Deployments() map[string]*cloudHubDeployment {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := map[string]*cloudHubDeployment{}
	for k, v := range m.deployments {
		out[k] = v
	}
	return out
}

func (m *MockCloudHubServer) hDeployments(w http.ResponseWriter, r *http.Request) {
	
	rest := strings.TrimPrefix(r.URL.Path, "/apis/apps/v1/namespaces/")
	parts := strings.Split(rest, "/")
	if len(parts) < 2 || parts[0] == "" || parts[1] != "deployments" {
		http.NotFound(w, r)
		return
	}
	ns := parts[0]
	var name string
	if len(parts) >= 3 {
		name = parts[2]
	}

	switch r.Method {
	case http.MethodPost:
		var dep cloudHubDeployment
		if err := json.NewDecoder(r.Body).Decode(&dep); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		n := dep.Meta.Name
		m.mu.Lock()
		if _, exists := m.deployments[ns+"/"+n]; exists {
			m.mu.Unlock()
			http.Error(w, "deployments.apps \""+n+"\" already exists", 409)
			return
		}
		dep.Meta.Namespace = ns
		dep.Status = kubeDeploymentStatus{Replicas: 1, ReadyReplicas: 1, AvailableReplicas: 1}
		m.deployments[ns+"/"+n] = &dep
		m.mu.Unlock()
		writeJSON(w, dep)
		return
	case http.MethodGet:
		if name == "" {
			
			m.mu.Lock()
			items := make([]*cloudHubDeployment, 0, len(m.deployments))
			for _, d := range m.deployments {
				items = append(items, d)
			}
			m.mu.Unlock()
			writeJSON(w, map[string]interface{}{"items": items})
			return
		}
		m.mu.Lock()
		dep, ok := m.deployments[ns+"/"+name]
		m.mu.Unlock()
		if !ok {
			http.Error(w, "not found", 404)
			return
		}
		writeJSON(w, dep)
		return
	case http.MethodPut:
		var dep cloudHubDeployment
		if err := json.NewDecoder(r.Body).Decode(&dep); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		m.mu.Lock()
		dep.Meta.Namespace = ns
		dep.Status = kubeDeploymentStatus{Replicas: 1, ReadyReplicas: 1, AvailableReplicas: 1}
		m.deployments[ns+"/"+name] = &dep
		m.mu.Unlock()
		writeJSON(w, dep)
		return
	}
	http.Error(w, "method not allowed", 405)
}

func (m *MockCloudHubServer) hNodes(w http.ResponseWriter, r *http.Request) {
	
	rest := strings.TrimPrefix(r.URL.Path, "/api/v1/nodes/")
	name := strings.TrimSuffix(rest, "/status")
	if name == "" {
		http.NotFound(w, r)
		return
	}
	m.mu.Lock()
	st, ok := m.nodes[name]
	m.mu.Unlock()
	if !ok {
		http.Error(w, "node not found", 404)
		return
	}
	writeJSON(w, map[string]interface{}{"status": st})
}