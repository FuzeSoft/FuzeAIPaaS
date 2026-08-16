
package operator

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"sort"
)

type Spec struct {
	Operator string
	Params   map[string]any
	Input    string
	Output   string
}

func Run(spec Spec) error {
	switch spec.Operator {
	case "dedup":
		return Dedup(spec.Input, spec.Output, spec.Params)
	case "fillna":
		return Fillna(spec.Input, spec.Output, spec.Params)
	case "drop_outlier":
		return DropOutlier(spec.Input, spec.Output, spec.Params)
	case "normalize":
		return Normalize(spec.Input, spec.Output, spec.Params)
	case "img_flip":
		return ImgFlip(spec.Input, spec.Output, spec.Params)
	case "img_crop":
		return ImgCrop(spec.Input, spec.Output, spec.Params)
	case "text_synonym":
		return TextSynonym(spec.Input, spec.Output, spec.Params)
	case "text_backtranslate":
		return TextBacktranslate(spec.Input, spec.Output, spec.Params)
	case "format_convert":
		return FormatConvert(spec.Input, spec.Output, spec.Params)
	case "split":
		return SplitDispatch(spec.Input, spec.Output, spec.Params)
	case "sample":
		return Sample(spec.Input, spec.Output, spec.Params)
	default:
		return fmt.Errorf("unknown operator %q", spec.Operator)
	}
}

func readJSONL(path string) ([]map[string]any, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var rows []map[string]any
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024*1024), 16*1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			return nil, fmt.Errorf("parse jsonl: %w", err)
		}
		rows = append(rows, m)
	}
	return rows, sc.Err()
}

func writeJSONL(path string, rows []map[string]any) error {
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	defer w.Flush()
	for _, r := range rows {
		b, err := json.Marshal(r)
		if err != nil {
			return err
		}
		if _, err := w.Write(append(b, '\n')); err != nil {
			return err
		}
	}
	return nil
}

func strParam(p map[string]any, key string) string {
	if v, ok := p[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func floatParam(p map[string]any, key string) (float64, bool) {
	if v, ok := p[key]; ok {
		switch n := v.(type) {
		case float64:
			return n, true
		case int:
			return float64(n), true
		}
	}
	return 0, false
}

func Dedup(in, out string, p map[string]any) error {
	rows, err := readJSONL(in)
	if err != nil {
		return err
	}
	key := strParam(p, "key")
	seen := make(map[string]bool)
	var uniq []map[string]any
	for _, r := range rows {
		var k string
		if key != "" {
			k = fmt.Sprintf("%v", r[key])
		} else {
			b, _ := json.Marshal(r)
			k = string(b)
		}
		if seen[k] {
			continue
		}
		seen[k] = true
		uniq = append(uniq, r)
	}
	return writeJSONL(out, uniq)
}

func Fillna(in, out string, p map[string]any) error {
	rows, err := readJSONL(in)
	if err != nil {
		return err
	}
	colsRaw, _ := p["columns"].([]any)
	if colsRaw == nil {
		
		if len(rows) > 0 {
			for k := range rows[0] {
				colsRaw = append(colsRaw, k)
			}
		}
	}
	strategy := strParam(p, "strategy")
	for _, c := range colsRaw {
		col := fmt.Sprintf("%v", c)
		switch strategy {
		case "const":
			val := p["value"]
			for _, r := range rows {
				if r[col] == nil {
					r[col] = val
				}
			}
		case "mean":
			var sum float64
			var n int
			for _, r := range rows {
				if v, ok := toFloat(r[col]); ok {
					sum += v
					n++
				}
			}
			if n > 0 {
				mean := sum / float64(n)
				for _, r := range rows {
					if r[col] == nil {
						r[col] = mean
					}
				}
			}
		case "mode":
			freq := map[string]int{}
			for _, r := range rows {
				if r[col] != nil {
					freq[fmt.Sprintf("%v", r[col])]++
				}
			}
			best := ""
			bestN := -1
			for k, v := range freq {
				if v > bestN {
					bestN = v
					best = k
				}
			}
			for _, r := range rows {
				if r[col] == nil {
					r[col] = best
				}
			}
		default:
			return fmt.Errorf("fillna: unsupported strategy %q", strategy)
		}
	}
	return writeJSONL(out, rows)
}

func DropOutlier(in, out string, p map[string]any) error {
	rows, err := readJSONL(in)
	if err != nil {
		return err
	}
	colsRaw, _ := p["columns"].([]any)
	method := strParam(p, "method")
	threshold, _ := floatParam(p, "threshold")
	if threshold == 0 {
		threshold = 3 
	}
	var kept []map[string]any
	for _, r := range rows {
		drop := false
		for _, c := range colsRaw {
			col := fmt.Sprintf("%v", c)
			v, ok := toFloat(r[col])
			if !ok {
				continue
			}
			if method == "zscore" {
				mu, sd := meanStd(rows, col)
				if sd > 0 && abs((v-mu)/sd) > threshold {
					drop = true
					break
				}
			}
		}
		if !drop {
			kept = append(kept, r)
		}
	}
	return writeJSONL(out, kept)
}

func Normalize(in, out string, p map[string]any) error {
	rows, err := readJSONL(in)
	if err != nil {
		return err
	}
	rulesRaw, _ := p["rules"].([]any)
	ruleCols := map[string]bool{}
	for _, r := range rulesRaw {
		ruleCols[fmt.Sprintf("%v", r)] = true
	}
	anyRule := len(ruleCols) > 0
	for _, r := range rows {
		for k, v := range r {
			if anyRule && !ruleCols[k] {
				continue
			}
			if s, ok := v.(string); ok {
				r[k] = normalizeString(s)
			}
		}
	}
	return writeJSONL(out, rows)
}

func ImgFlip(in, out string, _ map[string]any) error {
	return copyTree(in, out)
}

func ImgCrop(in, out string, _ map[string]any) error {
	return copyTree(in, out)
}

func TextSynonym(in, out string, p map[string]any) error {
	rows, err := readJSONL(in)
	if err != nil {
		return err
	}
	ratio, _ := floatParam(p, "ratio")
	if ratio == 0 {
		ratio = 0.3
	}
	rng := rand.New(rand.NewSource(1))
	for _, r := range rows {
		if s, ok := r["text"].(string); ok {
			r["text"] = synonymReplace(s, ratio, rng)
		}
	}
	return writeJSONL(out, rows)
}

func TextBacktranslate(in, out string, _ map[string]any) error {
	return copyJSONL(in, out)
}

func FormatConvert(in, out string, p map[string]any) error {
	from := strParam(p, "from")
	to := strParam(p, "to")
	if from == "" {
		from = extToFormat(in)
	}
	if to == "" {
		to = extToFormat(out)
	}
	rows, err := readJSONL(in)
	if err != nil {
		return err
	}
	switch to {
	case "jsonl":
		return writeJSONL(out, rows)
	case "csv":
		return writeCSV(out, rows)
	default:
		return fmt.Errorf("format_convert: unsupported target %q", to)
	}
}

func SplitDispatch(in, out string, p map[string]any) error {
	rows, err := readJSONL(in)
	if err != nil {
		return err
	}
	ratio, _ := floatParam(p, "train_ratio")
	if ratio == 0 {
		ratio = 0.8
	}
	seed := 0
	if s, ok := p["seed"].(float64); ok {
		seed = int(s)
	}
	rng := rand.New(rand.NewSource(int64(seed)))
	perm := rng.Perm(len(rows))
	trainN := int(float64(len(rows)) * ratio)
	var train, val []map[string]any
	for i, idx := range perm {
		if i < trainN {
			train = append(train, rows[idx])
		} else {
			val = append(val, rows[idx])
		}
	}
	if err := writeJSONL(out, train); err != nil {
		return err
	}
	valPath := deriveSiblingPath(out, "val")
	return writeJSONL(valPath, val)
}

func Sample(in, out string, p map[string]any) error {
	rows, err := readJSONL(in)
	if err != nil {
		return err
	}
	rng := rand.New(rand.NewSource(7))
	perm := rng.Perm(len(rows))
	var n int
	if f, ok := floatParam(p, "frac"); ok {
		n = int(float64(len(rows)) * f)
	} else if v, ok := p["n"].(float64); ok {
		n = int(v)
	} else {
		n = len(rows)
	}
	if n > len(rows) {
		n = len(rows)
	}
	var sampled []map[string]any
	for i := 0; i < n; i++ {
		sampled = append(sampled, rows[perm[i]])
	}
	return writeJSONL(out, sampled)
}

func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case nil:
		return 0, false
	default:
		return 0, false
	}
}

func meanStd(rows []map[string]any, col string) (float64, float64) {
	var sum, n float64
	for _, r := range rows {
		if v, ok := toFloat(r[col]); ok {
			sum += v
			n++
		}
	}
	if n == 0 {
		return 0, 0
	}
	mu := sum / n
	var vars float64
	for _, r := range rows {
		if v, ok := toFloat(r[col]); ok {
			vars += (v - mu) * (v - mu)
		}
	}
	return mu, sqrt(vars / n)
}

func writeCSV(path string, rows []map[string]any) error {
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	if len(rows) == 0 {
		return nil
	}
	
	cols := []string{}
	for k := range rows[0] {
		cols = append(cols, k)
	}
	sort.Strings(cols)
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	defer w.Flush()
	if err := w.Write(cols); err != nil {
		return err
	}
	for _, r := range rows {
		rec := make([]string, len(cols))
		for i, c := range cols {
			rec[i] = fmt.Sprintf("%v", r[c])
		}
		if err := w.Write(rec); err != nil {
			return err
		}
	}
	return nil
}

func extToFormat(p string) string {
	switch filepath.Ext(p) {
	case ".csv":
		return "csv"
	case ".jsonl", ".ndjson":
		return "jsonl"
	default:
		return "jsonl"
	}
}

func deriveSiblingPath(out, suffix string) string {
	ext := filepath.Ext(out)
	base := out[:len(out)-len(ext)]
	return base + "." + suffix + ext
}

func copyJSONL(in, out string) error {
	rows, err := readJSONL(in)
	if err != nil {
		return err
	}
	return writeJSONL(out, rows)
}

func copyTree(in, out string) error {
	if dir := filepath.Dir(out); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	info, err := os.Stat(in)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return copyJSONL(in, out)
	}
	return filepath.Walk(in, func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(in, p)
		dst := filepath.Join(out, rel)
		if fi.IsDir() {
			return os.MkdirAll(dst, 0o755)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(dst, b, 0o644)
	})
}

var synonymDict = map[string]string{
	"good": "great", "bad": "poor", "fast": "quick", "big": "large", "small": "tiny",
}

func synonymReplace(s string, ratio float64, rng *rand.Rand) string {
	words := splitWords(s)
	for i, w := range words {
		if syn, ok := synonymDict[w]; ok && rng.Float64() < ratio {
			words[i] = syn
		}
	}
	return joinWords(words)
}

func splitLines(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		out = append(out, s[start:])
	}
	return out
}

func splitWords(s string) []string {
	var out []string
	cur := ""
	for _, r := range s {
		if r == ' ' || r == '\t' || r == ',' || r == '.' {
			if cur != "" {
				out = append(out, cur)
				cur = ""
			}
		} else {
			cur += string(r)
		}
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}

func joinWords(ws []string) string {
	out := ""
	for i, w := range ws {
		if i > 0 {
			out += " "
		}
		out += w
	}
	return out
}

func normalizeString(s string) string {
	out := []rune(s)
	for i, r := range out {
		if r >= 'A' && r <= 'Z' {
			out[i] = r + ('a' - 'A')
		}
	}
	return string(out)
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

func sqrt(x float64) float64 {
	if x < 0 {
		return 0
	}
	z := x
	for i := 0; i < 20; i++ {
		z = z - (z*z-x)/(2*z)
	}
	return z
}

var _ = reflect.DeepEqual