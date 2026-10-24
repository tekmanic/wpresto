// Package strip removes WordPress-sensitive content from public HTML:
// admin/login/API/plugin links, forms that post to WordPress endpoints,
// fingerprinting meta tags, and tracking query parameters.
//
// It parses the document with golang.org/x/net/html, rewrites same-host
// absolute URLs to relative ones (so the static cache is host-agnostic),
// and re-renders the tree.
package strip

import (
	"bytes"
	"net/url"
	"strings"

	"golang.org/x/net/html"
)

// Options controls stripping behavior.
type Options struct {
	// Blocklist is a list of path prefixes considered sensitive.
	Blocklist []string
	// StripForms: "all", "wp-endpoints", or "none".
	StripForms string
	// StripGeneratorMeta removes <meta name="generator">.
	StripGeneratorMeta bool
	// QueryParams is a list of query-parameter names to strip from every URL.
	QueryParams []string
}

// Report summarizes what was removed from a page.
type Report struct {
	RemovedLinks    int
	RemovedForms    int
	RemovedElements int
	RewrittenURLs   int
}

// Process strips sensitive content from raw HTML and returns the cleaned
// bytes plus a report. base is the URL the page was fetched from.
func Process(raw []byte, base *url.URL, opts Options) ([]byte, *Report, error) {
	doc, err := html.Parse(bytes.NewReader(raw))
	if err != nil {
		return nil, nil, err
	}

	rep := &Report{}
	walk(doc, base, opts, rep)

	var buf bytes.Buffer
	if err := html.Render(&buf, doc); err != nil {
		return nil, nil, err
	}
	return buf.Bytes(), rep, nil
}

// IsBlocked reports whether a URL path matches any blocklist prefix.
func IsBlocked(path string, blocklist []string) bool {
	for _, prefix := range blocklist {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

// CleanQuery removes the configured tracking parameters from a URL,
// preserving order and the fragment. It returns the (possibly new) URL.
func CleanQuery(u *url.URL, params []string) *url.URL {
	if u == nil {
		return nil
	}
	q := u.Query()
	changed := false
	for _, p := range params {
		if q.Has(p) {
			q.Del(p)
			changed = true
		}
	}
	if !changed {
		return u
	}
	nu := *u
	nu.RawQuery = q.Encode()
	return &nu
}

// walk recursively processes the node tree.
func walk(n *html.Node, base *url.URL, opts Options, rep *Report) {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode {
			if shouldRemove(c, base, opts) {
				removeNode(c)
				if c.Data == "a" {
					rep.RemovedLinks++
				} else if c.Data == "form" {
					rep.RemovedForms++
				} else {
					rep.RemovedElements++
				}
				continue
			}
			rewriteURLs(c, base, opts, rep)
		}
		walk(c, base, opts, rep)
	}
}

// shouldRemove reports whether the element node should be dropped.
func shouldRemove(n *html.Node, base *url.URL, opts Options) bool {
	switch n.Data {
	case "a":
		href, ok := attr(n, "href")
		if !ok {
			return false
		}
		return sensitiveURL(href, base, opts)
	case "form":
		return formShouldRemove(n, base, opts)
	case "meta":
		if opts.StripGeneratorMeta {
			if name, ok := attr(n, "name"); ok && strings.EqualFold(name, "generator") {
				return true
			}
		}
		return false
	case "link":
		// Drop pingback (xmlrpc fingerprint). Keep feeds and stylesheets.
		if rel, ok := attr(n, "rel"); ok && strings.EqualFold(rel, "pingback") {
			return true
		}
		return false
	case "script":
		if src, ok := attr(n, "src"); ok {
			return sensitiveURL(src, base, opts)
		}
		return false
	case "img", "source", "video", "audio", "embed", "iframe":
		for _, a := range []string{"src", "data", "poster"} {
			if v, ok := attr(n, a); ok {
				if sensitiveURL(v, base, opts) {
					return true
				}
			}
		}
		return false
	case "div":
		// Defensive: drop the admin bar if it was ever rendered.
		if id, ok := attr(n, "id"); ok && id == "wpadminbar" {
			return true
		}
		return false
	}
	return false
}

// formShouldRemove applies the StripForms policy.
func formShouldRemove(n *html.Node, base *url.URL, opts Options) bool {
	if opts.StripForms == "none" {
		return false
	}
	if opts.StripForms == "all" {
		return true
	}
	// "wp-endpoints": remove forms whose action resolves to a blocked path.
	action, _ := attr(n, "action")
	if action == "" {
		action = base.Path // form posts to the current page
	}
	u := base.ResolveReference(mustParse(action))
	if u.Host != base.Host {
		return false
	}
	return IsBlocked(u.Path, opts.Blocklist)
}

// sensitiveURL reports whether a (possibly relative) URL reference points at
// a sensitive same-host resource.
func sensitiveURL(ref string, base *url.URL, opts Options) bool {
	ref = strings.TrimSpace(ref)
	if ref == "" || strings.HasPrefix(ref, "javascript:") || strings.HasPrefix(ref, "mailto:") || strings.HasPrefix(ref, "tel:") || strings.HasPrefix(ref, "#") {
		return false
	}
	u := base.ResolveReference(mustParse(ref))
	if u.Host != base.Host {
		return false
	}
	return IsBlocked(u.Path, opts.Blocklist)
}

// rewriteURLs rewrites URL-bearing attributes on a surviving element:
// strips tracking params and makes same-host absolute URLs relative.
func rewriteURLs(n *html.Node, base *url.URL, opts Options, rep *Report) {
	for i := range n.Attr {
		a := &n.Attr[i]
		key := a.Key
		if key != "href" && key != "src" && key != "action" && key != "poster" && key != "data" {
			continue
		}
		val := strings.TrimSpace(a.Val)
		if val == "" || strings.HasPrefix(val, "javascript:") || strings.HasPrefix(val, "mailto:") || strings.HasPrefix(val, "tel:") || strings.HasPrefix(val, "#") || strings.HasPrefix(val, "data:") {
			continue
		}
		u := base.ResolveReference(mustParse(val))
		u = CleanQuery(u, opts.QueryParams)
		// Only rewrite same-host URLs to relative; leave external alone.
		if u.Host == base.Host {
			a.Val = u.RequestURI()
			rep.RewrittenURLs++
		} else {
			a.Val = u.String()
		}
	}
}

// attr returns the value of the first attribute with the given key.
func attr(n *html.Node, key string) (string, bool) {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val, true
		}
	}
	return "", false
}

// removeNode unlinks n from its parent and siblings.
func removeNode(n *html.Node) {
	if n.PrevSibling != nil {
		n.PrevSibling.NextSibling = n.NextSibling
	}
	if n.NextSibling != nil {
		n.NextSibling.PrevSibling = n.PrevSibling
	}
	if n.Parent != nil {
		if n.Parent.FirstChild == n {
			n.Parent.FirstChild = n.NextSibling
		}
		if n.Parent.LastChild == n {
			n.Parent.LastChild = n.PrevSibling
		}
	}
}

// mustParse parses a URL reference, returning an empty URL on error.
func mustParse(s string) *url.URL {
	u, err := url.Parse(s)
	if err != nil {
		return &url.URL{}
	}
	return u
}
