package llm

import (
	"errors"
	"sort"
	"strings"
)

var (
	
	ErrEmptyTemplate = errors.New("llm: template content must not be empty")
	
	ErrMissingVariable = errors.New("llm: missing required template variable")
	
	ErrUnclosedPlaceholder = errors.New("llm: unclosed template placeholder")
	
	ErrNoVersion = errors.New("llm: prompt has no version")
	
	ErrVersionNotFound = errors.New("llm: prompt version not found")
	
	ErrInvalidWeight = errors.New("llm: variant weight must be positive")
)

type PromptTemplate struct {
	Content string `json:"content"`
}

func (t PromptTemplate) Variables() ([]string, error) {
	seen := make(map[string]struct{})
	rest := t.Content
	for {
		start := strings.Index(rest, "{{")
		if start < 0 {
			break
		}
		rest = rest[start+2:]
		end := strings.Index(rest, "}}")
		if end < 0 {
			return nil, ErrUnclosedPlaceholder
		}
		name := strings.TrimSpace(rest[:end])
		if name != "" {
			seen[name] = struct{}{}
		}
		rest = rest[end+2:]
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out, nil
}

func (t PromptTemplate) Render(vars map[string]string) (string, error) {
	if strings.TrimSpace(t.Content) == "" {
		return "", ErrEmptyTemplate
	}
	names, err := t.Variables()
	if err != nil {
		return "", err
	}
	for _, n := range names {
		if _, ok := vars[n]; !ok {
			return "", ErrMissingVariable
		}
	}

	var b strings.Builder
	rest := t.Content
	for {
		start := strings.Index(rest, "{{")
		if start < 0 {
			b.WriteString(rest)
			break
		}
		b.WriteString(rest[:start])
		rest = rest[start+2:]
		end := strings.Index(rest, "}}")
		if end < 0 {
			return "", ErrUnclosedPlaceholder
		}
		name := strings.TrimSpace(rest[:end])
		b.WriteString(vars[name])
		rest = rest[end+2:]
	}
	return b.String(), nil
}

type PromptVersion struct {
	Version int    `json:"version"`
	Content string `json:"content"`
	
	Note string `json:"note,omitempty"`
	
	Weight int `json:"weight,omitempty"`
}

func (v PromptVersion) Template() PromptTemplate {
	return PromptTemplate{Content: v.Content}
}

type Prompt struct {
	Name     string          `json:"name"`
	Versions []PromptVersion `json:"versions"`
	
	ActiveVersion int `json:"active_version"`
}

func (p *Prompt) AddVersion(content, note string) (PromptVersion, error) {
	if strings.TrimSpace(content) == "" {
		return PromptVersion{}, ErrEmptyTemplate
	}
	
	if _, err := (PromptTemplate{Content: content}).Variables(); err != nil {
		return PromptVersion{}, err
	}
	next := 1
	for _, v := range p.Versions {
		if v.Version >= next {
			next = v.Version + 1
		}
	}
	nv := PromptVersion{Version: next, Content: content, Note: note}
	p.Versions = append(p.Versions, nv)
	return nv, nil
}

func (p *Prompt) Version(n int) (PromptVersion, error) {
	for _, v := range p.Versions {
		if v.Version == n {
			return v, nil
		}
	}
	return PromptVersion{}, ErrVersionNotFound
}

func (p *Prompt) Latest() (PromptVersion, error) {
	if len(p.Versions) == 0 {
		return PromptVersion{}, ErrNoVersion
	}
	best := p.Versions[0]
	for _, v := range p.Versions[1:] {
		if v.Version > best.Version {
			best = v
		}
	}
	return best, nil
}

func (p *Prompt) Active() (PromptVersion, error) {
	if p.ActiveVersion == 0 {
		return p.Latest()
	}
	return p.Version(p.ActiveVersion)
}

func (p *Prompt) Activate(n int) error {
	if _, err := p.Version(n); err != nil {
		return err
	}
	p.ActiveVersion = n
	return nil
}

func (p *Prompt) PickVariant(seed uint64) (PromptVersion, error) {
	weighted := make([]PromptVersion, 0, len(p.Versions))
	total := 0
	for _, v := range p.Versions {
		if v.Weight > 0 {
			weighted = append(weighted, v)
			total += v.Weight
		}
	}
	if total == 0 {
		
		return p.Active()
	}
	sort.Slice(weighted, func(i, j int) bool { return weighted[i].Version < weighted[j].Version })

	bucket := int(seed % uint64(total))
	acc := 0
	for _, v := range weighted {
		acc += v.Weight
		if bucket < acc {
			return v, nil
		}
	}
	return weighted[len(weighted)-1], nil
}

func (p *Prompt) SetWeight(version, weight int) error {
	if weight < 0 {
		return ErrInvalidWeight
	}
	for i := range p.Versions {
		if p.Versions[i].Version == version {
			p.Versions[i].Weight = weight
			return nil
		}
	}
	return ErrVersionNotFound
}

func HashSeed(s string) uint64 {
	const (
		offset = 1469598103934665603
		prime  = 1099511628211
	)
	var h uint64 = offset
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= prime
	}
	return h
}