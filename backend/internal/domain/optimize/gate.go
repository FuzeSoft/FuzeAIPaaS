package optimize

import (
	"fmt"
)

const gateEpsilon = 1e-9

type GateVerdict struct {
	Pass     bool    `json:"pass"`      
	AccDelta float64 `json:"acc_delta"` 
	Reason   string  `json:"reason,omitempty"`
}

func EvaluateGate(origAcc, compAcc, threshold float64) GateVerdict {
	delta := origAcc - compAcc
	if delta <= threshold+gateEpsilon {
		return GateVerdict{Pass: true, AccDelta: delta}
	}
	return GateVerdict{
		Pass:     false,
		AccDelta: delta,
		Reason:   fmt.Sprintf("accuracy dropped by %.4f, exceeds allowed threshold %.4f", delta, threshold),
	}
}