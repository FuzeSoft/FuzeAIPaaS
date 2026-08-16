package edge

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	domainedge "fuze-ai-paas/backend/internal/domain/edge"
)

func TestMockRuntimePushAndRollback(t *testing.T) {
	rt := NewMockRuntime()
	node := &domainedge.EdgeNode{ID: "n1", Mode: domainedge.NodeModeAgent}
	full := &domainedge.EdgeDeployment{ID: "d1", Version: "v1", DesiredSpec: domainedge.EdgeDeploySpec{Replicas: 1}}
	canary := &domainedge.EdgeDeployment{ID: "d2", Version: "v2", CanaryWeight: 20, DesiredSpec: domainedge.EdgeDeploySpec{Replicas: 1}}

	res, err := rt.PushDeployment(context.Background(), node, full)
	if err != nil || !res.Accepted {
		t.Fatalf("push full failed: %v", err)
	}
	st, _ := rt.Status(context.Background(), node, full)
	if !st.Ready {
		t.Fatal("full deploy should be ready")
	}

	if _, err := rt.PushDeployment(context.Background(), node, canary); err != nil {
		t.Fatal(err)
	}
	cs, _ := rt.Status(context.Background(), node, canary)
	if cs.Ready {
		t.Fatal("canary deploy should not be ready while canarying")
	}

	if err := rt.Rollback(context.Background(), node, full, "v0"); err != nil {
		t.Fatal(err)
	}
}

func TestAgentRuntimeEndToEnd(t *testing.T) {
	
	srv := NewMockAgentServer()
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	node := &domainedge.EdgeNode{ID: "n1", Mode: domainedge.NodeModeAgent, Endpoint: ts.URL}
	
	rt := &AgentRuntime{client: ts.Client()}

	dep := &domainedge.EdgeDeployment{
		ID: "d1", ModelID: "m1", Version: "v1",
		DesiredSpec: domainedge.EdgeDeploySpec{Replicas: 2, Image: "img:latest"},
	}
	res, err := rt.PushDeployment(context.Background(), node, dep)
	if err != nil || !res.Accepted {
		t.Fatalf("agent push failed: %v", err)
	}
	st, err := rt.Status(context.Background(), node, dep)
	if err != nil || !st.Found || !st.Ready {
		t.Fatalf("agent status unexpected: %+v err=%v", st, err)
	}
	if err := rt.Rollback(context.Background(), node, dep, "v0"); err != nil {
		t.Fatalf("agent rollback failed: %v", err)
	}
	if len(srv.RolledBack()) != 1 {
		t.Fatalf("expected 1 rollback recorded, got %d", len(srv.RolledBack()))
	}
	
	h, err := rt.Heartbeat(context.Background(), node)
	if err != nil || !h.Online {
		t.Fatalf("agent heartbeat failed: %+v", h)
	}
}

func TestKubeEdgeRuntimeEndToEnd(t *testing.T) {
	
	srv := NewMockCloudHubServer()
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	rt, err := NewKubeEdgeRuntime(KubeEdgeConfig{
		CloudHub:          ts.URL,
		Token:             "test-token",
		Namespace:         "edge",
		InsecureSkipVerify: true,
	})
	if err != nil {
		t.Fatalf("new kubeedge runtime: %v", err)
	}

	node := &domainedge.EdgeNode{
		ID:       "n1",
		TenantID: "t1",
		Mode:     domainedge.NodeModeKubeEdge,
		Endpoint: ts.URL,
		Labels:   map[string]string{"kubernetes.io/hostname": "edge-node-1"},
	}

	dep := &domainedge.EdgeDeployment{
		ID:          "dep-1",
		TenantID:    "t1",
		Version:     "v1",
		ActiveVersion: "v1",
		DesiredSpec: domainedge.EdgeDeploySpec{
			Image:    "infer:v1",
			Replicas: 1,
			CPU:      "500m",
			Memory:   "512Mi",
			HealthCheck: &domainedge.EdgeHealthCheck{Path: "/healthz", Port: 8080, PeriodSeconds: 10},
		},
	}
	res, err := rt.PushDeployment(context.Background(), node, dep)
	if err != nil || !res.Accepted {
		t.Fatalf("kubeedge push failed: %v", err)
	}
	if res.RuntimeID != "dep-1" {
		t.Fatalf("expected runtime id dep-1, got %s", res.RuntimeID)
	}

	st, err := rt.Status(context.Background(), node, dep)
	if err != nil || !st.Found || !st.Ready {
		t.Fatalf("kubeedge status unexpected: %+v err=%v", st, err)
	}
	
	deps := srv.Deployments()
	d, ok := deps["edge/dep-1"]
	if !ok {
		t.Fatal("deployment not recorded in cloudhub")
	}
	tmpl := d.Spec["template"].(map[string]interface{})
	podSpec := tmpl["spec"].(map[string]interface{})
	ns := podSpec["nodeSelector"].(map[string]interface{})
	if ns["kubernetes.io/hostname"] != "edge-node-1" {
		t.Fatalf("nodeSelector wrong: %+v", ns)
	}
	containers := podSpec["containers"].([]interface{})
	c0 := containers[0].(map[string]interface{})
	if c0["image"] != "infer:v1" {
		t.Fatalf("image wrong: %v", c0["image"])
	}
	sc := c0["securityContext"].(map[string]interface{})
	if sc["runAsNonRoot"] != true || sc["readOnlyRootFilesystem"] != true {
		t.Fatalf("security baseline missing: %+v", sc)
	}

	if err := rt.Rollback(context.Background(), node, dep, "infer:v1"); err != nil {
		t.Fatalf("kubeedge rollback failed: %v", err)
	}

	ready := kubeNodeStatus{Conditions: []kubeNodeCondition{
		{Type: "Ready", Status: "True", LastHeartbeatTime: time.Now().UTC().Format(time.RFC3339)},
	}}
	srv.SetNodeStatus("edge-node-1", ready)
	h, err := rt.Heartbeat(context.Background(), node)
	if err != nil || !h.Online {
		t.Fatalf("kubeedge heartbeat should be online: %+v err=%v", h, err)
	}

	stale := kubeNodeStatus{Conditions: []kubeNodeCondition{
		{Type: "Ready", Status: "True", LastHeartbeatTime: time.Now().Add(-10 * time.Minute).UTC().Format(time.RFC3339)},
	}}
	srv.SetNodeStatus("edge-node-1", stale)
	h2, _ := rt.Heartbeat(context.Background(), node)
	if h2.Online {
		t.Fatal("kubeedge heartbeat should be offline when stale")
	}
}

func TestKubeEdgeRuntimeRequiresCloudHubAndToken(t *testing.T) {
	if _, err := NewKubeEdgeRuntime(KubeEdgeConfig{}); err == nil {
		t.Fatal("expected error when CloudHub empty")
	}
	if _, err := NewKubeEdgeRuntime(KubeEdgeConfig{CloudHub: "https://x"}); err == nil {
		t.Fatal("expected error when token empty")
	}
}