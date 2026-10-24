package publish

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPublishPromotesStaging(t *testing.T) {
	root := t.TempDir()
	staging := filepath.Join(root, ".staging-test")
	if err := os.MkdirAll(staging, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staging, "index.html"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	info := &BuildInfo{BuildID: "test", StartedAt: time.Now(), FinishedAt: time.Now(), Duration: time.Second, Pages: 1, Assets: 0, Bytes: 5, SiteURL: "http://x"}
	if err := Publish(root, staging, info); err != nil {
		t.Fatal(err)
	}

	live := filepath.Join(root, "live")
	b, err := os.ReadFile(filepath.Join(live, "index.html"))
	if err != nil {
		t.Fatalf("expected live/index.html: %v", err)
	}
	if string(b) != "hello" {
		t.Errorf("unexpected content: %s", b)
	}
	// build-info.json written into live.
	if _, err := os.Stat(filepath.Join(live, "build-info.json")); err != nil {
		t.Errorf("expected build-info.json in live: %v", err)
	}
	// Staging dir gone.
	if _, err := os.Stat(staging); !os.IsNotExist(err) {
		t.Errorf("expected staging to be renamed away")
	}
}

func TestPublishReplacesPreviousLive(t *testing.T) {
	root := t.TempDir()
	// Seed an existing live tree.
	live := filepath.Join(root, "live")
	if err := os.MkdirAll(live, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(live, "old.html"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	staging := filepath.Join(root, ".staging-2")
	if err := os.MkdirAll(staging, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staging, "new.html"), []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}

	info := &BuildInfo{BuildID: "2", StartedAt: time.Now(), FinishedAt: time.Now(), Duration: time.Second}
	if err := Publish(root, staging, info); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(live, "new.html")); err != nil {
		t.Errorf("expected new.html in live: %v", err)
	}
	if _, err := os.Stat(filepath.Join(live, "old.html")); !os.IsNotExist(err) {
		t.Errorf("expected old.html to be gone from live")
	}
}

func TestTreeBytes(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("12345"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sub", "b.txt"), []byte("67890"), 0o644); err != nil {
		t.Fatal(err)
	}
	total, err := TreeBytes(dir)
	if err != nil {
		t.Fatal(err)
	}
	if total != 10 {
		t.Errorf("expected 10 bytes, got %d", total)
	}
}
