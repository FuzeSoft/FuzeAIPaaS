package llm

import (
	"strings"
	"testing"
)

func TestChunkOptionsValidate(t *testing.T) {
	if err := (ChunkOptions{Size: 0}).Validate(); err != ErrInvalidChunkSize {
		t.Fatalf("want ErrInvalidChunkSize, got %v", err)
	}
	if err := (ChunkOptions{Size: 10, Overlap: -1}).Validate(); err != ErrInvalidOverlap {
		t.Fatalf("want ErrInvalidOverlap, got %v", err)
	}
	
	if err := (ChunkOptions{Size: 10, Overlap: 10}).Validate(); err != ErrInvalidOverlap {
		t.Fatalf("want ErrInvalidOverlap for overlap==size, got %v", err)
	}
	if err := DefaultChunkOptions().Validate(); err != nil {
		t.Fatalf("default options invalid: %v", err)
	}
}

func TestSplitTextShortDocument(t *testing.T) {
	segs, err := SplitText("short text", DefaultChunkOptions())
	if err != nil {
		t.Fatalf("SplitText: %v", err)
	}
	if len(segs) != 1 || segs[0].Content != "short text" {
		t.Fatalf("segments = %+v, want single full segment", segs)
	}
}

func TestSplitTextEmpty(t *testing.T) {
	if _, err := SplitText("   ", DefaultChunkOptions()); err != ErrEmptyDocument {
		t.Fatalf("want ErrEmptyDocument, got %v", err)
	}
}

func TestSplitTextDoesNotBreakMultibyteRunes(t *testing.T) {
	text := strings.Repeat("中文测试内容", 200)
	segs, err := SplitText(text, ChunkOptions{Size: 50, Overlap: 5})
	if err != nil {
		t.Fatalf("SplitText: %v", err)
	}
	if len(segs) < 2 {
		t.Fatalf("expected multiple segments, got %d", len(segs))
	}
	for _, s := range segs {
		if !utf8Valid(s.Content) {
			t.Fatalf("segment %d contains invalid utf-8: %q", s.Index, s.Content)
		}
	}
}

func utf8Valid(s string) bool {
	for _, r := range s {
		if r == '\uFFFD' {
			return false
		}
	}
	return true
}

func TestSplitTextCoversWholeDocument(t *testing.T) {
	text := strings.Repeat("abcdefghij", 100) 
	segs, err := SplitText(text, ChunkOptions{Size: 100, Overlap: 0})
	if err != nil {
		t.Fatalf("SplitText: %v", err)
	}
	var joined strings.Builder
	for _, s := range segs {
		joined.WriteString(s.Content)
	}
	if joined.String() != text {
		t.Fatalf("content lost: got %d chars, want %d", len(joined.String()), len(text))
	}
}

func TestSplitTextOverlapApplied(t *testing.T) {
	text := strings.Repeat("x", 300)
	segs, err := SplitText(text, ChunkOptions{Size: 100, Overlap: 20})
	if err != nil {
		t.Fatalf("SplitText: %v", err)
	}
	if len(segs) < 2 {
		t.Fatalf("expected multiple segments, got %d", len(segs))
	}
	
	firstEnd := segs[0].Start + len([]rune(segs[0].Content))
	if segs[1].Start >= firstEnd {
		t.Fatalf("no overlap: seg1 ends at %d, seg2 starts at %d", firstEnd, segs[1].Start)
	}
}

func TestSplitTextTerminatesWithLargeOverlap(t *testing.T) {
	text := strings.Repeat("y", 500)
	segs, err := SplitText(text, ChunkOptions{Size: 10, Overlap: 9})
	if err != nil {
		t.Fatalf("SplitText: %v", err)
	}
	if len(segs) == 0 {
		t.Fatal("no segments produced")
	}
}

func TestSplitTextPrefersSeparator(t *testing.T) {
	text := strings.Repeat("一句话内容。", 60)
	segs, err := SplitText(text, ChunkOptions{
		Size: 60, Overlap: 0, Separators: []string{"。"},
	})
	if err != nil {
		t.Fatalf("SplitText: %v", err)
	}
	if len(segs) < 2 {
		t.Fatalf("expected multiple segments, got %d", len(segs))
	}
	
	for _, s := range segs[:len(segs)-1] {
		if !strings.HasSuffix(s.Content, "。") {
			t.Fatalf("segment %d not cut at separator: %q", s.Index, s.Content)
		}
	}
}

func TestCosineSimilarity(t *testing.T) {
	a := []float32{1, 0, 0}
	if got, err := CosineSimilarity(a, a); err != nil || got < 0.999 {
		t.Fatalf("self similarity = %v, err=%v, want ~1", got, err)
	}
	if got, _ := CosineSimilarity([]float32{1, 0}, []float32{0, 1}); got != 0 {
		t.Fatalf("orthogonal similarity = %v, want 0", got)
	}
	if got, _ := CosineSimilarity([]float32{1, 0}, []float32{-1, 0}); got > -0.999 {
		t.Fatalf("opposite similarity = %v, want ~-1", got)
	}
}

func TestCosineSimilarityDimensionMismatch(t *testing.T) {
	if _, err := CosineSimilarity([]float32{1, 2}, []float32{1}); err != ErrDimensionMismatch {
		t.Fatalf("want ErrDimensionMismatch, got %v", err)
	}
}

func TestCosineSimilarityZeroVector(t *testing.T) {
	got, err := CosineSimilarity([]float32{0, 0}, []float32{1, 1})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 0 {
		t.Fatalf("zero vector similarity = %v, want 0", got)
	}
	if got, _ := CosineSimilarity(nil, nil); got != 0 {
		t.Fatalf("empty vector similarity = %v, want 0", got)
	}
}

func TestRankByScoreIsDeterministic(t *testing.T) {
	in := []ScoredSegment{
		{DocumentID: "b", Segment: Segment{Index: 0}, Score: 0.5},
		{DocumentID: "a", Segment: Segment{Index: 1}, Score: 0.5},
		{DocumentID: "c", Segment: Segment{Index: 0}, Score: 0.9},
	}
	RankByScore(in)
	if in[0].DocumentID != "c" {
		t.Fatalf("highest score not first: %+v", in[0])
	}
	
	if in[1].DocumentID != "a" || in[2].DocumentID != "b" {
		t.Fatalf("tie-break not deterministic: %+v", in)
	}
}

func TestTopK(t *testing.T) {
	in := []ScoredSegment{{Score: 3}, {Score: 2}, {Score: 1}}
	if got := TopK(in, 2); len(got) != 2 {
		t.Fatalf("TopK(2) len = %d, want 2", len(got))
	}
	if got := TopK(in, 0); len(got) != 3 {
		t.Fatalf("TopK(0) should return all, got %d", len(got))
	}
	if got := TopK(in, 99); len(got) != 3 {
		t.Fatalf("TopK(99) should return all, got %d", len(got))
	}
}

func TestKeywordScoreEnglish(t *testing.T) {
	if got := KeywordScore("golang testing", "Golang is great for TESTING code"); got != 1 {
		t.Fatalf("KeywordScore = %v, want 1", got)
	}
	if got := KeywordScore("golang rust", "golang only"); got != 0.5 {
		t.Fatalf("KeywordScore = %v, want 0.5", got)
	}
	if got := KeywordScore("", "anything"); got != 0 {
		t.Fatalf("empty query score = %v, want 0", got)
	}
}

func TestKeywordScoreChinese(t *testing.T) {
	hit := KeywordScore("知识库检索", "本平台支持知识库检索能力")
	if hit == 0 {
		t.Fatal("chinese query failed to match relevant text")
	}
	miss := KeywordScore("知识库检索", "今天天气很好适合出门")
	if miss >= hit {
		t.Fatalf("irrelevant text scored %v >= relevant %v", miss, hit)
	}
}

func TestBuildContext(t *testing.T) {
	if got := BuildContext(nil); got != "" {
		t.Fatalf("empty hits should yield empty context, got %q", got)
	}
	hits := []ScoredSegment{
		{DocumentID: "d1", Title: "Doc One", Segment: Segment{Content: "first"}},
		{DocumentID: "d2", Title: "Doc Two", Segment: Segment{Content: "second"}},
	}
	got := BuildContext(hits)
	
	for _, want := range []string{"[1]", "[2]", "Doc One", "Doc Two", "first", "second"} {
		if !strings.Contains(got, want) {
			t.Fatalf("context missing %q:\n%s", want, got)
		}
	}
}