package crawl

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tekmanic/wpstatic/internal/config"
	"github.com/tekmanic/wpstatic/internal/minify"
	"github.com/tekmanic/wpstatic/internal/strip"
)

func TestPagePath(t *testing.T) {
	cases := []struct {
		u    string
		want string
	}{
		{"http://x/", "index.html"},
		{"http://x", "index.html"},
		{"http://x/about", "about/index.html"},
		{"http://x/about/", "about/index.html"},
		{"http://x/category/news/page/2", "category/news/page/2/index.html"},
		{"http://x/page.html", "page.html"},
		{"http://x/a/b.html", "a/b.html"},
	}
	for _, c := range cases {
		u, _ := url.Parse(c.u)
		if got := pagePath(u); got != c.want {
			t.Errorf("pagePath(%s) = %q, want %q", c.u, got, c.want)
		}
	}
}

func TestAssetPath(t *testing.T) {
	cases := []struct {
		u    string
		want string
	}{
		{"http://x/wp-content/themes/t/style.css", "wp-content/themes/t/style.css"},
		{"http://x/wp-content/uploads/2024/01/a.png", "wp-content/uploads/2024/01/a.png"},
	}
	for _, c := range cases {
		u, _ := url.Parse(c.u)
		if got := assetPath(u); got != c.want {
			t.Errorf("assetPath(%s) = %q, want %q", c.u, got, c.want)
		}
	}
}

func TestResolveSameHostOnly(t *testing.T) {
	base, _ := url.Parse("http://example.com/dir")
	cases := []struct {
		ref  string
		want bool
	}{
		{"/about", true},
		{"about", true},
		{"http://example.com/x", true},
		{"https://other.com/x", false},
		{"javascript:alert(1)", false},
		{"mailto:a@b.c", false},
		{"#frag", false},
		{"data:text/plain,hi", false},
		{"", false},
	}
	for _, c := range cases {
		got := resolve(base, c.ref)
		if (got != nil) != c.want {
			t.Errorf("resolve(%q) nil=%v, want nil=%v", c.ref, got == nil, !c.want)
		}
	}
}

func TestParseSrcset(t *testing.T) {
	base, _ := url.Parse("http://example.com")
	got := parseSrcset(base, "/a.png 480w, /b.png 800w, https://other.com/c.png 1200w")
	if len(got) != 2 {
		t.Fatalf("expected 2 same-host URLs, got %d: %v", len(got), got)
	}
	if got[0].Path != "/a.png" || got[1].Path != "/b.png" {
		t.Errorf("unexpected paths: %v", got)
	}
}

func TestExtractCSSRefs(t *testing.T) {
	cssURL, _ := url.Parse("http://example.com/wp-content/themes/t/style.css")
	css := []byte(`body{background:url(../images/bg.png)} @font-face{src:url('fonts/f.woff2')}`)
	got := extractCSSRefs(css, cssURL)
	if len(got) != 2 {
		t.Fatalf("expected 2 refs, got %d: %v", len(got), got)
	}
	// ../images/bg.png resolves against the CSS dir /wp-content/themes/t/
	if got[0].Path != "/wp-content/themes/images/bg.png" {
		t.Errorf("unexpected first ref: %s", got[0])
	}
	if got[1].Path != "/wp-content/themes/t/fonts/f.woff2" {
		t.Errorf("unexpected second ref: %s", got[1])
	}
}

func TestCrawlEndToEnd(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write([]byte(`<html><head><title>Home</title>
<meta name="generator" content="WordPress 6.5">
<link rel="stylesheet" href="/style.css">
</head><body>
<a href="/about">About</a>
<a href="/wp-admin/">Admin</a>
<img src="/logo.png">
</body></html>`))
		case "/about":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write([]byte(`<html><body><h1>About</h1><a href="/">Home</a></body></html>`))
		case "/style.css":
			w.Header().Set("Content-Type", "text/css")
			w.Write([]byte(`body{color:red;background:url(/bg.png)}`))
		case "/logo.png", "/bg.png":
			w.Header().Set("Content-Type", "image/png")
			w.Write([]byte{0x89, 'P', 'N', 'G'})
		default:
			http.NotFound(w, r)
		}
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	base, _ := url.Parse(srv.URL)
	dir := t.TempDir()
	staging := filepath.Join(dir, "staging")
	if err := os.MkdirAll(staging, 0o755); err != nil {
		t.Fatal(err)
	}

	ccfg := config.CrawlConfig{Concurrency: 4, Timeout: 5e9, MaxPages: 100, MaxAssetBytes: 1 << 20}
	sopt := strip.Options{
		Blocklist:          []string{"/wp-admin/", "/wp-login.php"},
		StripForms:         "wp-endpoints",
		StripGeneratorMeta: true,
		QueryParams:        []string{"utm_source"},
	}
	mopt := minify.Options{Enabled: true, HTML: true, CSS: true, JS: true}

	cr := New(base, "test", ccfg, sopt, mopt, true, staging, srv.Client())
	res, err := cr.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if res.Pages < 2 {
		t.Errorf("expected >=2 pages, got %d", res.Pages)
	}
	if res.Assets < 3 {
		t.Errorf("expected >=3 assets (css, logo, bg), got %d", res.Assets)
	}

	// Home page written and stripped.
	home := read(t, filepath.Join(staging, "index.html"))
	if strings.Contains(home, "wp-admin") {
		t.Errorf("home should not contain wp-admin link: %s", home)
	}
	if strings.Contains(home, "generator") {
		t.Errorf("home should not contain generator meta: %s", home)
	}
	if !strings.Contains(home, "About") {
		t.Errorf("home should contain About link: %s", home)
	}

	// About page written.
	about := read(t, filepath.Join(staging, "about", "index.html"))
	if !strings.Contains(about, "About") {
		t.Errorf("about page missing: %s", about)
	}

	// CSS written and minified (no spaces after {).
	cssOut := read(t, filepath.Join(staging, "style.css"))
	if strings.Contains(cssOut, "color: red") {
		t.Errorf("css should be minified: %s", cssOut)
	}

	// Gzip siblings exist.
	if _, err := os.Stat(filepath.Join(staging, "index.html.gz")); err != nil {
		t.Errorf("expected index.html.gz: %v", err)
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}
