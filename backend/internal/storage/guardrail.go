package storage

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"fuze-ai-paas/backend/internal/domain/llm"
	"fuze-ai-paas/backend/internal/models"
	"fuze-ai-paas/backend/internal/ports"

	"gorm.io/gorm"
)

type GuardrailRepo struct {
	db *gorm.DB
}

func NewGuardrailRepository(db *gorm.DB) *GuardrailRepo {
	return &GuardrailRepo{db: db}
}

func (r *GuardrailRepo) Resolve(ctx context.Context, tenantID string) ([]llm.Rule, error) {
	if tenantID != "" {
		tenantRules, err := r.List(ctx, tenantID)
		if err != nil {
			return nil, err
		}
		if rules := toDomainRules(tenantRules); len(rules) > 0 {
			return rules, nil
		}
	}

	globalRules, err := r.List(ctx, "")
	if err != nil {
		return nil, err
	}
	if rules := toDomainRules(globalRules); len(rules) > 0 {
		return rules, nil
	}

	return llm.DefaultRules(), nil
}

func (r *GuardrailRepo) List(ctx context.Context, tenantID string) ([]models.LLMGuardrailRule, error) {
	var out []models.LLMGuardrailRule
	err := r.db.WithContext(ctx).
		Where("tenant_id = ?", tenantID).
		Order("name ASC").
		Find(&out).Error
	if err != nil {
		return nil, fmt.Errorf("list guardrail rules: %w", err)
	}
	return out, nil
}

func (r *GuardrailRepo) Upsert(ctx context.Context, rule *models.LLMGuardrailRule) error {
	if err := validateRule(rule); err != nil {
		return err
	}
	now := now().UTC()
	rule.UpdatedAt = now
	if rule.CreatedAt.IsZero() {
		rule.CreatedAt = now
	}
	if err := r.db.WithContext(ctx).Save(rule).Error; err != nil {
		return fmt.Errorf("save guardrail rule: %w", err)
	}
	return nil
}

func (r *GuardrailRepo) Delete(ctx context.Context, tenantID, id string) error {
	res := r.db.WithContext(ctx).
		Where("id = ? AND tenant_id = ?", id, tenantID).
		Delete(&models.LLMGuardrailRule{})
	if res.Error != nil {
		return fmt.Errorf("delete guardrail rule: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrGuardrailRuleNotFound
	}
	return nil
}

var (
	ErrGuardrailRuleNotFound = ports.ErrGuardrailRuleNotFound
	ErrGuardrailInvalidRule  = ports.ErrGuardrailInvalidRule
)

func validateRule(rule *models.LLMGuardrailRule) error {
	if rule.ID == "" || rule.Name == "" {
		return fmt.Errorf("%w: id 与 name 不能为空", ErrGuardrailInvalidRule)
	}
	if rule.Pattern == "" && strings.TrimSpace(rule.Keywords) == "" {
		return fmt.Errorf("%w: pattern 与 keywords 至少提供一项", ErrGuardrailInvalidRule)
	}
	if rule.Pattern != "" {
		if _, err := regexp.Compile(rule.Pattern); err != nil {
			return fmt.Errorf("%w: 正则无法编译: %v", ErrGuardrailInvalidRule, err)
		}
	}
	switch rule.Action {
	case llm.ActionAllow, llm.ActionRedact, llm.ActionBlock:
	default:
		return fmt.Errorf("%w: 未知 action %q", ErrGuardrailInvalidRule, rule.Action)
	}
	switch rule.Direction {
	case "", llm.DirectionInput, llm.DirectionOutput, llm.DirectionBoth:
	default:
		return fmt.Errorf("%w: 未知 direction %q", ErrGuardrailInvalidRule, rule.Direction)
	}
	return nil
}

func toDomainRules(rows []models.LLMGuardrailRule) []llm.Rule {
	out := make([]llm.Rule, 0, len(rows))
	for _, row := range rows {
		if !row.Enabled {
			continue
		}
		out = append(out, llm.Rule{
			Name:        row.Name,
			Category:    row.Category,
			Direction:   row.Direction,
			Action:      row.Action,
			Pattern:     row.Pattern,
			Keywords:    splitKeywords(row.Keywords),
			Replacement: row.Replacement,
		})
	}
	return out
}

func splitKeywords(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if v := strings.TrimSpace(p); v != "" {
			out = append(out, v)
		}
	}
	return out
}