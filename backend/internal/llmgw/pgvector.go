package llmgw

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"

	"fuze-ai-paas/backend/internal/domain/llm"
	"fuze-ai-paas/backend/internal/ports"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type pgVectorRow struct {
	ID         string `gorm:"column:id;primaryKey"`
	Collection string `gorm:"column:collection;index"`
	DocumentID string `gorm:"column:document_id"`
	Title      string `gorm:"column:title"`
	Segment    string `gorm:"column:segment"` 
	
	Vector string `gorm:"column:vector"`
}

func (pgVectorRow) TableName() string { return "vector_items" }

type PGVectorStore struct {
	db  *gorm.DB
	dim int
}

func NewPGVectorStore(db *gorm.DB, dim int) (*PGVectorStore, error) {
	if dim <= 0 {
		dim = 256
	}
	if db == nil {
		return nil, errors.New("llmgw: nil db for pgvector")
	}
	if db.Dialector.Name() != "postgres" {
		return nil, fmt.Errorf("llmgw: pgvector requires postgres, got %q", db.Dialector.Name())
	}
	if err := db.Exec("CREATE EXTENSION IF NOT EXISTS vector").Error; err != nil {
		return nil, fmt.Errorf("llmgw: create pgvector extension: %w", err)
	}
	
	if dim < 1 || dim > 65536 {
		return nil, fmt.Errorf("llmgw: invalid vector dimension %d (expected 1..65536)", dim)
	}
	if err := db.Exec(fmt.Sprintf(
		"CREATE TABLE IF NOT EXISTS vector_items (id text PRIMARY KEY, collection text, document_id text, title text, segment text, vector vector(%s))",
		strconv.Itoa(dim))).Error; err != nil {
		return nil, fmt.Errorf("llmgw: create vector_items: %w", err)
	}
	return &PGVectorStore{db: db, dim: dim}, nil
}

func (s *PGVectorStore) Upsert(ctx context.Context, collection string, items []ports.VectorItem) error {
	if len(items) == 0 {
		return nil
	}
	
	rows := make([]pgVectorRow, 0, len(items))
	for _, it := range items {
		seg, err := json.Marshal(it.Segment)
		if err != nil {
			return fmt.Errorf("llmgw: marshal segment: %w", err)
		}
		rows = append(rows, pgVectorRow{
			ID:         it.ID,
			Collection: collection,
			DocumentID: it.DocumentID,
			Title:      it.Title,
			Segment:    string(seg),
			Vector:     vecToPG(it.Vector),
		})
	}
	err := s.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "id"}},
			DoUpdates: clause.AssignmentColumns([]string{"collection", "document_id", "title", "segment", "vector"}),
		}).
		CreateInBatches(rows, 200).Error
	if err != nil {
		return fmt.Errorf("llmgw: batch upsert vector rows: %w", err)
	}
	return nil
}

func (s *PGVectorStore) Search(ctx context.Context, collection string, vector []float32, topK int) ([]ports.VectorHit, error) {
	if topK <= 0 {
		topK = 5
	}
	var rows []pgVectorRow
	
	if err := s.db.WithContext(ctx).
		Where("collection = ?", collection).
		Order(fmt.Sprintf("vector <=> '%s'::vector", vecToPG(vector))).
		Limit(topK).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	return s.rowsToHits(rows, vector), nil
}

func (s *PGVectorStore) Delete(ctx context.Context, collection, documentID string) error {
	return s.db.WithContext(ctx).
		Where("collection = ? AND document_id = ?", collection, documentID).
		Delete(&pgVectorRow{}).Error
}

func (s *PGVectorStore) DropCollection(ctx context.Context, collection string) error {
	return s.db.WithContext(ctx).Where("collection = ?", collection).Delete(&pgVectorRow{}).Error
}

func (s *PGVectorStore) rowsToHits(rows []pgVectorRow, q []float32) []ports.VectorHit {
	hits := make([]ports.VectorHit, 0, len(rows))
	for _, r := range rows {
		it, err := rowToItem(r)
		if err != nil {
			continue
		}
		score := 0.0
		if len(q) > 0 && len(it.Vector) > 0 {
			if sim, err := llm.CosineSimilarity(q, it.Vector); err == nil {
				score = sim
			}
		}
		hits = append(hits, ports.VectorHit{Item: it, Score: score})
	}
	sort.SliceStable(hits, func(i, j int) bool { return hits[i].Score > hits[j].Score })
	return hits
}

func rowToItem(r pgVectorRow) (ports.VectorItem, error) {
	var seg llm.Segment
	if err := json.Unmarshal([]byte(r.Segment), &seg); err != nil {
		return ports.VectorItem{}, err
	}
	vec, err := pgToVec(r.Vector)
	if err != nil {
		return ports.VectorItem{}, err
	}
	return ports.VectorItem{
		ID:         r.ID,
		DocumentID: r.DocumentID,
		Title:      r.Title,
		Segment:    seg,
		Vector:     vec,
	}, nil
}

func vecToPG(v []float32) string {
	if len(v) == 0 {
		return "[]"
	}
	b := make([]byte, 0, len(v)*8)
	b = append(b, '[')
	for i, x := range v {
		if i > 0 {
			b = append(b, ',')
		}
		b = append(b, fmt.Sprintf("%g", x)...)
	}
	b = append(b, ']')
	return string(b)
}

func pgToVec(s string) ([]float32, error) {
	if len(s) < 2 {
		return nil, errors.New("llmgw: empty vector")
	}
	inner := s[1 : len(s)-1]
	if inner == "" {
		return nil, nil
	}
	var out []float32
	start := 0
	for i := 0; i < len(inner); i++ {
		if inner[i] == ',' {
			var f float32
			if _, err := fmt.Sscanf(inner[start:i], "%g", &f); err != nil {
				return nil, err
			}
			out = append(out, f)
			start = i + 1
		}
	}
	if start < len(inner) {
		var f float32
		if _, err := fmt.Sscanf(inner[start:], "%g", &f); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, nil
}

func cosineDist(a, b []float32) float32 {
	if len(a) == 0 || len(b) == 0 {
		return 1
	}
	var dot, na, nb float64
	for i := 0; i < len(a) && i < len(b); i++ {
		dot += float64(a[i] * b[i])
		na += float64(a[i] * a[i])
		nb += float64(b[i] * b[i])
	}
	if na == 0 || nb == 0 {
		return 1
	}
	sim := dot / (math.Sqrt(na) * math.Sqrt(nb))
	return float32(1 - sim)
}