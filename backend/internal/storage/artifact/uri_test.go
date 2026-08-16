package artifact

import "testing"

func TestNormalizeKey(t *testing.T) {
	ok := map[string]string{
		"a":              "a",
		"a/b/c.bin":      "a/b/c.bin",
		" runs/r1/x.pt ": "runs/r1/x.pt",
		"./a/b":          "a/b",
	}
	for in, want := range ok {
		got, err := normalizeKey(in)
		if err != nil {
			t.Fatalf("normalizeKey(%q) unexpected error: %v", in, err)
		}
		if got != want {
			t.Fatalf("normalizeKey(%q) = %q, want %q", in, got, want)
		}
	}

	bad := []string{"", "   ", "/abs", "../up", "a/../../up", "a//b", "a/./../../b"}
	for _, in := range bad {
		if got, err := normalizeKey(in); err == nil {
			t.Fatalf("normalizeKey(%q) must fail, got %q", in, got)
		}
	}
}

func TestParseS3URI(t *testing.T) {
	bucket, key, err := parseS3URI("s3://my-bucket/runs/r1/model.pt")
	if err != nil {
		t.Fatalf("parseS3URI: %v", err)
	}
	if bucket != "my-bucket" || key != "runs/r1/model.pt" {
		t.Fatalf("got bucket=%q key=%q", bucket, key)
	}

	for _, in := range []string{"", "s3://", "s3://bucket", "s3://bucket/", "file:///tmp/x", "http://x/y", "://bad"} {
		if _, _, err := parseS3URI(in); err == nil {
			t.Fatalf("parseS3URI(%q) must fail", in)
		}
	}
}

func TestS3URI(t *testing.T) {
	if got := s3URI("b", "k/v"); got != "s3://b/k/v" {
		t.Fatalf("s3URI = %q", got)
	}
}