package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tekmanic/wpresto/internal/config"
	"github.com/tekmanic/wpresto/internal/plugin"
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

// TestRunBuildDynamicReport validates the plugin-system wiring in the build:
// dynamic parts are detected, matched to a registered plugin (form action
// rewritten to the plugin route), unmatched parts are flagged, and the report
// is published to live.
func TestRunBuildDynamicReport(t *testing.T) {
	// Register a test plugin that handles contact forms (the cmd test binary
	// does not link the inventory, so we register one here).
	plugin.Register(plugin.FuncPlugin{
		NameStr: "test-contact",
		Desc:    "test contact-form handler",
		KindsL:  []string{"contact-form"},
		RoutesL: []plugin.Route{{Pattern: "/wpresto/contact", Methods: []string{"POST"}}},
		Handler: func(ctx context.Context, req *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Body: http.NoBody}, nil
		},
	})

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch r.URL.Path {
		case "/":
			_, _ = w.Write([]byte(`<html><body>
<a href="/contact/">Contact</a>
<a href="/blog/">Blog</a>
</body></html>`))
		case "/contact/":
			_, _ = w.Write([]byte(`<html><body>
<form class="wpcf7" action="/wp-admin/admin-post.php" method="post">
<input name="your-name"/><input name="your-email" type="email"/>
<textarea name="your-message"></textarea><button>Send</button>
</form>
</body></html>`))
		case "/blog/":
			_, _ = w.Write([]byte(`<html><body>
<a href="/">Home</a>
<form id="commentform" action="/wp-comments-post.php" method="post">
<textarea name="comment"></textarea>
</form>
</body></html>`))
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
	cfg.Plugins.Enabled = true

	if err := RunBuild(context.Background(), cfg); err != nil {
		t.Fatalf("RunBuild: %v", err)
	}

	live := filepath.Join(outDir, "live")

	// The contact form's action is rewritten to the plugin route.
	contact := readFile(t, filepath.Join(live, "contact", "index.html"))
	if !strings.Contains(contact, "/wpresto/contact") {
		t.Errorf("contact form action should be rewritten to /wpresto/contact, got: %s", contact)
	}
	if strings.Contains(contact, "admin-post") {
		t.Errorf("contact form should no longer post to admin-post, got: %s", contact)
	}

	// The comment form (no plugin) is stripped.
	blog := readFile(t, filepath.Join(live, "blog", "index.html"))
	if strings.Contains(blog, "commentform") {
		t.Errorf("comment form should be stripped (no plugin), got: %s", blog)
	}

	// dynamic-report.json is published and reflects the match + flag.
	repRaw := readFile(t, filepath.Join(live, "dynamic-report.json"))
	var rep plugin.Report
	if err := json.Unmarshal([]byte(repRaw), &rep); err != nil {
		t.Fatalf("unmarshal dynamic-report.json: %v", err)
	}
	if rep.Detected < 2 {
		t.Errorf("expected >=2 detected dynamic parts, got %d", rep.Detected)
	}
	if rep.Handled < 1 {
		t.Errorf("expected >=1 handled (contact-form), got %d", rep.Handled)
	}
	if rep.Flagged < 1 {
		t.Errorf("expected >=1 flagged (comment-form), got %d", rep.Flagged)
	}
}
