package artifact

import (
	"context"
	"io"
	"path/filepath"
	"strings"
	"testing"
)

func newLocal(t *testing.T) *LocalStore {
	t.Helper()
	s, err := NewLocalStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewLocalStore: %v", err)
	}
	return s
}

func TestLocalPutGetRoundTrip(t *testing.T) {
	s := newLocal(t)
	ctx := context.Background()

	uri, err := s.Put(ctx, "runs/r1/ckpt.bin", strings.NewReader("hello-checkpoint"))
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if !strings.HasPrefix(uri, "file://") {
		t.Fatalf("local URI must carry file:// scheme, got %q", uri)
	}

	rc, err := s.Get(ctx, uri)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer rc.Close()
	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(got) != "hello-checkpoint" {
		t.Fatalf("content mismatch: %q", got)
	}
}

func TestLocalPutCreatesNestedDirs(t *testing.T) {
	s := newLocal(t)
	uri, err := s.Put(context.Background(), "a/b/c/d/model.pt", strings.NewReader("x"))
	if err != nil {
		t.Fatalf("Put nested: %v", err)
	}
	if filepath.Base(strings.TrimPrefix(uri, "file://")) != "model.pt" {
		t.Fatalf("unexpected uri %q", uri)
	}
}

func TestLocalPutOverwritesSameKey(t *testing.T) {
	s := newLocal(t)
	ctx := context.Background()
	if _, err := s.Put(ctx, "k", strings.NewReader("v1")); err != nil {
		t.Fatalf("first put: %v", err)
	}
	uri, err := s.Put(ctx, "k", strings.NewReader("v2"))
	if err != nil {
		t.Fatalf("second put: %v", err)
	}
	rc, err := s.Get(ctx, uri)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer rc.Close()
	got, _ := io.ReadAll(rc)
	if string(got) != "v2" {
		t.Fatalf("expected overwrite to v2, got %q", got)
	}
}

func TestLocalListByPrefix(t *testing.T) {
	s := newLocal(t)
	ctx := context.Background()
	for _, k := range []string{"runs/r1/a.bin", "runs/r1/b.bin", "runs/r2/c.bin"} {
		if _, err := s.Put(ctx, k, strings.NewReader("data")); err != nil {
			t.Fatalf("put %s: %v", k, err)
		}
	}

	got, err := s.List(ctx, "runs/r1")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 artifacts under runs/r1, got %d: %+v", len(got), got)
	}
	for _, a := range got {
		if !strings.HasPrefix(a.Key, "runs/r1/") {
			t.Fatalf("listed key escaped prefix: %q", a.Key)
		}
		if a.Size != int64(len("data")) {
			t.Fatalf("expected size 4, got %d", a.Size)
		}
		if a.URI == "" || a.LastModified.IsZero() {
			t.Fatalf("incomplete artifact info: %+v", a)
		}
	}
}

func TestLocalListEmptyPrefixReturnsAll(t *testing.T) {
	s := newLocal(t)
	ctx := context.Background()
	for _, k := range []string{"x/1", "y/2"} {
		if _, err := s.Put(ctx, k, strings.NewReader("d")); err != nil {
			t.Fatalf("put: %v", err)
		}
	}
	got, err := s.List(ctx, "")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected all 2 artifacts, got %d", len(got))
	}
}

func TestLocalListMissingPrefixIsEmptyNotError(t *testing.T) {
	s := newLocal(t)
	got, err := s.List(context.Background(), "nope/nothing")
	if err != nil {
		t.Fatalf("listing an absent prefix is not an error, got %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty result, got %d", len(got))
	}
}

func TestLocalDelete(t *testing.T) {
	s := newLocal(t)
	ctx := context.Background()
	uri, err := s.Put(ctx, "tmp/gone.bin", strings.NewReader("bye"))
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if err := s.Delete(ctx, uri); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := s.Get(ctx, uri); err == nil {
		t.Fatal("expected Get to fail after Delete")
	}
}

func TestLocalRejectsTraversalAndInvalidKeys(t *testing.T) {
	s := newLocal(t)
	ctx := context.Background()
	for _, k := range []string{"", "   ", "../escape", "a/../../escape", "/abs/path", "a//b"} {
		if _, err := s.Put(ctx, k, strings.NewReader("x")); err == nil {
			t.Fatalf("key %q must be rejected", k)
		}
	}
}

func TestLocalGetRejectsURIOutsideRoot(t *testing.T) {
	s := newLocal(t)
	if _, err := s.Get(context.Background(), "file:///etc/passwd"); err == nil {
		t.Fatal("must refuse to read outside the store root")
	}
}

func TestLocalGetRejectsForeignScheme(t *testing.T) {
	s := newLocal(t)
	if _, err := s.Get(context.Background(), "s3://bucket/key"); err == nil {
		t.Fatal("local store must reject s3:// URIs")
	}
}

func TestLocalPresignReturnsUsableURI(t *testing.T) {
	s := newLocal(t)
	ctx := context.Background()
	uri, err := s.Put(ctx, "p/q.bin", strings.NewReader("z"))
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	got, err := s.Presign(ctx, uri, 0)
	if err != nil {
		t.Fatalf("Presign: %v", err)
	}
	if got != uri {
		t.Fatalf("expected presign to echo the URI, got %q", got)
	}
	if _, err := s.Presign(ctx, "file:///etc/passwd", 0); err == nil {
		t.Fatal("presign must apply the same containment check as Get")
	}
}

func TestNewLocalStoreRejectsEmptyRoot(t *testing.T) {
	if _, err := NewLocalStore(""); err == nil {
		t.Fatal("empty root must be rejected")
	}
}