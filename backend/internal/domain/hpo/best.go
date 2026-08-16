package hpo

func BestTrial(obj Objective, trials []Trial) *Trial {
	var best *Trial
	for i := range trials {
		tr := &trials[i]
		if tr.Status != TrialCompleted || tr.Value == nil {
			continue
		}
		if best == nil || obj.IsBetter(tr.Value, best.Value) {
			best = tr
		}
	}
	return best
}

func TopTrials(obj Objective, trials []Trial, k int) []Trial {
	done := make([]Trial, 0, len(trials))
	for i := range trials {
		if trials[i].Status == TrialCompleted && trials[i].Value != nil {
			done = append(done, trials[i])
		}
	}
	
	sortBySign(done, obj)
	if k > 0 && len(done) > k {
		done = done[:k]
	}
	return done
}

func sortBySign(trials []Trial, obj Objective) {
	sign := obj.sign()
	for i := 1; i < len(trials); i++ {
		for j := i; j > 0; j-- {
			vi, vj := 0.0, 0.0
			if trials[j].Value != nil {
				vi = *trials[j].Value * sign
			}
			if trials[j-1].Value != nil {
				vj = *trials[j-1].Value * sign
			}
			if vi > vj {
				trials[j], trials[j-1] = trials[j-1], trials[j]
			} else {
				break
			}
		}
	}
}