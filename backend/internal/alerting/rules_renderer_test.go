package alerting

import (
	"strings"
	"testing"

	"fuze-ai-paas/backend/internal/models"

	"gopkg.in/yaml.v3"
)

func TestRenderRulesFiltersDisabledAndEmpty(t *testing.T) {
	rules := []models.AlertRule{
		{Name: "enabled-rule", Expr: "up == 0", For: "5m", Severity: models.SeverityCritical, TenantID: "t1", Enabled: true},
		{Name: "disabled-rule", Expr: "up == 1", Enabled: false}, 
		{Name: "empty-expr", Expr: "", Enabled: true},            
	}
	out, err := RenderRules("tenant-t1", rules)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	var g RuleGroup
	if err := yaml.Unmarshal([]byte(out), &g); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(g.Rules) != 1 {
		t.Fatalf("仅启用且 expr 非空规则应被渲染，got %d", len(g.Rules))
	}
	if g.Rules[0].Alert != "enabled-rule" {
		t.Fatalf("wrong rule rendered: %s", g.Rules[0].Alert)
	}
	if g.Rules[0].Labels["severity"] != "critical" {
		t.Fatalf("severity 应作为标签注入，got %q", g.Rules[0].Labels["severity"])
	}
	if g.Rules[0].Labels["tenant_id"] != "t1" {
		t.Fatalf("tenant_id 应作为标签注入，got %q", g.Rules[0].Labels["tenant_id"])
	}
	if g.Rules[0].Annotations["summary"] != "" {
		t.Fatalf("summary 应进入 annotations")
	}
}

func TestRenderGroupsYAMLMultipleTenants(t *testing.T) {
	groups := map[string][]models.AlertRule{
		"tenant-a": {{Name: "a-rule", Expr: "x > 1", Enabled: true, TenantID: "a"}},
		"platform": {}, 
	}
	out, err := RenderGroupsYAML(groups)
	if err != nil {
		t.Fatalf("render groups: %v", err)
	}
	if !strings.Contains(out, "tenant-a") {
		t.Fatalf("应包含 tenant-a 组")
	}
	if strings.Contains(out, "platform") {
		t.Fatalf("空组不应被渲染: %s", out)
	}
	var doc struct {
		Groups []RuleGroup `yaml:"groups"`
	}
	if err := yaml.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(doc.Groups) != 1 {
		t.Fatalf("应只有 1 个非空组，got %d", len(doc.Groups))
	}
}