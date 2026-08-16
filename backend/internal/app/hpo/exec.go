package hpo

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"fuze-ai-paas/backend/internal/domain/hpo"
	trainingdomain "fuze-ai-paas/backend/internal/domain/training"
	"fuze-ai-paas/backend/internal/models"
	trainingapp "fuze-ai-paas/backend/internal/app/training"
)

func marshalJSON(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func trainingSpecFromTemplate(tmpl map[string]any) trainingdomain.Spec {
	spec := trainingdomain.Spec{}
	if v, ok := tmpl["image"].(string); ok {
		spec.Image = v
	}
	if v, ok := tmpl["command"].(string); ok {
		spec.Command = v
	}
	if v, ok := tmpl["priority"].(float64); ok {
		spec.Priority = int(v)
	}
	if v, ok := tmpl["gpus"].(float64); ok {
		spec.GPUs = int(v)
	}
	if v, ok := tmpl["memory"].(float64); ok {
		spec.Memory = int(v)
	}
	return spec
}

func newID(prefix string) string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return prefix + "_" + strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return prefix + "_" + hex.EncodeToString(b)
}

func (s *Service) quotaAvailable(tenantID string) bool {
	if s.quota == nil {
		return true
	}
	q, err := s.quota.GetQuota(tenantID)
	if err != nil || q == nil || q.JobQuota <= 0 {
		return true 
	}
	return q.JobUsed < q.JobQuota
}

func renderCommand(template map[string]any, params map[string]any) string {
	flat := renderParams(params)
	base := ""
	if c, ok := template["command"].(string); ok {
		base = c
	}
	if ct, ok := template["command_template"].(string); ok && strings.Contains(ct, "{params}") {
		return strings.ReplaceAll(ct, "{params}", flat)
	}
	if base == "" {
		return flat
	}
	if flat == "" {
		return base
	}
	return base + " " + flat
}

func renderParams(params map[string]any) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("--%s %v", k, params[k]))
	}
	return strings.Join(parts, " ")
}

func (s *Service) spawnTrial(ctx context.Context, study *hpo.Study, params map[string]any) error {
	num := studySpawnNumber(study.ID, s)
	trial := &hpo.Trial{
		ID:      newID("trial"),
		StudyID: study.ID,
		Number:  num,
		Params:  params,
		Status:  hpo.TrialPending,
	}
	tm, err := toModelTrial(trial)
	if err != nil {
		return err
	}
	if err := s.trials.CreateTrial(tm); err != nil {
		return err
	}

	spec := trainingSpecFromTemplate(study.TrainingTemplate)
	command := renderCommand(study.TrainingTemplate, params)
	spec.Command = command
	job, err := s.training.Submit(study.TenantID, trainingapp.SubmitInput{
		Name:   fmt.Sprintf("%s-%d", study.Name, num),
		Spec:   spec,
	})
	if err != nil {
		
		trial.Status = hpo.TrialFailed
		tm2, _ := toModelTrial(trial)
		_ = s.trials.UpdateTrial(tm2)
		return fmt.Errorf("submit trial %s: %w", trial.ID, err)
	}

	if study.ExperimentID != "" {
		hp, _ := marshalJSON(params)
		run := &models.Run{
			ID:              newID("run"),
			ExperimentID:    study.ExperimentID,
			TenantID:        study.TenantID,
			Name:            fmt.Sprintf("%s-trial-%d", study.Name, num),
			Hyperparameters: hp,
			SourceJobID:     job.ID,
			Status:          "pending",
			CreatedAt:       time.Now(),
			UpdatedAt:       time.Now(),
		}
		if err := s.exp.CreateRun(run); err == nil {
			trial.RunID = run.ID
		}
	}

	trial.Status = hpo.TrialRunning
	trial.JobID = job.ID
	tm3, err := toModelTrial(trial)
	if err != nil {
		return err
	}
	return s.trials.UpdateTrial(tm3)
}

func studySpawnNumber(studyID string, s *Service) int {
	list, err := s.trials.ListTrials(studyID)
	if err != nil || len(list) == 0 {
		return 1
	}
	max := 0
	for _, t := range list {
		if t.Number > max {
			max = t.Number
		}
	}
	return max + 1
}

func (s *Service) stopTrial(ctx context.Context, study *hpo.Study, trialID string) error {
	tm, err := s.trials.GetTrial(trialID)
	if err != nil {
		return err
	}
	dt, err := toDomainTrial(tm)
	if err != nil {
		return err
	}
	if dt.Status != hpo.TrialRunning {
		return nil 
	}
	if dt.JobID != "" {
		if _, err := s.training.Cancel(dt.JobID); err != nil {
			return err
		}
	}
	dt.Status = hpo.TrialPruned
	tm2, err := toModelTrial(dt)
	if err != nil {
		return err
	}
	return s.trials.UpdateTrial(tm2)
}

func (s *Service) completeStudy(ctx context.Context, study *hpo.Study, reason string) error {
	study.Status = hpo.StudyCompleted
	if best := hpo.BestTrial(study.Objective, allDomainTrials(s, study.ID)); best != nil {
		study.BestTrialID = best.ID
	}
	m, err := toModelStudy(study)
	if err != nil {
		return err
	}
	return s.studies.UpdateStudy(m)
}

func allDomainTrials(s *Service, studyID string) []hpo.Trial {
	ms, err := s.trials.ListTrials(studyID)
	if err != nil {
		return nil
	}
	out := make([]hpo.Trial, 0, len(ms))
	for i := range ms {
		dt, err := toDomainTrial(&ms[i])
		if err != nil {
			continue
		}
		out = append(out, *dt)
	}
	return out
}

func (s *Service) Report(ctx context.Context, tenantID, trialID string, step int, value float64) (bool, error) {
	tm, err := s.trials.GetTrial(trialID)
	if err != nil {
		return false, err
	}
	dt, err := toDomainTrial(tm)
	if err != nil {
		return false, err
	}
	dt.Intermediate = append(dt.Intermediate, hpo.Report{Step: step, Value: value})
	
	rep := &models.HPOTrialReport{
		ID:        newID("rep"),
		TrialID:   trialID,
		StudyID:   dt.StudyID,
		Step:      step,
		Value:     value,
		CreatedAt: time.Now(),
	}
	tm2, err := toModelTrial(dt)
	if err != nil {
		return false, err
	}
	if err := s.trials.UpdateTrial(tm2); err != nil {
		return false, err
	}
	_ = s.persistReport(rep)

	stop := s.shouldPrune(ctx, dt)
	if stop {
		
		_ = s.stopTrialByID(ctx, dt)
	}
	return stop, nil
}

func (s *Service) shouldPrune(ctx context.Context, trial *hpo.Trial) bool {
	sm, err := s.studies.GetStudy(trial.StudyID)
	if err != nil {
		return false
	}
	study, err := toDomainStudy(sm)
	if err != nil {
		return false
	}
	if study.EarlyStop == nil || !study.EarlyStop.Enabled {
		return false
	}
	all, err := s.trials.ListTrials(study.ID)
	if err != nil {
		return false
	}
	domainAll := make([]hpo.Trial, 0, len(all))
	for i := range all {
		dt, err := toDomainTrial(&all[i])
		if err != nil {
			continue
		}
		domainAll = append(domainAll, *dt)
	}
	dec := hpo.EvaluateASHA(study, trial, domainAll)
	return dec.ShouldStop
}

func (s *Service) stopTrialByID(ctx context.Context, trial *hpo.Trial) error {
	sm, err := s.studies.GetStudy(trial.StudyID)
	if err != nil {
		return err
	}
	study, err := toDomainStudy(sm)
	if err != nil {
		return err
	}
	return s.stopTrial(ctx, study, trial.ID)
}

func (s *Service) persistReport(rep *models.HPOTrialReport) error {
	
	if s.reportRepo == nil {
		return nil
	}
	return s.reportRepo.CreateReport(rep)
}

func (s *Service) ReportFinal(ctx context.Context, tenantID, trialID string, value float64) error {
	tm, err := s.trials.GetTrial(trialID)
	if err != nil {
		return err
	}
	dt, err := toDomainTrial(tm)
	if err != nil {
		return err
	}
	v := value
	dt.Value = &v
	dt.Status = hpo.TrialCompleted
	tm2, err := toModelTrial(dt)
	if err != nil {
		return err
	}
	if err := s.trials.UpdateTrial(tm2); err != nil {
		return err
	}
	
	if dt.RunID != "" {
		_ = s.syncRunFinal(dt.RunID, &v)
	}
	return nil
}

func (s *Service) OnRunCompleted(ctx context.Context, trialID, runStatus string, value *float64) error {
	
	status := hpo.TrialRunning
	switch strings.ToLower(runStatus) {
	case "completed", "succeeded", "success", "done":
		status = hpo.TrialCompleted
	case "failed", "error", "errored", "killed", "evicted":
		status = hpo.TrialFailed
	case "cancelled", "canceled", "pruned":
		status = hpo.TrialCancelled
	}
	tm, err := s.trials.GetTrial(trialID)
	if err != nil {
		return err
	}
	dt, err := toDomainTrial(tm)
	if err != nil {
		return err
	}
	if status == hpo.TrialCompleted && value != nil {
		v := *value
		dt.Value = &v
	}
	dt.Status = status
	tm2, err := toModelTrial(dt)
	if err != nil {
		return err
	}
	if err := s.trials.UpdateTrial(tm2); err != nil {
		return err
	}
	
	return s.RunOnce(ctx, "", dt.StudyID)
}

func (s *Service) syncRunFinal(runID string, value *float64) error {
	run, err := s.exp.GetRun(runID)
	if err != nil {
		return err
	}
	run.Status = "completed"
	run.MetricValue = value
	run.EndedAt = timePtr(time.Now())
	return s.exp.UpdateRun(run)
}

func timePtr(t time.Time) *time.Time { return &t }

func jsonString(v any) (string, error) {
	if v == nil {
		return "", nil
	}
	return marshalJSON(v)
}