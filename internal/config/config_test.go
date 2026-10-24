package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultValid(t *testing.T) {
	c := Default()
	if err := c.Validate(); err != nil {
		t.Fatalf("default config should be valid: %v", err)
	}
	if c.Site.URL == "" || c.Output.Dir == "" {
		t.Errorf("default should have site.url and output.dir")
	}
	if len(c.Strip.Blocklist) == 0 {
		t.Errorf("default should have a blocklist")
	}
}

func TestValidateRejectsBadSiteURL(t *testing.T) {
	c := Default()
	c.Site.URL = "ftp://example.com"
	if err := c.Validate(); err == nil {
		t.Errorf("expected error for non-http URL")
	}
}

func TestValidateRejectsHookWithoutSecret(t *testing.T) {
	c := Default()
	c.Hook.Enabled = true
	c.Hook.Secret = ""
	if err := c.Validate(); err == nil {
		t.Errorf("expected error for hook enabled without secret")
	}
}

func TestValidateRejectsBadStripForms(t *testing.T) {
	c := Default()
	c.Strip.StripForms = "sometimes"
	if err := c.Validate(); err == nil {
		t.Errorf("expected error for invalid strip_forms")
	}
}

func TestLoadFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wpresto.yaml")
	content := `
site:
  url: http://127.0.0.1:9999
output:
  dir: /tmp/out
crawl:
  concurrency: 2
  max_pages: 10
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.Site.URL != "http://127.0.0.1:9999" {
		t.Errorf("expected site.url from file, got %q", c.Site.URL)
	}
	if c.Output.Dir != "/tmp/out" {
		t.Errorf("expected output.dir from file, got %q", c.Output.Dir)
	}
	if c.Crawl.Concurrency != 2 {
		t.Errorf("expected concurrency 2, got %d", c.Crawl.Concurrency)
	}
	// Defaults still applied for unset keys.
	if c.Crawl.Timeout <= 0 {
		t.Errorf("expected default timeout to be applied")
	}
}

func TestLoadEnvOverride(t *testing.T) {
	t.Setenv("WPRESTO_SITE_URL", "http://env.example")
	c, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if c.Site.URL != "http://env.example" {
		t.Errorf("expected env override, got %q", c.Site.URL)
	}
}

func TestLoadMissingFileErrors(t *testing.T) {
	if _, err := Load("/nonexistent/path/wpresto.yaml"); err == nil {
		t.Errorf("expected error for missing config file")
	}
}

func TestTimeoutParsing(t *testing.T) {
	c := Default()
	if c.Crawl.Timeout != 30*time.Second {
		t.Errorf("expected 30s default timeout, got %v", c.Crawl.Timeout)
	}
}
