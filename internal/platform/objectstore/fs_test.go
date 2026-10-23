package objectstore

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
)

func TestValidKey(t *testing.T) {
	good := []string{"briefs/0199a1b2-0000-7000-8000-000000000001/0199a1b2-0000-7000-8000-000000000002.pdf", "a", "a/b-c/d.e"}
	bad := []string{"", "/abs", "a//b", "a/../b", "..", "a/..", "a/.hidden", "A/b", "a\\b", "c:/x", "a b", "a/b/", "%2e%2e"}
	for _, k := range good {
		if err := ValidKey(k); err != nil {
			t.Errorf("ValidKey(%q) = %v, want nil", k, err)
		}
	}
	for _, k := range bad {
		if err := ValidKey(k); err == nil {
			t.Errorf("ValidKey(%q) = nil, want error", k)
		}
	}
}

func TestFSStoreRoundTrip(t *testing.T) {
	s, err := OpenFS(t.TempDir(), "local")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := s.Check(ctx, 0); err != nil {
		t.Fatal(err)
	}
	body := []byte("%PDF-1.7 test")
	if err := s.Put(ctx, "briefs/f/x.pdf", bytes.NewReader(body), int64(len(body)), "application/pdf"); err != nil {
		t.Fatal(err)
	}
	r, n, err := s.Get(ctx, "briefs/f/x.pdf")
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(r)
	_ = r.Close()
	if n != int64(len(body)) || !bytes.Equal(got, body) {
		t.Fatalf("round trip: got %q (%d)", got, n)
	}
	if _, _, err := s.Get(ctx, "briefs/f/missing.pdf"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing object: %v", err)
	}
	if err := s.Put(ctx, "../escape", bytes.NewReader(body), int64(len(body)), ""); !errors.Is(err, ErrInvalidKey) {
		t.Fatalf("traversal key accepted: %v", err)
	}
	if err := s.Put(ctx, "briefs/f/short.pdf", bytes.NewReader(body), int64(len(body))+5, ""); err == nil {
		t.Fatal("short body accepted")
	}
	if _, _, err := s.Get(ctx, "briefs/f/short.pdf"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("short body left an object: %v", err)
	}
	if _, err := OpenFS(t.TempDir(), "../up"); err == nil {
		t.Fatal("traversal bucket accepted")
	}
}
