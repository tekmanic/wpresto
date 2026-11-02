package plugin

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/tekmanic/wpresto/internal/detect"
)

// testPlugin is a minimal Plugin for tests.
type testPlugin struct {
	name    string
	kinds   []string
	routes  []Route
	handler Handler
}

func (p testPlugin) Name() string        { return p.name }
func (p testPlugin) Description() string { return "test" }
func (p testPlugin) Kinds() []string     { return p.kinds }
func (p testPlugin) Routes() []Route     { return p.routes }
func (p testPlugin) Handle(ctx context.Context, req *http.Request) (*http.Response, error) {
	return p.handler(ctx, req)
}

func okHandler(body string) Handler {
	return func(ctx context.Context, req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: 200,
			Header:     http.Header{"Content-Type": []string{"text/plain"}},
			Body:       io.NopCloser(strings.NewReader(body)),
		}, nil
	}
}

func TestRegisterGetAll(t *testing.T) {
	Register(testPlugin{name: "t-reg", kinds: []string{"k1"}, routes: []Route{{Pattern: "/t-reg"}}})
	if _, ok := Get("t-reg"); !ok {
		t.Fatal("expected t-reg to be registered")
	}
	found := false
	for _, p := range All() {
		if p.Name() == "t-reg" {
			found = true
		}
	}
	if !found {
		t.Fatal("t-reg not in All()")
	}
}

func TestFindKind(t *testing.T) {
	Register(testPlugin{name: "t-kind", kinds: []string{"contact-form"}, routes: nil})
	p, ok := FindKind("contact-form")
	if !ok || p.Name() != "t-kind" {
		t.Fatalf("FindKind(contact-form) = %v, %v; want t-kind, true", p, ok)
	}
	if _, ok := FindKind("does-not-exist"); ok {
		t.Fatal("FindKind(does-not-exist) should be false")
	}
}

func TestFindRoute(t *testing.T) {
	Register(testPlugin{name: "t-route", kinds: nil,
		routes: []Route{{Pattern: "/api/x", Methods: []string{"POST"}}}})
	if _, ok := Find("/api/x", "POST"); !ok {
		t.Fatal("Find(/api/x, POST) should match")
	}
	if _, ok := Find("/api/x", "GET"); ok {
		t.Fatal("Find(/api/x, GET) should not match (method)")
	}
	if _, ok := Find("/other", "POST"); ok {
		t.Fatal("Find(/other, POST) should not match")
	}
}

func TestPathMatches(t *testing.T) {
	cases := []struct {
		pattern, path string
		want          bool
	}{
		{"/exact", "/exact", true},
		{"/exact", "/exact/extra", false},
		{"/prefix/*", "/prefix/a/b", true},
		{"/prefix/*", "/other", false},
		{"/a", "/a", true},
		{"/a", "/ab", false},
	}
	for _, c := range cases {
		if got := pathMatches(c.pattern, c.path); got != c.want {
			t.Errorf("pathMatches(%q, %q) = %v; want %v", c.pattern, c.path, got, c.want)
		}
	}
}

func TestServerRoutesToPlugin(t *testing.T) {
	Register(testPlugin{name: "t-srv", kinds: nil,
		routes:  []Route{{Pattern: "/hello"}},
		handler: okHandler("from-plugin")})

	fallback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte("from-fallback"))
	}))
	defer fallback.Close()
	fu, _ := url.Parse(fallback.URL)

	srv := NewServer("127.0.0.1:0", fu)

	// Plugin route.
	r1 := httptest.NewRequest("GET", "/hello", nil)
	w1 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w1, r1)
	if w1.Body.String() != "from-plugin" {
		t.Errorf("plugin route body = %q; want from-plugin", w1.Body.String())
	}

	// Unmatched -> fallback.
	r2 := httptest.NewRequest("GET", "/uncached", nil)
	w2 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w2, r2)
	if w2.Body.String() != "from-fallback" {
		t.Errorf("fallback body = %q; want from-fallback", w2.Body.String())
	}
}

func TestServerNoFallback(t *testing.T) {
	srv := NewServer("127.0.0.1:0", nil)
	r := httptest.NewRequest("GET", "/nothing", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, r)
	if w.Code != http.StatusNotFound {
		t.Errorf("no-fallback status = %d; want 404", w.Code)
	}
}

func TestMatchReport(t *testing.T) {
	Register(testPlugin{name: "t-match", kinds: []string{"subscribe"}, routes: nil})
	parts := []detect.DynamicPart{
		{Page: "/p1", Kind: "form", Class: "subscribe", Target: "/wpresto/subscribe"},
		{Page: "/p2", Kind: "form", Class: "comment-form", Target: "/wp-comments-post.php"},
	}
	rep := Match(parts)
	if rep.Detected != 2 || rep.Handled != 1 || rep.Flagged != 1 {
		t.Fatalf("report = %+v; want detected=2 handled=1 flagged=1", rep)
	}
	if rep.Parts[0].Plugin != "t-match" || !rep.Parts[0].Handled {
		t.Errorf("part0 = %+v; want handled by t-match", rep.Parts[0])
	}
	if rep.Parts[1].Handled {
		t.Errorf("part1 should be flagged (no plugin for comment-form)")
	}
}
