
package alerting

import (
	"fmt"
	"maps"

	"fuze-ai-paas/backend/internal/models"

	"gopkg.in/yaml.v3"
)

type RuleGroup struct {
	Name  string      `yaml:"name"`
	Rules []RenderedRule `yaml:"rules"`
}

type RenderedRule struct {
	Alert       string            `yaml:"alert"`
	Expr        string            `yaml:"expr"`
	For         string            `yaml:"for,omitempty"`
	Labels      map[string]string `yaml:"labels,omitempty"`
	Annotations map[string]string `yaml:"annotations,omitempty"`
}

func RenderRules(groupName string, rules []models.AlertRule) (string, error) {
	group := RuleGroup{Name: groupName}
	for _, r := range rules {
		if !r.Enabled || r.Expr == "" {
			continue
		}
		labels := map[string]string{}
		maps.Copy(labels, r.Labels)
		if r.Severity != "" {
			labels["severity"] = string(r.Severity)
		}
		if r.TenantID != "" {
			labels["tenant_id"] = r.TenantID
		}
		annotations := map[string]string{}
		if r.Summary != "" {
			annotations["summary"] = r.Summary
		}
		if r.Description != "" {
			annotations["description"] = r.Description
		}
		group.Rules = append(group.Rules, RenderedRule{
			Alert:       r.Name,
			Expr:        r.Expr,
			For:         r.For,
			Labels:      labels,
			Annotations: annotations,
		})
	}
	if len(group.Rules) == 0 {
		
		group.Rules = []RenderedRule{}
	}
	out, err := yaml.Marshal(group)
	if err != nil {
		return "", fmt.Errorf("render alert rules: %w", err)
	}
	return string(out), nil
}

func RenderGroupsYAML(groups map[string][]models.AlertRule) (string, error) {
	all := make([]RuleGroup, 0, len(groups))
	for key, rules := range groups {
		text, err := RenderRules(key, rules)
		if err != nil {
			return "", err
		}
		var g RuleGroup
		if err := yaml.Unmarshal([]byte(text), &g); err != nil {
			return "", fmt.Errorf("re-parse rendered group: %w", err)
		}
		if len(g.Rules) > 0 {
			all = append(all, g)
		}
	}
	doc := struct {
		Groups []RuleGroup `yaml:"groups"`
	}{Groups: all}
	out, err := yaml.Marshal(doc)
	if err != nil {
		return "", fmt.Errorf("render alert groups: %w", err)
	}
	return string(out), nil
}