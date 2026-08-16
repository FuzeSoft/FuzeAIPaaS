package hpo

import (
	"fmt"
	"math"
	"math/rand"
	"time"
)

type ErrUnknownAlgorithm string

func (a ErrUnknownAlgorithm) Error() string {
	return fmt.Sprintf("unknown hpo algorithm: %q", string(a))
}

const (
	ActionSpawnTrial   = "spawn_trial"   
	ActionStopTrial    = "stop_trial"    
	ActionCompleteStudy = "complete_study" 
	ActionNoop         = "noop"          
)

type Action struct {
	Kind    string
	TrialID string 
	
	Params map[string]any
	
	Reason string
}

type SchedulingContext struct {
	
	QuotaAvailable bool
	
	Now time.Time
	
	Rand *rand.Rand
}

func NextActions(study *Study, trials []Trial, ctx SchedulingContext) []Action {
	if study == nil || !study.IsActive() {
		return []Action{{Kind: ActionNoop}}
	}

	totalSpawned := len(trials)
	running := runningCount(trials)
	done := completedCount(trials)
	failed := failedCount(trials)
	effective := done + failed 

	if study.MaxDuration > 0 && !ctx.Now.IsZero() &&
		!study.CreatedAt.IsZero() && ctx.Now.Sub(study.CreatedAt) >= study.MaxDuration {
		acts := make([]Action, 0, running+1)
		for i := range trials {
			if trials[i].Status == TrialRunning {
				acts = append(acts, Action{Kind: ActionStopTrial, TrialID: trials[i].ID, Reason: "study max duration reached"})
			}
		}
		acts = append(acts, Action{Kind: ActionCompleteStudy, Reason: "max duration reached"})
		return acts
	}

	if study.MaxTrials > 0 && effective >= study.MaxTrials && running == 0 {
		return []Action{{Kind: ActionCompleteStudy, Reason: "max trials reached"}}
	}

	if study.MaxParallel > 0 && running > study.MaxParallel {
		acts := []Action{}
		excess := running - study.MaxParallel
		
		runningTrials := make([]*Trial, 0, running)
		for i := range trials {
			if trials[i].Status == TrialRunning {
				runningTrials = append(runningTrials, &trials[i])
			}
		}
		sortRunningWorstFirst(runningTrials, study.Objective)
		for i := 0; i < len(runningTrials) && excess > 0; i++ {
			dec := EvaluateASHA(study, runningTrials[i], trials)
			if dec.ShouldStop {
				acts = append(acts, Action{Kind: ActionStopTrial, TrialID: runningTrials[i].ID, Reason: dec.Reason})
				excess--
			}
		}
		if len(acts) > 0 {
			return acts
		}
	}

	if !ctx.QuotaAvailable {
		return []Action{{Kind: ActionNoop, Reason: "quota unavailable"}}
	}

	if study.MaxParallel > 0 && running >= study.MaxParallel {
		return []Action{{Kind: ActionNoop, Reason: "parallelism saturated"}}
	}
	if study.MaxTrials > 0 && totalSpawned >= study.MaxTrials {
		return []Action{{Kind: ActionNoop, Reason: "max trials reached"}}
	}

	sampler, err := NewSampler(study.Algorithm, study.Objective)
	if err != nil {
		return []Action{{Kind: ActionNoop, Reason: "unknown algorithm: " + study.Algorithm}}
	}
	params, err := sampler.Suggest(study.Space, trials, ctx.Rand)
	if err != nil {
		
		if isExhausted(err) {
			return []Action{{Kind: ActionCompleteStudy, Reason: "search space exhausted"}}
		}
		return []Action{{Kind: ActionNoop, Reason: "sampling error: " + err.Error()}}
	}
	return []Action{{Kind: ActionSpawnTrial, Params: params}}
}

func NewSampler(algorithm string, obj Objective) (Sampler, error) {
	switch algorithm {
	case "random", "":
		return RandomSampler{}, nil
	case "grid":
		return GridSampler{}, nil
	case "tpe":
		return TPESampler{Objective: obj}, nil
	default:
		return nil, ErrUnknownAlgorithm(algorithm)
	}
}

func sortRunningWorstFirst(ts []*Trial, obj Objective) {
	sign := obj.sign()
	for i := 1; i < len(ts); i++ {
		for j := i; j > 0; j-- {
			vi, vj := worstVal(ts[j], sign), worstVal(ts[j-1], sign)
			if vi < vj { 
				ts[j], ts[j-1] = ts[j-1], ts[j]
			} else {
				break
			}
		}
	}
}

func worstVal(t *Trial, sign float64) float64 {
	v, ok := t.LatestValue()
	if !ok {
		return math.Inf(-1) 
	}
	return v * sign
}

func isExhausted(err error) bool {
	if err == nil {
		return false
	}
	return err == ErrExhausted || containsStr(err.Error(), "exhausted")
}

func containsStr(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}