package llm

import (
	"regexp"
	"sort"
	"strings"
)

const (
	
	ActionAllow = "allow"
	
	ActionRedact = "redact"
	
	ActionBlock = "block"
)

const (
	
	DirectionInput = "input"
	
	DirectionOutput = "output"
	
	DirectionBoth = "both"
)

const (
	CategorySensitive = "sensitive_word"
	CategoryPII       = "pii"
	CategoryJailbreak = "jailbreak"
)

type Finding struct {
	
	Category string `json:"category"`
	
	Rule string `json:"rule"`
	
	Action string `json:"action"`
	
	Excerpt string `json:"excerpt,omitempty"`
}

type GuardResult struct {
	
	Action string `json:"action"`
	
	Content string `json:"content"`
	
	Findings []Finding `json:"findings,omitempty"`
}

func (r GuardResult) Blocked() bool { return r.Action == ActionBlock }

func (r GuardResult) Modified() bool { return r.Action == ActionRedact }

type Rule struct {
	
	Name string `json:"name"`
	
	Category string `json:"category"`
	
	Direction string `json:"direction"`
	
	Action string `json:"action"`
	
	Pattern string `json:"pattern,omitempty"`
	
	Keywords []string `json:"keywords,omitempty"`
	
	Replacement string `json:"replacement,omitempty"`

	re *regexp.Regexp
}

func (r *Rule) compile() {
	if r.Pattern == "" {
		return
	}
	re, err := regexp.Compile(r.Pattern)
	if err != nil {
		return
	}
	r.re = re
}

func (r *Rule) matches(text string) [][]int {
	if r.re != nil {
		return r.re.FindAllStringIndex(text, -1)
	}
	if len(r.Keywords) == 0 {
		return nil
	}
	lower := strings.ToLower(text)
	var out [][]int
	for _, kw := range r.Keywords {
		if kw == "" {
			continue
		}
		k := strings.ToLower(kw)
		from := 0
		for {
			idx := strings.Index(lower[from:], k)
			if idx < 0 {
				break
			}
			start := from + idx
			out = append(out, []int{start, start + len(k)})
			from = start + len(k)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i][0] < out[j][0] })
	return out
}

func (r *Rule) appliesTo(direction string) bool {
	return r.Direction == DirectionBoth || r.Direction == direction
}

type Guard struct {
	rules []*Rule
}

func NewGuard(rules ...Rule) *Guard {
	g := &Guard{}
	for i := range rules {
		r := rules[i]
		r.compile()
		if r.Direction == "" {
			r.Direction = DirectionBoth
		}
		if r.Action == "" {
			r.Action = ActionBlock
		}
		g.rules = append(g.rules, &r)
	}
	return g
}

func (g *Guard) Rules() []Rule {
	out := make([]Rule, 0, len(g.rules))
	for _, r := range g.rules {
		out = append(out, Rule{
			Name: r.Name, Category: r.Category, Direction: r.Direction,
			Action: r.Action, Pattern: r.Pattern, Keywords: r.Keywords,
			Replacement: r.Replacement,
		})
	}
	return out
}

func (g *Guard) Check(text, direction string) GuardResult {
	res := GuardResult{Action: ActionAllow, Content: text}

	for _, r := range g.rules {
		if !r.appliesTo(direction) {
			continue
		}
		hits := r.matches(text)
		if len(hits) == 0 {
			continue
		}
		if r.Action == ActionBlock {
			res.Action = ActionBlock
			res.Findings = append(res.Findings, Finding{
				Category: r.Category, Rule: r.Name, Action: ActionBlock,
				Excerpt: excerpt(text, hits[0]),
			})
			
			res.Content = ""
			return res
		}
	}

	content := text
	for _, r := range g.rules {
		if !r.appliesTo(direction) || r.Action != ActionRedact {
			continue
		}
		hits := r.matches(content)
		if len(hits) == 0 {
			continue
		}
		res.Findings = append(res.Findings, Finding{
			Category: r.Category, Rule: r.Name, Action: ActionRedact,
			Excerpt: excerpt(content, hits[0]),
		})
		content = redact(content, hits, r.replacement())
		res.Action = ActionRedact
	}
	res.Content = content
	return res
}

func (r *Rule) replacement() string {
	if r.Replacement == "" {
		return "[REDACTED]"
	}
	return r.Replacement
}

func redact(text string, hits [][]int, with string) string {
	if len(hits) == 0 {
		return text
	}
	merged := mergeRanges(hits)
	out := text
	for i := len(merged) - 1; i >= 0; i-- {
		h := merged[i]
		out = out[:h[0]] + with + out[h[1]:]
	}
	return out
}

func mergeRanges(in [][]int) [][]int {
	if len(in) == 0 {
		return nil
	}
	cp := make([][]int, len(in))
	copy(cp, in)
	sort.Slice(cp, func(i, j int) bool { return cp[i][0] < cp[j][0] })
	out := [][]int{cp[0]}
	for _, r := range cp[1:] {
		last := out[len(out)-1]
		if r[0] <= last[1] {
			if r[1] > last[1] {
				last[1] = r[1]
			}
			continue
		}
		out = append(out, r)
	}
	return out
}

func excerpt(text string, hit []int) string {
	if len(hit) < 2 || hit[0] < 0 || hit[1] > len(text) {
		return ""
	}
	s := text[hit[0]:hit[1]]
	if len(s) > 32 {
		s = s[:32] + "..."
	}
	return s
}

func DefaultRules() []Rule {
	return []Rule{
		{
			Name:        "pii_phone_cn",
			Category:    CategoryPII,
			Direction:   DirectionBoth,
			Action:      ActionRedact,
			Pattern:     `1[3-9]\d{9}`,
			Replacement: "[PHONE]",
		},
		{
			Name:      "pii_id_card_cn",
			Category:  CategoryPII,
			Direction: DirectionBoth,
			Action:    ActionRedact,
			
			Pattern:     `[1-9]\d{5}(19|20)\d{2}(0[1-9]|1[0-2])(0[1-9]|[12]\d|3[01])\d{3}[\dXx]`,
			Replacement: "[ID_CARD]",
		},
		{
			Name:        "pii_email",
			Category:    CategoryPII,
			Direction:   DirectionBoth,
			Action:      ActionRedact,
			Pattern:     `[\w.+-]+@[\w-]+\.[\w.-]+`,
			Replacement: "[EMAIL]",
		},
		{
			Name:        "pii_bank_card",
			Category:    CategoryPII,
			Direction:   DirectionBoth,
			Action:      ActionRedact,
			Pattern:     `\b\d{16,19}\b`,
			Replacement: "[BANK_CARD]",
		},
		{
			Name:      "jailbreak_common",
			Category:  CategoryJailbreak,
			Direction: DirectionInput,
			Action:    ActionBlock,
			Keywords: []string{
				"ignore previous instructions",
				"ignore all previous",
				"disregard the above",
				"you are now DAN",
				"developer mode enabled",
				"忽略以上所有指令",
				"忽略之前的指令",
				"忘记你的设定",
			},
		},
	}
}

func NewDefaultGuard() *Guard { return NewGuard(DefaultRules()...) }