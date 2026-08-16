package data

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"

	datadomain "fuze-ai-paas/backend/internal/domain/data"
	"fuze-ai-paas/backend/internal/models"
)

func (s *Service) CreateAnnotation(a *models.AnnotationTask) error {
	if a.Status == "" {
		a.Status = models.AnnotationStatusOpen
	}
	return s.dataRepo.CreateAnnotation(a)
}

func (s *Service) GetAnnotation(id string) (*models.AnnotationTask, error) {
	return s.dataRepo.GetAnnotation(id)
}

func (s *Service) ListAnnotations(tenantID string) ([]models.AnnotationTask, error) {
	return s.dataRepo.ListAnnotations(tenantID)
}

func (s *Service) UpdateAnnotationProgress(id string, progress int, status models.AnnotationStatus) error {
	a, err := s.dataRepo.GetAnnotation(id)
	if err != nil {
		return err
	}
	a.Progress = progress
	if status != "" {
		a.Status = status
	}
	return s.dataRepo.UpdateAnnotation(a)
}

func (s *Service) ExportAnnotation(ctx context.Context, id, srcFormat, inputPath, outputPath string) error {
	a, err := s.dataRepo.GetAnnotation(id)
	if err != nil {
		return err
	}
	if a.Status == models.AnnotationStatusExported {
		return fmt.Errorf("annotation %s already exported", id)
	}
	if err := s.validateExportPath(inputPath); err != nil {
		return fmt.Errorf("invalid input_path: %w", err)
	}
	if err := s.validateExportPath(outputPath); err != nil {
		return fmt.Errorf("invalid output_path: %w", err)
	}
	raw, err := os.ReadFile(inputPath)
	if err != nil {
		return fmt.Errorf("read annotation source %s: %w", inputPath, err)
	}
	records, err := datadomain.ParseAnnotations(srcFormat, raw)
	if err != nil {
		return err
	}
	out, err := datadomain.ConvertAnnotations(records, a.OutputFormat)
	if err != nil {
		return err
	}
	
	if dir := filepath.Dir(outputPath); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	if err := os.WriteFile(outputPath, out, 0o644); err != nil {
		return fmt.Errorf("write annotation export %s: %w", outputPath, err)
	}
	
	uri := outputPath
	if s.artifact != nil {
		key := fmt.Sprintf("annotations/%s/export.%s", id, a.OutputFormat)
		got, err := s.artifact.Put(ctx, key, bytes.NewReader(out))
		if err != nil {
			return fmt.Errorf("register annotation artifact: %w", err)
		}
		uri = got
	}
	a.ExportedURI = uri
	a.Status = models.AnnotationStatusExported
	return s.dataRepo.UpdateAnnotation(a)
}

func (s *Service) validateExportPath(p string) error {
	if p == "" {
		return fmt.Errorf("path must not be empty")
	}
	root := s.exportRoot
	if root == "" {
		
		return nil
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("resolve export root: %w", err)
	}
	absPath, err := filepath.Abs(p)
	if err != nil {
		return fmt.Errorf("resolve path: %w", err)
	}
	rel, err := filepath.Rel(absRoot, absPath)
	if err != nil {
		return fmt.Errorf("path %q is outside export root %q", p, root)
	}
	if rel == ".." || len(rel) >= 2 && rel[:2] == ".."+string(os.PathSeparator) {
		return fmt.Errorf("path %q escapes export root %q", p, root)
	}
	return nil
}