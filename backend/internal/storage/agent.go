package storage

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"fuze-ai-paas/backend/internal/domain/agent"
	"fuze-ai-paas/backend/internal/ports"
	"gorm.io/gorm"
)

type agentRepo struct{ db *gorm.DB }

func NewAgentRepository(db *gorm.DB) ports.AgentRepository {
	return &agentRepo{db: db}
}

func (s *Storage) Agent() ports.AgentRepository { return NewAgentRepository(s.db) }

type AgentRow struct {
	ID          string `gorm:"primaryKey;size:64"`
	TenantID    string `gorm:"index;size:64"`
	Name        string `gorm:"index;size:256"`
	Description string `gorm:"type:text"`
	DAGJSON     string `gorm:"column:dag;type:text"`
	Status      string `gorm:"size:32"`
	CreatedBy   string `gorm:"size:64"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (AgentRow) TableName() string { return "agents" }

type AgentRunRow struct {
	ID          string `gorm:"primaryKey;size:64"`
	AgentID     string `gorm:"index;size:64"`
	TenantID    string `gorm:"index;size:64"`
	Status      string `gorm:"size:32"`
	Input       string `gorm:"type:text"`
	ResultsJSON string `gorm:"column:results;type:text"`
	PausedAt    string `gorm:"size:64"`
	PausePrompt string `gorm:"type:text"`
	FinalOutput string `gorm:"type:text"`
	CreatedBy   string `gorm:"size:64"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (AgentRunRow) TableName() string { return "agent_runs" }

func rowFromAgent(a *agent.Agent) (AgentRow, error) {
	b, err := json.Marshal(a.DAG)
	if err != nil {
		return AgentRow{}, err
	}
	return AgentRow{
		ID:          a.ID,
		TenantID:    a.TenantID,
		Name:        a.Name,
		Description: a.Description,
		DAGJSON:     string(b),
		Status:      a.Status,
		CreatedBy:   a.CreatedBy,
		CreatedAt:   a.CreatedAt,
		UpdatedAt:   a.UpdatedAt,
	}, nil
}

func (r AgentRow) toDomain() (*agent.Agent, error) {
	var dag agent.DAG
	if err := json.Unmarshal([]byte(r.DAGJSON), &dag); err != nil {
		return nil, err
	}
	return &agent.Agent{
		ID:          r.ID,
		TenantID:    r.TenantID,
		Name:        r.Name,
		Description: r.Description,
		DAG:         dag,
		Status:      r.Status,
		CreatedBy:   r.CreatedBy,
		CreatedAt:   r.CreatedAt,
		UpdatedAt:   r.UpdatedAt,
	}, nil
}

func rowFromRun(run *agent.Run) (AgentRunRow, error) {
	b, err := json.Marshal(run.Results)
	if err != nil {
		return AgentRunRow{}, err
	}
	return AgentRunRow{
		ID:          run.ID,
		AgentID:     run.AgentID,
		TenantID:    run.TenantID,
		Status:      run.Status,
		Input:       run.Input,
		ResultsJSON: string(b),
		PausedAt:    run.PausedAt,
		PausePrompt: run.PausePrompt,
		FinalOutput: run.FinalOutput,
		CreatedBy:   run.CreatedBy,
		CreatedAt:   run.CreatedAt,
		UpdatedAt:   run.UpdatedAt,
	}, nil
}

func (r AgentRunRow) toDomain() (*agent.Run, error) {
	var results []agent.NodeResult
	if err := json.Unmarshal([]byte(r.ResultsJSON), &results); err != nil {
		return nil, err
	}
	return &agent.Run{
		ID:          r.ID,
		AgentID:     r.AgentID,
		TenantID:    r.TenantID,
		Status:      r.Status,
		Input:       r.Input,
		Results:     results,
		PausedAt:    r.PausedAt,
		PausePrompt: r.PausePrompt,
		FinalOutput: r.FinalOutput,
		CreatedBy:   r.CreatedBy,
		CreatedAt:   r.CreatedAt,
		UpdatedAt:   r.UpdatedAt,
	}, nil
}

func (r *agentRepo) Create(ctx context.Context, a *agent.Agent) error {
	if a == nil {
		return errors.New("storage: agent is nil")
	}
	row, err := rowFromAgent(a)
	if err != nil {
		return err
	}
	row.CreatedAt = now()
	row.UpdatedAt = row.CreatedAt
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&AgentRow{}).
			Where("tenant_id = ? AND name = ?", a.TenantID, a.Name).
			Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return ports.ErrAgentConflict
		}
		return tx.Create(&row).Error
	})
	return err
}

func (r *agentRepo) Get(ctx context.Context, tenantID, id string) (*agent.Agent, error) {
	var row AgentRow
	q := scopeTenant(r.db.WithContext(ctx), tenantID)
	if err := q.First(&row, "id = ?", id).Error; err != nil {
		if isNotFoundErr(err) {
			return nil, ports.ErrAgentNotFound
		}
		return nil, err
	}
	return row.toDomain()
}

func (r *agentRepo) List(ctx context.Context, f ports.AgentFilter) ([]*agent.Agent, error) {
	q := scopeTenant(r.db.WithContext(ctx), f.TenantID)
	if f.Name != "" {
		q = q.Where("name LIKE ?", "%"+f.Name+"%")
	}
	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}
	var rows []AgentRow
	if err := q.Order("created_at DESC").Limit(defaultListLimit).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]*agent.Agent, 0, len(rows))
	for i := range rows {
		a, err := rows[i].toDomain()
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, nil
}

func (r *agentRepo) Update(ctx context.Context, a *agent.Agent) error {
	if a == nil {
		return errors.New("storage: agent is nil")
	}
	row, err := rowFromAgent(a)
	if err != nil {
		return err
	}
	row.UpdatedAt = now()
	res := scopeTenant(r.db.WithContext(ctx), a.TenantID).
		Model(&AgentRow{}).
		Where("id = ?", a.ID).
		Updates(map[string]interface{}{
			"name":        row.Name,
			"description": row.Description,
			"dag":         row.DAGJSON,
			"status":      row.Status,
			"updated_at":  row.UpdatedAt,
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ports.ErrAgentNotFound
	}
	return nil
}

func (r *agentRepo) Delete(ctx context.Context, tenantID, id string) error {
	res := scopeTenant(r.db.WithContext(ctx), tenantID).
		Where("id = ?", id).
		Delete(&AgentRow{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ports.ErrAgentNotFound
	}
	
	r.db.WithContext(ctx).Where("agent_id = ? AND tenant_id = ?", id, tenantID).
		Delete(&AgentRunRow{})
	return nil
}

func (r *agentRepo) CreateRun(ctx context.Context, run *agent.Run) error {
	if run == nil {
		return errors.New("storage: run is nil")
	}
	row, err := rowFromRun(run)
	if err != nil {
		return err
	}
	row.CreatedAt = now()
	row.UpdatedAt = row.CreatedAt
	return r.db.WithContext(ctx).Create(&row).Error
}

func (r *agentRepo) GetRun(ctx context.Context, tenantID, id string) (*agent.Run, error) {
	var row AgentRunRow
	q := scopeTenant(r.db.WithContext(ctx), tenantID)
	if err := q.First(&row, "id = ?", id).Error; err != nil {
		if isNotFoundErr(err) {
			return nil, ports.ErrRunNotFound
		}
		return nil, err
	}
	return row.toDomain()
}

func (r *agentRepo) ListRuns(ctx context.Context, f ports.RunFilter) ([]*agent.Run, error) {
	q := scopeTenant(r.db.WithContext(ctx), f.TenantID)
	if f.AgentID != "" {
		q = q.Where("agent_id = ?", f.AgentID)
	}
	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}
	var rows []AgentRunRow
	if err := q.Order("created_at DESC").Limit(defaultListLimit).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]*agent.Run, 0, len(rows))
	for i := range rows {
		run, err := rows[i].toDomain()
		if err != nil {
			return nil, err
		}
		out = append(out, run)
	}
	return out, nil
}

func (r *agentRepo) UpdateRun(ctx context.Context, run *agent.Run) error {
	if run == nil {
		return errors.New("storage: run is nil")
	}
	row, err := rowFromRun(run)
	if err != nil {
		return err
	}
	row.UpdatedAt = now()
	res := scopeTenant(r.db.WithContext(ctx), run.TenantID).
		Model(&AgentRunRow{}).
		Where("id = ?", run.ID).
		Updates(map[string]interface{}{
			"status":      row.Status,
			"results":     row.ResultsJSON,
			"paused_at":   row.PausedAt,
			"pause_prompt": row.PausePrompt,
			"final_output": row.FinalOutput,
			"updated_at":  row.UpdatedAt,
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ports.ErrRunNotFound
	}
	return nil
}

var _ ports.AgentRepository = (*agentRepo)(nil)