package data

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"strings"
)

type AnnotationRecord struct {
	ID         string            `json:"id,omitempty"`
	Image      string            `json:"image,omitempty"`      
	Text       string            `json:"text,omitempty"`       
	Category   string            `json:"category"`             
	Score      float64           `json:"score,omitempty"`      
	BBox       []float64         `json:"bbox,omitempty"`       
	Attributes map[string]string `json:"attributes,omitempty"` 
}

func ParseAnnotations(format string, raw []byte) ([]AnnotationRecord, error) {
	switch strings.ToLower(format) {
	case "jsonl":
		return parseJSONL(raw)
	case "coco":
		return parseCOCO(raw)
	case "csv":
		return parseCSV(raw)
	default:
		return nil, fmt.Errorf("unsupported annotation input format %q", format)
	}
}

func ConvertAnnotations(records []AnnotationRecord, format string) ([]byte, error) {
	switch strings.ToLower(format) {
	case "jsonl":
		return toJSONL(records), nil
	case "csv":
		return toCSV(records), nil
	case "coco":
		return toCOCO(records), nil
	default:
		return nil, fmt.Errorf("unsupported annotation output format %q", format)
	}
}

func parseJSONL(raw []byte) ([]AnnotationRecord, error) {
	var out []AnnotationRecord
	for _, line := range splitLines(string(raw)) {
		if line == "" {
			continue
		}
		var r AnnotationRecord
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			return nil, fmt.Errorf("parse jsonl: %w", err)
		}
		out = append(out, r)
	}
	return out, nil
}

func parseCSV(raw []byte) ([]AnnotationRecord, error) {
	r := csv.NewReader(strings.NewReader(string(raw)))
	rows, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("parse csv: %w", err)
	}
	if len(rows) == 0 {
		return nil, nil
	}
	header := rows[0]
	idx := map[string]int{}
	for i, h := range header {
		idx[strings.ToLower(h)] = i
	}
	get := func(row []string, key string) string {
		if i, ok := idx[key]; ok && i < len(row) {
			return row[i]
		}
		return ""
	}
	var out []AnnotationRecord
	for _, row := range rows[1:] {
		rec := AnnotationRecord{
			Image:    get(row, "image"),
			Text:     get(row, "text"),
			Category: get(row, "category"),
		}
		if v := get(row, "score"); v != "" {
			fmt.Sscanf(v, "%f", &rec.Score)
		}
		if v := get(row, "bbox"); v != "" {
			rec.BBox = parseFloats(v)
		}
		out = append(out, rec)
	}
	return out, nil
}

func parseCOCO(raw []byte) ([]AnnotationRecord, error) {
	var coco struct {
		Images []struct {
			ID       int    `json:"id"`
			FileName string `json:"file_name"`
		} `json:"images"`
		Categories []struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
		} `json:"categories"`
		Annotations []struct {
			ID         int       `json:"id"`
			ImageID    int       `json:"image_id"`
			CategoryID int       `json:"category_id"`
			BBox       []float64 `json:"bbox"`
			Score      float64   `json:"score"`
		} `json:"annotations"`
	}
	if err := json.Unmarshal(raw, &coco); err != nil {
		return nil, fmt.Errorf("parse coco: %w", err)
	}
	imgName := map[int]string{}
	for _, im := range coco.Images {
		imgName[im.ID] = im.FileName
	}
	catName := map[int]string{}
	for _, c := range coco.Categories {
		catName[c.ID] = c.Name
	}
	var out []AnnotationRecord
	for _, a := range coco.Annotations {
		out = append(out, AnnotationRecord{
			ID:       fmt.Sprintf("%d", a.ID),
			Image:    imgName[a.ImageID],
			Category: catName[a.CategoryID],
			Score:    a.Score,
			BBox:     a.BBox,
		})
	}
	return out, nil
}

func toJSONL(records []AnnotationRecord) []byte {
	var b strings.Builder
	for _, r := range records {
		line, _ := json.Marshal(r)
		b.Write(line)
		b.WriteByte('\n')
	}
	return []byte(b.String())
}

func toCSV(records []AnnotationRecord) []byte {
	var b strings.Builder
	w := csv.NewWriter(&b)
	_ = w.Write([]string{"id", "image", "text", "category", "score", "bbox"})
	for _, r := range records {
		bbox := ""
		for i, v := range r.BBox {
			if i > 0 {
				bbox += " "
			}
			bbox += fmt.Sprintf("%g", v)
		}
		_ = w.Write([]string{
			r.ID, r.Image, r.Text, r.Category,
			fmt.Sprintf("%g", r.Score), bbox,
		})
	}
	w.Flush()
	return []byte(b.String())
}

func toCOCO(records []AnnotationRecord) []byte {
	type cocoImg struct {
		ID       int    `json:"id"`
		FileName string `json:"file_name"`
	}
	type cocoCat struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}
	type cocoAnn struct {
		ID         int       `json:"id"`
		ImageID    int       `json:"image_id"`
		CategoryID int       `json:"category_id"`
		BBox       []float64 `json:"bbox"`
		Score      float64   `json:"score"`
	}
	imgSet := map[string]int{}
	catSet := map[string]int{}
	var imgs []cocoImg
	var cats []cocoCat
	var anns []cocoAnn
	imgID := 0
	catID := 0
	annID := 0
	for _, r := range records {
		if _, ok := imgSet[r.Image]; !ok {
			imgID++
			imgSet[r.Image] = imgID
			imgs = append(imgs, cocoImg{ID: imgID, FileName: r.Image})
		}
		if _, ok := catSet[r.Category]; !ok {
			catID++
			catSet[r.Category] = catID
			cats = append(cats, cocoCat{ID: catID, Name: r.Category})
		}
		annID++
		anns = append(anns, cocoAnn{
			ID:         annID,
			ImageID:    imgSet[r.Image],
			CategoryID: catSet[r.Category],
			BBox:       r.BBox,
			Score:      r.Score,
		})
	}
	out, _ := json.MarshalIndent(map[string]any{
		"images":      imgs,
		"categories":  cats,
		"annotations": anns,
	}, "", "  ")
	return out
}

func parseFloats(s string) []float64 {
	var out []float64
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		var f float64
		if _, err := fmt.Sscanf(p, "%f", &f); err == nil {
			out = append(out, f)
		}
	}
	return out
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