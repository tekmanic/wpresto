package minify

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMinifyHTML(t *testing.T) {
	m := New()
	in := []byte("<html><body>\n  <p>Hello   World</p>\n</body></html>")
	out := MinifyBytes(m, "text/html; charset=utf-8", in, Options{Enabled: true, HTML: true})
	if len(out) >= len(in) {
		t.Errorf("expected minified html to be shorter: in=%d out=%d", len(in), len(out))
	}
	if !strings.Contains(string(out), "Hello") {
		t.Errorf("expected content preserved: %s", out)
	}
}

func TestMinifyCSS(t *testing.T) {
	m := New()
	in := []byte("body {\n  color: red;\n  margin: 0;\n}")
	out := MinifyBytes(m, "text/css", in, Options{Enabled: true, CSS: true})
	if strings.Contains(string(out), "color: red") {
		t.Errorf("expected css minified (no space after colon): %s", out)
	}
}

func TestMinifyJS(t *testing.T) {
	m := New()
	in := []byte("function f() {\n  var x = 1;\n  return x;\n}")
	out := MinifyBytes(m, "application/javascript", in, Options{Enabled: true, JS: true})
	if len(out) >= len(in) {
		t.Errorf("expected minified js to be shorter: in=%d out=%d", len(in), len(out))
	}
}

func TestMinifyDisabled(t *testing.T) {
	m := New()
	in := []byte("body { color: red; }")
	out := MinifyBytes(m, "text/css", in, Options{Enabled: false})
	if string(out) != string(in) {
		t.Errorf("expected unchanged content when disabled")
	}
}

func TestMinifyUnknownMime(t *testing.T) {
	m := New()
	in := []byte("some binary \x00\x01")
	out := MinifyBytes(m, "image/png", in, Options{Enabled: true, CSS: true, JS: true, HTML: true})
	if string(out) != string(in) {
		t.Errorf("expected unknown mime to pass through unchanged")
	}
}

func TestMinifyFallsBackOnError(t *testing.T) {
	m := New()
	// Invalid CSS that may fail to minify; must not panic and returns original.
	in := []byte("body { color: red; } /* unclosed")
	out := MinifyBytes(m, "text/css", in, Options{Enabled: true, CSS: true})
	if out == nil {
		t.Errorf("expected non-nil output")
	}
}

func TestGzipRoundTrip(t *testing.T) {
	in := []byte(strings.Repeat("hello world, static cache, ", 20))
	gz, err := GzipBytes(in)
	if err != nil {
		t.Fatal(err)
	}
	if len(gz) >= len(in) {
		t.Errorf("expected gzip to be smaller for repetitive input: in=%d gz=%d", len(in), len(gz))
	}
}

func TestWriteFileWithGzip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "a.css")
	if err := WriteFile(path, []byte("body{}"), true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("expected file: %v", err)
	}
	if _, err := os.Stat(path + ".gz"); err != nil {
		t.Errorf("expected .gz sibling: %v", err)
	}
}

func TestWriteFileWithoutGzip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.css")
	if err := WriteFile(path, []byte("body{}"), false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path + ".gz"); !os.IsNotExist(err) {
		t.Errorf("expected no .gz sibling when gzip disabled")
	}
}

func TestBaseMIME(t *testing.T) {
	cases := map[string]string{
		"text/html; charset=utf-8": "text/html",
		"application/javascript":   "application/javascript",
		"  text/css  ":             "text/css",
	}
	for in, want := range cases {
		if got := BaseMIME(in); got != want {
			t.Errorf("BaseMIME(%q) = %q, want %q", in, got, want)
		}
	}
}
