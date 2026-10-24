// Package minify minifies HTML/CSS/JS and writes gzip siblings for nginx.
package minify

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tdewolff/minify/v2"
	"github.com/tdewolff/minify/v2/css"
	minifyhtml "github.com/tdewolff/minify/v2/html"
	"github.com/tdewolff/minify/v2/js"
)

// Options selects which content types are minified.
type Options struct {
	Enabled bool
	HTML    bool
	CSS     bool
	JS      bool
}

// M is the concrete minifier type (alias of the tdewolff minifier).
type M = minify.M

// New returns a configured minifier.
func New() *minify.M {
	m := minify.New()
	m.AddFunc("text/html", minifyhtml.Minify)
	m.AddFunc("text/css", css.Minify)
	m.AddFunc("application/javascript", js.Minify)
	m.AddFunc("application/x-javascript", js.Minify)
	m.AddFunc("text/javascript", js.Minify)
	return m
}

// BaseMIME strips parameters (e.g. "; charset=utf-8") from a MIME type.
func BaseMIME(mime string) string {
	if i := strings.IndexByte(mime, ';'); i >= 0 {
		return strings.TrimSpace(mime[:i])
	}
	return strings.TrimSpace(mime)
}

// MinifyBytes minifies content for the given MIME type. On any error it
// returns the original content unchanged (minification is best-effort).
func MinifyBytes(m *minify.M, mime string, content []byte, opts Options) []byte {
	if !opts.Enabled || m == nil || len(content) == 0 {
		return content
	}
	base := BaseMIME(mime)
	var enabled bool
	switch {
	case base == "text/html" || base == "application/xhtml+xml":
		enabled = opts.HTML
	case base == "text/css":
		enabled = opts.CSS
	case base == "application/javascript" || base == "application/x-javascript" || base == "text/javascript":
		enabled = opts.JS
	default:
		return content
	}
	if !enabled {
		return content
	}
	out, err := m.Bytes(base, content)
	if err != nil {
		return content
	}
	return out
}

// GzipBytes returns the gzip-compressed form of content.
func GzipBytes(content []byte) ([]byte, error) {
	var buf bytes.Buffer
	w, err := gzip.NewWriterLevel(&buf, gzip.BestSpeed)
	if err != nil {
		return nil, fmt.Errorf("gzip writer: %w", err)
	}
	if _, err := w.Write(content); err != nil {
		return nil, fmt.Errorf("gzip write: %w", err)
	}
	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("gzip close: %w", err)
	}
	return buf.Bytes(), nil
}

// WriteFile writes content to path (creating parent dirs) and, when gzip is
// enabled, also writes a path+".gz" sibling for nginx gzip_static.
func WriteFile(path string, content []byte, gzip bool) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir for %s: %w", path, err)
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	if gzip {
		gz, err := GzipBytes(content)
		if err != nil {
			return err
		}
		if err := os.WriteFile(path+".gz", gz, 0o644); err != nil {
			return fmt.Errorf("write %s.gz: %w", path, err)
		}
	}
	return nil
}
