package artifact

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"fuze-ai-paas/backend/internal/ports"
)

type LocalStore struct {
	root string
}

func NewLocalStore(root string) (*LocalStore, error) {
	r := strings.TrimSpace(root)
	if r == "" {
		return nil, fmt.Errorf("artifact: local store root must not be empty")
	}
	abs, err := filepath.Abs(r)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return nil, err
	}
	return &LocalStore{root: abs}, nil
}

func (s *LocalStore) Put(_ context.Context, key string, r io.Reader) (string, error) {
	k, err := normalizeKey(key)
	if err != nil {
		return "", err
	}
	if r == nil {
		return "", fmt.Errorf("artifact: nil reader for key %q", key)
	}

	full := filepath.Join(s.root, filepath.FromSlash(k))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return "", err
	}

	tmp, err := os.CreateTemp(filepath.Dir(full), ".partial-*")
	if err != nil {
		return "", err
	}
	tmpName := tmp.Name()
	if _, err := io.Copy(tmp, r); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return "", err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return "", err
	}
	if err := os.Rename(tmpName, full); err != nil {
		os.Remove(tmpName)
		return "", err
	}
	return schemeFile + filepath.ToSlash(full), nil
}

func (s *LocalStore) Get(_ context.Context, uri string) (io.ReadCloser, error) {
	full, err := s.pathOf(uri)
	if err != nil {
		return nil, err
	}
	return os.Open(full)
}

func (s *LocalStore) List(_ context.Context, prefix string) ([]ports.ArtifactInfo, error) {
	base := s.root
	if p := strings.Trim(strings.TrimSpace(prefix), "/"); p != "" {
		k, err := normalizeKey(p)
		if err != nil {
			return nil, err
		}
		base = filepath.Join(s.root, filepath.FromSlash(k))
	}

	out := []ports.ArtifactInfo{}
	err := filepath.WalkDir(base, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(s.root, path)
		if err != nil {
			return err
		}
		out = append(out, ports.ArtifactInfo{
			Key:          filepath.ToSlash(rel),
			URI:          schemeFile + filepath.ToSlash(path),
			Size:         info.Size(),
			LastModified: info.ModTime(),
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (s *LocalStore) Delete(_ context.Context, uri string) error {
	full, err := s.pathOf(uri)
	if err != nil {
		return err
	}
	if err := os.Remove(full); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (s *LocalStore) Presign(_ context.Context, uri string, _ time.Duration) (string, error) {
	if _, err := s.pathOf(uri); err != nil {
		return "", err
	}
	return uri, nil
}

func (s *LocalStore) pathOf(uri string) (string, error) {
	u := strings.TrimSpace(uri)
	if !strings.HasPrefix(u, schemeFile) {
		return "", fmt.Errorf("artifact: %q is not a file:// URI", uri)
	}
	p := filepath.Clean(filepath.FromSlash(strings.TrimPrefix(u, schemeFile)))
	if !strings.HasPrefix(p, s.root+string(os.PathSeparator)) && p != s.root {
		return "", fmt.Errorf("artifact: %q escapes the store root", uri)
	}
	return p, nil
}

var _ ports.ArtifactStore = (*LocalStore)(nil)