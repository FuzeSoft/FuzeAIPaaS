package hpo

import (
	"math"
	"sort"
)

type AshaDecision struct {
	ShouldStop bool
	Reason     string
}

func ashaRungBound(eta float64, rung int) int {
	if rung <= 0 {
		return int(eta) 
	}
	return int(math.Pow(eta, float64(rung+1)))
}

func EvaluateASHA(study *Study, trial *Trial, all []Trial) AshaDecision {
	es := study.EarlyStop.normalized()
	if !es.Enabled || es.Eta <= 1 {
		return AshaDecision{}
	}
	_, ok := trial.LatestValue()
	if !ok {
		return AshaDecision{}
	}
	
	rung := study.rungFor(highestStep(trial))
	
	minStep := ashaRungBound(es.Eta, es.MinRungs-1)
	if highestStep(trial) < minStep {
		return AshaDecision{} 
	}
	_ = rung

	type peer struct {
		id    string
		value float64
		ok    bool
	}
	peers := make([]peer, 0, len(all))
	for i := range all {
		tr := &all[i]
		if tr.ID == trial.ID {
			v, o := trial.LatestValue()
			peers = append(peers, peer{id: tr.ID, value: v, ok: o})
			continue
		}
		if tr.Status == TrialFailed || tr.Status == TrialPruned {
			continue
		}
		
		if study.rungFor(highestStep(tr)) < rung {
			continue
		}
		v, o := tr.LatestValue()
		if !o {
			continue
		}
		peers = append(peers, peer{id: tr.ID, value: v, ok: true})
	}
	if len(peers) < 2 {
		return AshaDecision{} 
	}
	sort.Slice(peers, func(i, j int) bool {
		
		si, sj := peers[i].value*study.Objective.sign(), peers[j].value*study.Objective.sign()
		return si > sj
	})
	
	keep := int(math.Ceil(float64(len(peers)) / es.Eta))
	if keep < 1 {
		keep = 1
	}
	for i := range peers {
		if peers[i].id == trial.ID {
			if i >= keep {
				return AshaDecision{ShouldStop: true, Reason: "asha: below top 1/eta at rung " + itoa(rung)}
			}
			return AshaDecision{}
		}
	}
	return AshaDecision{}
}

func highestStep(t *Trial) int {
	h := 0
	for _, r := range t.Intermediate {
		if r.Step > h {
			h = r.Step
		}
	}
	return h
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}