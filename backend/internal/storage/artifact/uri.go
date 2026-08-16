package artifact

import (
	"fmt"
	"strings"
)

const (
	schemeFile = "file://"
	schemeS3   = "s3://"
)

func normalizeKey(key string) (string, error) {
	k := strings.TrimSpace(key)
	if k == "" {
		return "", fmt.Errorf("artifact: key must not be empty")
	}
	if strings.HasPrefix(k, "/") {
		return "", fmt.Errorf("artifact: key %q must be relative", key)
	}
	if strings.Contains(k, "\\") {
		return "", fmt.Errorf("artifact: key %q must not contain backslashes", key)
	}

	segs := strings.Split(k, "/")
	out := make([]string, 0, len(segs))
	for _, s := range segs {
		switch s {
		case "":
			return "", fmt.Errorf("artifact: key %q contains an empty path segment", key)
		case ".":
			
			continue
		case "..":
			return "", fmt.Errorf("artifact: key %q must not traverse upwards", key)
		default:
			out = append(out, s)
		}
	}
	if len(out) == 0 {
		return "", fmt.Errorf("artifact: key %q resolves to nothing", key)
	}
	return strings.Join(out, "/"), nil
}

func parseS3URI(uri string) (bucket, key string, err error) {
	u := strings.TrimSpace(uri)
	if !strings.HasPrefix(u, schemeS3) {
		return "", "", fmt.Errorf("artifact: %q is not an s3:// URI", uri)
	}
	rest := strings.TrimPrefix(u, schemeS3)
	idx := strings.Index(rest, "/")
	if idx <= 0 {
		return "", "", fmt.Errorf("artifact: s3 URI %q lacks bucket or key", uri)
	}
	bucket = rest[:idx]
	key = rest[idx+1:]
	if bucket == "" || key == "" {
		return "", "", fmt.Errorf("artifact: s3 URI %q lacks bucket or key", uri)
	}
	return bucket, key, nil
}

func s3URI(bucket, key string) string {
	return schemeS3 + bucket + "/" + key
}