package hpo

import "math/rand"

type RandomSampler struct{}

func (RandomSampler) Suggest(space SearchSpace, _ []Trial, r *rand.Rand) (map[string]any, error) {
	if err := space.Validate(); err != nil {
		return nil, err
	}
	params := make(map[string]any, len(space.Params))
	for _, p := range space.Params {
		params[p.Name] = sampleParam(p, r)
	}
	return params, nil
}