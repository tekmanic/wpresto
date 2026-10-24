// Package crawl performs a breadth-first crawl of a WordPress site as an
// anonymous user, extracting same-host pages and assets, stripping sensitive
// content, minifying, and writing the result into a staging directory.
package crawl

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/tekmanic/wpstatic/internal/config"
	"github.com/tekmanic/wpstatic/internal/minify"
	"github.com/tekmanic/wpstatic/internal/publish"
	"github.com/tekmanic/wpstatic/internal/strip"
	"golang.org/x/net/html"
)

type kind int

const (
	kindPage kind = iota
	kindAsset
)

// Crawler crawls a WordPress origin and writes a static tree into a staging
// directory. It is safe for a single Run per instance.
type Crawler struct {
	base      *url.URL
	userAgent string
	crawlCfg  config.CrawlConfig
	stripOpt  strip.Options
	minOpt    minify.Options
	gzip      bool
	staging   string
	client    *http.Client
	minifier  *minify.M
	maxAsset  int64

	mu       sync.Mutex
	cond     *sync.Cond
	seen     map[string]bool
	queue    []job
	inflight int
	closed   bool

	pages   int
	assets  int
	skipped int
	errors  int
}

type job struct {
	u    *url.URL
	kind kind
}

// Result summarizes a crawl.
type Result struct {
	Pages   int
	Assets  int
	Skipped int
	Errors  int
	Bytes   int64
}

// New builds a Crawler. staging is the directory the static tree is written to.
func New(base *url.URL, userAgent string, ccfg config.CrawlConfig, sopt strip.Options, mopt minify.Options, gzip bool, staging string, client *http.Client) *Crawler {
	if client == nil {
		client = &http.Client{}
	}
	c := &Crawler{
		base:      base,
		userAgent: userAgent,
		crawlCfg:  ccfg,
		stripOpt:  sopt,
		minOpt:    mopt,
		gzip:      gzip,
		staging:   staging,
		client:    client,
		minifier:  minify.New(),
		seen:      make(map[string]bool),
		maxAsset:  ccfg.MaxAssetBytes,
	}
	c.cond = sync.NewCond(&c.mu)
	return c
}

// Run executes the crawl until the queue drains or ctx is cancelled.
func (c *Crawler) Run(ctx context.Context) (*Result, error) {
	c.push(c.base, kindPage)

	go func() {
		<-ctx.Done()
		c.mu.Lock()
		c.closed = true
		c.cond.Broadcast()
		c.mu.Unlock()
	}()

	var wg sync.WaitGroup
	for i := 0; i < c.crawlCfg.Concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				j, ok := c.pop(ctx)
				if !ok {
					return
				}
				c.process(ctx, j)
			}
		}()
	}
	wg.Wait()

	res := &Result{Pages: c.pages, Assets: c.assets, Skipped: c.skipped, Errors: c.errors}
	res.Bytes, _ = publish.TreeBytes(c.staging)
	return res, nil
}

// push enqueues a URL if not already seen.
func (c *Crawler) push(u *url.URL, k kind) {
	if u == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || c.seen[normKey(u, k)] {
		return
	}
	if k == kindPage {
		if c.pages >= c.crawlCfg.MaxPages {
			c.skipped++
			return
		}
		c.pages++
	}
	c.seen[normKey(u, k)] = true
	c.queue = append(c.queue, job{u: u, kind: k})
	c.inflight++
	c.cond.Signal()
}

// pop blocks until a job is available or the crawl is finished/cancelled.
func (c *Crawler) pop(ctx context.Context) (job, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for len(c.queue) == 0 && c.inflight > 0 && !c.closed {
		c.cond.Wait()
	}
	if len(c.queue) == 0 {
		return job{}, false
	}
	j := c.queue[0]
	c.queue = c.queue[1:]
	return j, true
}

// process fetches and handles a single job, enqueuing any new references.
func (c *Crawler) process(ctx context.Context, j job) {
	var newJobs []job
	switch j.kind {
	case kindPage:
		newJobs = c.processPage(ctx, j.u)
	case kindAsset:
		newJobs = c.processAsset(ctx, j.u)
	}

	c.mu.Lock()
	c.inflight--
	for _, nj := range newJobs {
		if c.closed || c.seen[normKey(nj.u, nj.kind)] {
			continue
		}
		// Never fetch or crawl sensitive paths.
		if strip.IsBlocked(nj.u.Path, c.stripOpt.Blocklist) {
			continue
		}
		if nj.kind == kindPage && c.pages >= c.crawlCfg.MaxPages {
			c.skipped++
			continue
		}
		if nj.kind == kindPage {
			c.pages++
		} else {
			c.assets++
		}
		c.seen[normKey(nj.u, nj.kind)] = true
		c.queue = append(c.queue, nj)
		c.inflight++
	}
	c.cond.Broadcast()
	c.mu.Unlock()
}

func (c *Crawler) processPage(ctx context.Context, u *url.URL) []job {
	body, final, ctype, err := c.fetch(ctx, u)
	if err != nil {
		c.noteError(u, err)
		return nil
	}
	if final != nil {
		u = final
	}
	if u.RawQuery != "" {
		// Pages with a residual query can't map to a clean static path.
		c.noteSkipped(u)
		return nil
	}

	var newJobs []job
	for _, p := range extractPages(body, u) {
		newJobs = append(newJobs, job{u: p, kind: kindPage})
	}
	for _, a := range extractAssets(body, u) {
		newJobs = append(newJobs, job{u: a, kind: kindAsset})
	}

	clean, _, err := strip.Process(body, u, c.stripOpt)
	if err != nil {
		clean = body
	}
	out := minify.MinifyBytes(c.minifier, ctype, clean, c.minOpt)

	path := filepath.Join(c.staging, pagePath(u))
	if err := minify.WriteFile(path, out, c.gzip); err != nil {
		c.noteError(u, err)
	}
	return newJobs
}

func (c *Crawler) processAsset(ctx context.Context, u *url.URL) []job {
	body, final, ctype, err := c.fetch(ctx, u)
	if err != nil {
		c.noteError(u, err)
		return nil
	}
	if final != nil {
		u = final
	}

	var newJobs []job
	if strings.HasPrefix(minify.BaseMIME(ctype), "text/css") {
		for _, ref := range extractCSSRefs(body, u) {
			newJobs = append(newJobs, job{u: ref, kind: kindAsset})
		}
	}

	out := minify.MinifyBytes(c.minifier, ctype, body, c.minOpt)
	path := filepath.Join(c.staging, assetPath(u))
	if err := minify.WriteFile(path, out, c.gzip); err != nil {
		c.noteError(u, err)
	}
	return newJobs
}

// fetch performs a GET, returning the body, the final URL, and content type.
func (c *Crawler) fetch(ctx context.Context, u *url.URL) ([]byte, *url.URL, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, nil, "", err
	}
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Accept", "*/*")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, nil, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, nil, "", fmt.Errorf("status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, c.maxAsset+1))
	if err != nil {
		return nil, nil, "", err
	}
	if int64(len(body)) > c.maxAsset {
		return nil, nil, "", fmt.Errorf("asset exceeds %d bytes", c.maxAsset)
	}
	return body, resp.Request.URL, resp.Header.Get("Content-Type"), nil
}

func (c *Crawler) noteError(u *url.URL, err error) {
	c.mu.Lock()
	c.errors++
	c.mu.Unlock()
	fmt.Printf("wpstatic: error %s: %v\n", u, err)
}

func (c *Crawler) noteSkipped(u *url.URL) {
	c.mu.Lock()
	c.skipped++
	c.mu.Unlock()
}

// normKey returns a dedup key for a URL of a given kind.
func normKey(u *url.URL, k kind) string {
	s := u.Scheme + "://" + strings.ToLower(u.Host) + u.Path
	if k == kindAsset && u.RawQuery != "" {
		s += "?" + u.RawQuery
	}
	return s
}

// pagePath maps a page URL to a static filesystem path.
func pagePath(u *url.URL) string {
	p := strings.TrimPrefix(u.Path, "/")
	if p == "" {
		return "index.html"
	}
	last := filepath.Base(p)
	if strings.Contains(last, ".") {
		return p
	}
	return strings.TrimSuffix(p, "/") + "/index.html"
}

// assetPath maps an asset URL to a static filesystem path (original layout).
func assetPath(u *url.URL) string {
	p := strings.TrimPrefix(u.Path, "/")
	if p == "" {
		return "index.html"
	}
	return p
}

// extractPages pulls candidate page URLs out of raw HTML.
func extractPages(raw []byte, base *url.URL) []*url.URL {
	var pages []*url.URL
	doc, err := html.Parse(bytes.NewReader(raw))
	if err != nil {
		return nil
	}
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if c.Type == html.ElementNode {
				switch c.Data {
				case "a":
					if href, ok := getAttr(c, "href"); ok {
						if u := resolve(base, href); u != nil {
							pages = append(pages, u)
						}
					}
				case "meta":
					if he, ok := getAttr(c, "http-equiv"); ok && strings.EqualFold(he, "refresh") {
						if content, ok := getAttr(c, "content"); ok {
							if u := parseRefresh(base, content); u != nil {
								pages = append(pages, u)
							}
						}
					}
				}
			}
			walk(c)
		}
	}
	walk(doc)
	return pages
}

// extractAssets pulls candidate asset URLs out of raw HTML.
func extractAssets(raw []byte, base *url.URL) []*url.URL {
	var assets []*url.URL
	doc, err := html.Parse(bytes.NewReader(raw))
	if err != nil {
		return nil
	}
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if c.Type == html.ElementNode {
				switch c.Data {
				case "link":
					if href, ok := getAttr(c, "href"); ok {
						if u := resolve(base, href); u != nil {
							assets = append(assets, u)
						}
					}
				case "script":
					if src, ok := getAttr(c, "src"); ok {
						if u := resolve(base, src); u != nil {
							assets = append(assets, u)
						}
					}
				case "img":
					if src, ok := getAttr(c, "src"); ok {
						if u := resolve(base, src); u != nil {
							assets = append(assets, u)
						}
					}
					if ss, ok := getAttr(c, "srcset"); ok {
						assets = append(assets, parseSrcset(base, ss)...)
					}
				case "source":
					if src, ok := getAttr(c, "src"); ok {
						if u := resolve(base, src); u != nil {
							assets = append(assets, u)
						}
					}
					if ss, ok := getAttr(c, "srcset"); ok {
						assets = append(assets, parseSrcset(base, ss)...)
					}
				case "video", "audio":
					if src, ok := getAttr(c, "src"); ok {
						if u := resolve(base, src); u != nil {
							assets = append(assets, u)
						}
					}
				case "embed":
					if src, ok := getAttr(c, "src"); ok {
						if u := resolve(base, src); u != nil {
							assets = append(assets, u)
						}
					}
				}
			}
			walk(c)
		}
	}
	walk(doc)
	return assets
}

// resolve resolves a reference against base, keeping only same-host http(s).
func resolve(base *url.URL, ref string) *url.URL {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil
	}
	lower := strings.ToLower(ref)
	if strings.HasPrefix(lower, "javascript:") || strings.HasPrefix(lower, "mailto:") ||
		strings.HasPrefix(lower, "tel:") || strings.HasPrefix(lower, "data:") ||
		strings.HasPrefix(ref, "#") {
		return nil
	}
	u, err := url.Parse(ref)
	if err != nil {
		return nil
	}
	abs := base.ResolveReference(u)
	if abs.Host != base.Host {
		return nil
	}
	if abs.Scheme != "http" && abs.Scheme != "https" {
		return nil
	}
	return abs
}

// parseSrcset parses a srcset attribute into resolved URLs.
func parseSrcset(base *url.URL, ss string) []*url.URL {
	var out []*url.URL
	for _, part := range strings.Split(ss, ",") {
		tok := strings.TrimSpace(part)
		if tok == "" {
			continue
		}
		if i := strings.IndexAny(tok, " \t"); i >= 0 {
			tok = tok[:i]
		}
		if u := resolve(base, tok); u != nil {
			out = append(out, u)
		}
	}
	return out
}

// parseRefresh extracts the target URL from a meta refresh content value.
func parseRefresh(base *url.URL, content string) *url.URL {
	for _, field := range strings.Split(content, ";") {
		field = strings.TrimSpace(field)
		if strings.HasPrefix(strings.ToLower(field), "url=") {
			return resolve(base, strings.TrimSpace(field[4:]))
		}
	}
	return nil
}

var cssURLRe = regexp.MustCompile(`url\(\s*['"]?([^'")]+?)['"]?\s*\)`)

// extractCSSRefs finds same-host url() references in CSS.
func extractCSSRefs(css []byte, cssURL *url.URL) []*url.URL {
	var out []*url.URL
	for _, m := range cssURLRe.FindAllSubmatch(css, -1) {
		ref := strings.TrimSpace(string(m[1]))
		if ref == "" || strings.HasPrefix(ref, "data:") || strings.HasPrefix(ref, "#") {
			continue
		}
		if u := resolve(cssURL, ref); u != nil {
			out = append(out, u)
		}
	}
	return out
}

func getAttr(n *html.Node, key string) (string, bool) {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val, true
		}
	}
	return "", false
}
