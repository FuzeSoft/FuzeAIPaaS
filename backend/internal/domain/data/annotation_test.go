package data

import (
	"strings"
	"testing"
)

func sampleRecords() []AnnotationRecord {
	return []AnnotationRecord{
		{ID: "1", Image: "a.jpg", Category: "cat", Score: 0.9, BBox: []float64{1, 2, 3, 4}},
		{ID: "2", Image: "b.jpg", Category: "dog", Score: 0.8, BBox: []float64{5, 6, 7, 8}},
	}
}

func TestJSONLRoundTrip(t *testing.T) {
	raw, err := ConvertAnnotations(sampleRecords(), "jsonl")
	if err != nil {
		t.Fatalf("to jsonl: %v", err)
	}
	back, err := ParseAnnotations("jsonl", raw)
	if err != nil {
		t.Fatalf("parse jsonl: %v", err)
	}
	if len(back) != 2 || back[0].Category != "cat" {
		t.Fatalf("jsonl roundtrip mismatch: %+v", back)
	}
}

func TestCSVConversion(t *testing.T) {
	raw, err := ConvertAnnotations(sampleRecords(), "csv")
	if err != nil {
		t.Fatalf("to csv: %v", err)
	}
	s := string(raw)
	if !strings.Contains(s, "image,text,category") {
		t.Fatalf("csv header missing: %s", s)
	}
	if !strings.Contains(s, "a.jpg") || !strings.Contains(s, "cat") {
		t.Fatalf("csv body missing: %s", s)
	}
	
	back, err := ParseAnnotations("csv", raw)
	if err != nil {
		t.Fatalf("parse csv: %v", err)
	}
	if len(back) != 2 || back[1].Category != "dog" {
		t.Fatalf("csv roundtrip mismatch: %+v", back)
	}
}

func TestCOCOConversionAndParse(t *testing.T) {
	raw, err := ConvertAnnotations(sampleRecords(), "coco")
	if err != nil {
		t.Fatalf("to coco: %v", err)
	}
	if !strings.Contains(string(raw), "annotations") {
		t.Fatalf("coco missing annotations: %s", raw)
	}
	back, err := ParseAnnotations("coco", raw)
	if err != nil {
		t.Fatalf("parse coco: %v", err)
	}
	if len(back) != 2 || back[0].Category != "cat" {
		t.Fatalf("coco roundtrip mismatch: %+v", back)
	}
}

func TestConvertRejectsBadFormat(t *testing.T) {
	if _, err := ConvertAnnotations(sampleRecords(), "xml"); err == nil {
		t.Fatal("expected error for xml format")
	}
}