package cmd

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tekmanic/wpstatic/internal/config"
)

// TestRunBuildEndToEnd validates the full CLI orchestrator: config -> crawl ->
// strip -> minify -> publish, against an in-process fake WordPress site.
func TestRunBuildEndToEnd(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<html><head><title>Home</title>
<meta name="generator" content="WordPress 6.5">
<link rel="stylesheet" href="/style.css"></head><body>
<a href="/about/">About</a>
<a href="/wp-admin/">Admin</a>
<a href="/wp-login.php">Login</a>
<img src="/logo.png">
</body></html>`))
		case "/about/":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<html><body><h1>About</h1><a href="/">Home</a></body></html>`))
		case "/style.css":
			w.Header().Set("Content-Type", "text/css")
			_, _ = w.Write([]byte(`body { color: red; margin: 0; }`))
		case "/logo.png":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write([]byte{0x89, 'P', 'N', 'G'})
		default:
			http.NotFound(w, r)
		}
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	outDir := t.TempDir()
	cfg := config.Default()
	cfg.Site.URL = srv.URL
	cfg.Output.Dir = outDir
	cfg.Crawl.Timeout = 10 * time.Second
	cfg.Crawl.MaxPages = 50

	if err := RunBuild(context.Background(), cfg); err != nil {
		t.Fatalf("RunBuild: %v", err)
	}

	live := filepath.Join(outDir, "live")

	// Home page published and stripped.
	home := readFile(t, filepath.Join(live, "index.html"))
	if strings.Contains(home, "wp-admin") || strings.Contains(home, "wp-login") {
		t.Errorf("home should not contain admin/login links: %s", home)
	}
	if strings.Contains(home, "generator") {
		t.Errorf("home should not contain generator meta: %s", home)
	}
	if !strings.Contains(home, "About") {
		t.Errorf("home should contain About link: %s", home)
	}

	// About page published.
	if _, err := os.Stat(filepath.Join(live, "about", "index.html")); err != nil {
		t.Errorf("expected about/index.html: %v", err)
	}

	// CSS published and minified.
	cssOut := readFile(t, filepath.Join(live, "style.css"))
	if strings.Contains(cssOut, "color: red") {
		t.Errorf("css should be minified: %s", cssOut)
	}

	// Gzip sibling present.
	if _, err := os.Stat(filepath.Join(live, "index.html.gz")); err != nil {
		t.Errorf("expected index.html.gz: %v", err)
	}

	// build-info.json present.
	if _, err := os.Stat(filepath.Join(live, "build-info.json")); err != nil {
		t.Errorf("expected build-info.json: %v", err)
	}

	// No leftover staging dirs.
	entries, _ := os.ReadDir(outDir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".staging-") {
			t.Errorf("leftover staging dir: %s", e.Name())
		}
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}
