package models

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestClusterMarshalOmitsKubeConfig(t *testing.T) {
	c := Cluster{
		ID:         "cluster-1",
		Name:       "gpu-prod",
		KubeConfig: "apiVersion: v1\nkind: Config\nusers:\n- name: admin\n  user:\n    token: super-secret",
	}

	b, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	out := string(b)
	if strings.Contains(out, "super-secret") {
		t.Fatalf("kubeconfig secret leaked in JSON output: %s", out)
	}
	if strings.Contains(out, "kube_config") {
		t.Fatalf("kube_config field must not appear in JSON output: %s", out)
	}
	if !strings.Contains(out, "cluster-1") {
		t.Fatalf("expected non-sensitive fields (id) present: %s", out)
	}
}

func TestClusterUnmarshalReadsKubeConfig(t *testing.T) {
	in := `{"name":"gpu-prod","kube_config":"SECRET-YAML"}`
	var c Cluster
	if err := json.Unmarshal([]byte(in), &c); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if c.KubeConfig != "SECRET-YAML" {
		t.Fatalf("expected kubeconfig parsed from request body, got %q", c.KubeConfig)
	}
}