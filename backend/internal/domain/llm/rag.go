package llm

import (
	"errors"
	"math"
	"sort"
	"strings"
)

var (
	
	ErrEmptyDocument = errors.New("llm: document content must not be empty")
	
	ErrInvalidChunkSize = errors.New("llm: chunk size must be positive")
	
	ErrInvalidOverlap = errors.New("llm: chunk overlap must be non-negative and less than size")
	
	ErrDimensionMismatch = errors.New("llm: vector dimension mismatch")
)

type Segment struct {
	
	Index int `json:"index"`
	
	Content string `json:"content"`
	
	Start int `json:"start"`
}

type ChunkOptions struct {
	
	Size int
	
	Overlap int
	
	Separators []string
}

func DefaultChunkOptions() ChunkOptions {
	return ChunkOptions{
		Size:    512,
		Overlap: 64,
		
		Separators: []string{"\n\n", "\n", "。", ". ", "！", "？", "；", " "},
	}
}

func (o ChunkOptions) Validate() error {
	if o.Size <= 0 {
		return ErrInvalidChunkSize
	}
	if o.Overlap < 0 || o.Overlap >= o.Size {
		return ErrInvalidOverlap
	}
	return nil
}

func SplitText(text string, opt ChunkOptions) ([]Segment, error) {
	if strings.TrimSpace(text) == "" {
		return nil, ErrEmptyDocument
	}
	if err := opt.Validate(); err != nil {
		return nil, err
	}

	runes := []rune(text)
	total := len(runes)
	if total <= opt.Size {
		return []Segment{{Index: 0, Content: text, Start: 0}}, nil
	}

	var segs []Segment
	start := 0
	for start < total {
		end := start + opt.Size
		if end >= total {
			segs = append(segs, Segment{Index: len(segs), Content: string(runes[start:total]), Start: start})
			break
		}
		
		cut := findCut(runes, start, end, opt)
		segs = append(segs, Segment{Index: len(segs), Content: string(runes[start:cut]), Start: start})

		next := cut - opt.Overlap
		if next <= start {
			
			next = cut
		}
		start = next
	}
	return segs, nil
}

func findCut(runes []rune, start, end int, opt ChunkOptions) int {
	lower := start + opt.Size/2
	if lower <= start {
		lower = start + 1
	}
	window := string(runes[start:end])
	for _, sep := range opt.Separators {
		if sep == "" {
			continue
		}
		idx := strings.LastIndex(window, sep)
		if idx < 0 {
			continue
		}
		
		cut := start + len([]rune(window[:idx+len(sep)]))
		if cut > lower && cut < end {
			return cut
		}
	}
	return end
}

type Document struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	
	Source string `json:"source,omitempty"`
	
	Content string `json:"content"`
}

type ScoredSegment struct {
	
	DocumentID string `json:"document_id"`
	
	Title string `json:"title,omitempty"`
	
	Segment Segment `json:"segment"`
	
	Score float64 `json:"score"`
}

func CosineSimilarity(a, b []float32) (float64, error) {
	if len(a) != len(b) {
		return 0, ErrDimensionMismatch
	}
	if len(a) == 0 {
		return 0, nil
	}
	var dot, na, nb float64
	for i := range a {
		x, y := float64(a[i]), float64(b[i])
		dot += x * y
		na += x * x
		nb += y * y
	}
	if na == 0 || nb == 0 {
		return 0, nil
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb)), nil
}

func RankByScore(in []ScoredSegment) {
	sort.SliceStable(in, func(i, j int) bool {
		if in[i].Score != in[j].Score {
			return in[i].Score > in[j].Score
		}
		if in[i].DocumentID != in[j].DocumentID {
			return in[i].DocumentID < in[j].DocumentID
		}
		return in[i].Segment.Index < in[j].Segment.Index
	})
}

func TopK(in []ScoredSegment, k int) []ScoredSegment {
	if k <= 0 || k >= len(in) {
		return in
	}
	return in[:k]
}

func KeywordScore(query, text string) float64 {
	terms := tokenize(query)
	if len(terms) == 0 {
		return 0
	}
	lower := strings.ToLower(text)
	hit := 0
	for _, term := range terms {
		if strings.Contains(lower, term) {
			hit++
		}
	}
	return float64(hit) / float64(len(terms))
}

func tokenize(s string) []string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return nil
	}
	var terms []string
	var latin strings.Builder
	var cjk []rune

	flushLatin := func() {
		if latin.Len() > 0 {
			terms = append(terms, latin.String())
			latin.Reset()
		}
	}
	flushCJK := func() {
		switch {
		case len(cjk) == 1:
			terms = append(terms, string(cjk))
		case len(cjk) > 1:
			for i := 0; i+1 < len(cjk); i++ {
				terms = append(terms, string(cjk[i:i+2]))
			}
		}
		cjk = cjk[:0]
	}

	for _, r := range s {
		switch {
		case isCJK(r):
			flushLatin()
			cjk = append(cjk, r)
		case isWordRune(r):
			flushCJK()
			latin.WriteRune(r)
		default:
			flushLatin()
			flushCJK()
		}
	}
	flushLatin()
	flushCJK()
	return terms
}

func isWordRune(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
}

func BuildContext(hits []ScoredSegment) string {
	if len(hits) == 0 {
		return ""
	}
	var b strings.Builder
	for i, h := range hits {
		if i > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString("[")
		b.WriteString(itoa(i + 1))
		b.WriteString("] ")
		if h.Title != "" {
			b.WriteString(h.Title)
			b.WriteString("\n")
		}
		b.WriteString(h.Segment.Content)
	}
	return b.String()
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