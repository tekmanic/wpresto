package strip

import (
	"net/url"
	"strings"
	"testing"
)

func testBase(t *testing.T) *url.URL {
	t.Helper()
	u, err := url.Parse("http://example.com")
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func testOpts() Options {
	return Options{
		Blocklist:          []string{"/wp-admin/", "/wp-login.php", "/wp-json/", "/xmlrpc.php", "/wp-comments-post.php"},
		StripForms:         "wp-endpoints",
		StripGeneratorMeta: true,
		QueryParams:        []string{"utm_source", "fbclid", "replytocom"},
	}
}

func TestProcessRemovesSensitiveLinks(t *testing.T) {
	base := testBase(t)
	in := `<html><body>
<a href="/about">About</a>
<a href="/wp-admin/">Admin</a>
<a href="/wp-login.php">Log in</a>
<a href="/wp-json/wp/v2/posts">API</a>
<a href="https://external.example/page">External</a>
</body></html>`
	out, rep, err := Process([]byte(in), base, testOpts())
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !strings.Contains(s, "/about") {
		t.Errorf("expected /about link to remain, got: %s", s)
	}
	if strings.Contains(s, "wp-admin") {
		t.Errorf("expected /wp-admin/ link to be removed, got: %s", s)
	}
	if strings.Contains(s, "wp-login") {
		t.Errorf("expected /wp-login.php link to be removed, got: %s", s)
	}
	if strings.Contains(s, "wp-json") {
		t.Errorf("expected /wp-json/ link to be removed, got: %s", s)
	}
	if !strings.Contains(s, "external.example") {
		t.Errorf("expected external link to remain, got: %s", s)
	}
	if rep.RemovedLinks != 3 {
		t.Errorf("expected 3 removed links, got %d", rep.RemovedLinks)
	}
}

func TestProcessRemovesGeneratorMeta(t *testing.T) {
	base := testBase(t)
	in := `<html><head><meta name="generator" content="WordPress 6.5"></head><body>hi</body></html>`
	out, _, err := Process([]byte(in), base, testOpts())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), "generator") {
		t.Errorf("expected generator meta to be removed, got: %s", out)
	}
}

func TestProcessKeepsGeneratorMetaWhenDisabled(t *testing.T) {
	base := testBase(t)
	opts := testOpts()
	opts.StripGeneratorMeta = false
	in := `<html><head><meta name="generator" content="WordPress 6.5"></head><body>hi</body></html>`
	out, _, err := Process([]byte(in), base, opts)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "generator") {
		t.Errorf("expected generator meta to remain, got: %s", out)
	}
}

func TestProcessRemovesPingback(t *testing.T) {
	base := testBase(t)
	in := `<html><head><link rel="pingback" href="http://example.com/xmlrpc.php"></head><body>hi</body></html>`
	out, _, err := Process([]byte(in), base, testOpts())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), "pingback") {
		t.Errorf("expected pingback link to be removed, got: %s", out)
	}
}

func TestProcessRemovesWPForms(t *testing.T) {
	base := testBase(t)
	in := `<html><body>
<form action="/wp-comments-post.php"><input name="comment"></form>
<form action="/contact"><input name="email"></form>
</body></html>`
	out, rep, err := Process([]byte(in), base, testOpts())
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if strings.Contains(s, "wp-comments-post") {
		t.Errorf("expected comment form to be removed, got: %s", s)
	}
	if !strings.Contains(s, "/contact") {
		t.Errorf("expected non-wp form to remain, got: %s", s)
	}
	if rep.RemovedForms != 1 {
		t.Errorf("expected 1 removed form, got %d", rep.RemovedForms)
	}
}

func TestProcessStripAllForms(t *testing.T) {
	base := testBase(t)
	opts := testOpts()
	opts.StripForms = "all"
	in := `<html><body><form action="/contact"><input></form></body></html>`
	out, rep, err := Process([]byte(in), base, opts)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), "<form") {
		t.Errorf("expected all forms removed, got: %s", out)
	}
	if rep.RemovedForms != 1 {
		t.Errorf("expected 1 removed form, got %d", rep.RemovedForms)
	}
}

func TestProcessStripsTrackingParams(t *testing.T) {
	base := testBase(t)
	in := `<html><body><a href="/about?utm_source=news&fbclid=abc&id=5">About</a></body></html>`
	out, _, err := Process([]byte(in), base, testOpts())
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if strings.Contains(s, "utm_source") || strings.Contains(s, "fbclid") {
		t.Errorf("expected tracking params stripped, got: %s", s)
	}
	if !strings.Contains(s, "id=5") {
		t.Errorf("expected non-tracking param to remain, got: %s", s)
	}
}

func TestProcessMakesSameHostRelative(t *testing.T) {
	base := testBase(t)
	in := `<html><body><a href="http://example.com/about">About</a></body></html>`
	out, _, err := Process([]byte(in), base, testOpts())
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if strings.Contains(s, "http://example.com/about") {
		t.Errorf("expected same-host absolute URL to become relative, got: %s", s)
	}
	if !strings.Contains(s, "/about") {
		t.Errorf("expected /about to remain, got: %s", s)
	}
}

func TestIsBlocked(t *testing.T) {
	bl := []string{"/wp-admin/", "/wp-login.php"}
	cases := []struct {
		path string
		want bool
	}{
		{"/wp-admin/", true},
		{"/wp-admin/profile.php", true},
		{"/wp-login.php", true},
		{"/wp-login.php?redirect_to=x", true},
		{"/about", false},
		{"/wp-administrative", false}, // prefix must match a real boundary
	}
	for _, c := range cases {
		if got := IsBlocked(c.path, bl); got != c.want {
			t.Errorf("IsBlocked(%q) = %v, want %v", c.path, got, c.want)
		}
	}
}

func TestCleanQuery(t *testing.T) {
	u, _ := url.Parse("http://x/p?a=1&b=2&utm_source=c")
	c := CleanQuery(u, []string{"utm_source"})
	if c.Query().Has("utm_source") {
		t.Errorf("expected utm_source removed")
	}
	if c.Query().Get("a") != "1" || c.Query().Get("b") != "2" {
		t.Errorf("expected other params preserved, got %v", c.Query())
	}
}
