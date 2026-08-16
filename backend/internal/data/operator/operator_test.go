package operator

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func writeNDJSON(t *testing.T, dir, name string, rows []map[string]any) string {
	t.Helper()
	p := filepath.Join(dir, name)
	f, err := os.Create(p)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	defer f.Close()
	for _, r := range rows {
		b, _ := json.Marshal(r)
		f.Write(append(b, '\n'))
	}
	return p
}

func readNDJSON(t *testing.T, p string) []map[string]any {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read %s: %v", p, err)
	}
	var out []map[string]any
	for _, line := range splitLines(string(b)) {
		if line == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("unmarshal line: %v", err)
		}
		out = append(out, m)
	}
	return out
}

func TestDedup(t *testing.T) {
	dir := t.TempDir()
	in := writeNDJSON(t, dir, "in.jsonl", []map[string]any{
		{"id": 1, "text": "a"},
		{"id": 2, "text": "a"}, 
		{"id": 3, "text": "b"},
	})
	out := filepath.Join(dir, "out.jsonl")
	if err := Dedup(in, out, map[string]any{"method": "exact", "key": "text"}); err != nil {
		t.Fatalf("dedup: %v", err)
	}
	rows := readNDJSON(t, out)
	if len(rows) != 2 {
		t.Fatalf("expected 2 unique rows, got %d", len(rows))
	}
}

func TestFillna(t *testing.T) {
	dir := t.TempDir()
	in := writeNDJSON(t, dir, "in.jsonl", []map[string]any{
		{"x": 1.0},
		{"x": nil},
	})
	out := filepath.Join(dir, "out.jsonl")
	if err := Fillna(in, out, map[string]any{"strategy": "const", "columns": []any{"x"}, "value": 0.0}); err != nil {
		t.Fatalf("fillna: %v", err)
	}
	rows := readNDJSON(t, out)
	if rows[1]["x"] != 0.0 {
		t.Fatalf("expected filled 0, got %v", rows[1]["x"])
	}
}

func TestFormatConvertJSONLToCSV(t *testing.T) {
	dir := t.TempDir()
	in := writeNDJSON(t, dir, "in.jsonl", []map[string]any{
		{"a": "1", "b": "2"},
		{"a": "3", "b": "4"},
	})
	out := filepath.Join(dir, "out.csv")
	if err := FormatConvert(in, out, map[string]any{"from": "jsonl", "to": "csv"}); err != nil {
		t.Fatalf("format_convert: %v", err)
	}
	b, _ := os.ReadFile(out)
	if len(splitLines(string(b))) != 3 { 
		t.Fatalf("expected 3 csv lines, got %v", splitLines(string(b)))
	}
}

func TestSplit(t *testing.T) {
	dir := t.TempDir()
	var rows []map[string]any
	for i := 0; i < 10; i++ {
		rows = append(rows, map[string]any{"i": i})
	}
	in := writeNDJSON(t, dir, "in.jsonl", rows)
	train := filepath.Join(dir, "train.jsonl")
	if err := SplitDispatch(in, train, map[string]any{"train_ratio": 0.8, "seed": 42}); err != nil {
		t.Fatalf("split: %v", err)
	}
	val := filepath.Join(dir, "train.val.jsonl")
	tr := readNDJSON(t, train)
	va := readNDJSON(t, val)
	if len(tr) != 8 || len(va) != 2 {
		t.Fatalf("expected 8/2 split, got %d/%d", len(tr), len(va))
	}
}

func TestDispatchBuiltin(t *testing.T) {
	dir := t.TempDir()
	in := writeNDJSON(t, dir, "in.jsonl", []map[string]any{{"text": "a"}, {"text": "a"}})
	out := filepath.Join(dir, "out.jsonl")
	spec := Spec{Operator: "dedup", Params: map[string]any{"method": "exact", "key": "text"}, Input: in, Output: out}
	if err := Run(spec); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if len(readNDJSON(t, out)) != 1 {
		t.Fatalf("dispatch dedup failed")
	}
}

func TestDispatchUnknown(t *testing.T) {
	spec := Spec{Operator: "nope", Input: "x", Output: "y"}
	if err := Run(spec); err == nil {
		t.Fatal("expected error for unknown operator")
	}
}