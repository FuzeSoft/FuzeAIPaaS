package artifact

import (
	"fmt"
	"strings"

	"fuze-ai-paas/backend/internal/ports"
)

const (
	BackendLocal = "local"
	BackendS3    = "s3"
)

const defaultLocalRoot = "./data/artifacts"

type Config struct {
	Backend   string
	LocalRoot string
	Endpoint  string
	AccessKey string
	SecretKey string
	Bucket    string
	Region    string
	UseSSL    bool
}

func ConfigFromEnv(getenv func(string) string) Config {
	backend := strings.ToLower(strings.TrimSpace(getenv("ARTIFACT_BACKEND")))
	if backend == "" {
		backend = BackendLocal
	}
	root := strings.TrimSpace(getenv("ARTIFACT_LOCAL_ROOT"))
	if root == "" {
		root = defaultLocalRoot
	}
	return Config{
		Backend:   backend,
		LocalRoot: root,
		Endpoint:  strings.TrimSpace(getenv("ARTIFACT_S3_ENDPOINT")),
		AccessKey: strings.TrimSpace(getenv("ARTIFACT_S3_ACCESS_KEY")),
		SecretKey: strings.TrimSpace(getenv("ARTIFACT_S3_SECRET_KEY")),
		Bucket:    strings.TrimSpace(getenv("ARTIFACT_S3_BUCKET")),
		Region:    strings.TrimSpace(getenv("ARTIFACT_S3_REGION")),
		UseSSL:    strings.EqualFold(strings.TrimSpace(getenv("ARTIFACT_S3_USE_SSL")), "true"),
	}
}

func New(cfg Config) (ports.ArtifactStore, error) {
	switch cfg.Backend {
	case BackendLocal, "":
		root := cfg.LocalRoot
		if root == "" {
			root = defaultLocalRoot
		}
		return NewLocalStore(root)
	case BackendS3:
		return NewS3Store(cfg)
	default:
		return nil, fmt.Errorf("artifact: unsupported backend %q (want %q or %q)", cfg.Backend, BackendLocal, BackendS3)
	}
}

func NewFromEnv(getenv func(string) string) (ports.ArtifactStore, error) {
	return New(ConfigFromEnv(getenv))
}