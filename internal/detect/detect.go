// Package detect scans crawled WordPress HTML for dynamic parts — forms,
// analytics/tracking scripts, and API/AJAX endpoints — and classifies each
// into a kind that a wpresto plugin can replace. The build matches detected
// parts to plugins by kind and flags any part no plugin handles.
package detect

import (
	"bytes"
	"net/url"
	"strings"

	"golang.org/x/net/html"
)

// Dynamic-part classes. A plugin's Kinds() lists the classes it handles.
const (
	ClassContactForm = "contact-form"
	ClassSubscribe   = "subscribe"
	ClassCommentForm = "comment-form"
	ClassSearch      = "search"
	ClassForm        = "form" // unclassified form
	ClassAnalytics   = "analytics"
	ClassGeoIP       = "geoip"
	ClassAPI         = "api"
	ClassAJAX        = "ajax"
)

// DynamicPart is a detected piece of dynamic content on a page.
type DynamicPart struct {
	// Page is the path of the page where the part was found.
	Page string `json:"page"`
	// Kind is the element category: "form" or "script".
	Kind string `json:"kind"`
	// Class is the specific classification (e.g. "contact-form", "analytics").
	Class string `json:"class"`
	// Target is the endpoint path the part posts to / loads from.
	Target string `json:"target"`
	// Detail is a human-readable description.
	Detail string `json:"detail"`
}

// Detect scans raw HTML for dynamic parts relative to base.
func Detect(raw []byte, base *url.URL) []DynamicPart {
	doc, err := html.Parse(bytes.NewReader(raw))
	if err != nil {
		return nil
	}
	var parts []DynamicPart
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if c.Type == html.ElementNode {
				switch c.Data {
				case "form":
					if p := classifyForm(c, base); p != nil {
						parts = append(parts, *p)
					}
				case "script":
					if p := classifyScript(c, base); p != nil {
						parts = append(parts, *p)
					}
				}
			}
			walk(c)
		}
	}
	walk(doc)
	return parts
}

// classifyForm classifies a <form> element into a dynamic part.
func classifyForm(n *html.Node, base *url.URL) *DynamicPart {
	action, _ := attr(n, "action")
	method, _ := attr(n, "method")
	if method == "" {
		method = "GET"
	}
	target := base.ResolveReference(mustParse(action)).Path

	return &DynamicPart{
		Page:   base.Path,
		Kind:   "form",
		Class:  ClassifyForm(n, base),
		Target: target,
		Detail: "form " + strings.ToUpper(method) + " " + target,
	}
}

// ClassifyForm returns the dynamic class of a <form> element (one of the
// Class* constants). It is exposed so the build can decide which plugin route
// to rewrite a form's action to.
func ClassifyForm(n *html.Node, base *url.URL) string {
	action, _ := attr(n, "action")
	id, _ := attr(n, "id")
	class, _ := attr(n, "class")
	target := base.ResolveReference(mustParse(action)).Path
	fields := inputNames(n)

	switch {
	case isCommentForm(target, id, class):
		return ClassCommentForm
	case isSearchForm(id, class, fields):
		return ClassSearch
	case isContactForm(class, fields, target):
		return ClassContactForm
	case isSubscribeForm(class, fields, target):
		return ClassSubscribe
	}
	return ClassForm
}

// classifyScript classifies a <script src> element into a dynamic part.
// Inline scripts are not classified (too noisy); only external script srcs
// that point at known dynamic endpoints.
func classifyScript(n *html.Node, base *url.URL) *DynamicPart {
	src, ok := attr(n, "src")
	if !ok {
		return nil
	}
	u := base.ResolveReference(mustParse(src))
	if u.Host != base.Host {
		// External (CDN) scripts are not part of this site's dynamic surface.
		return nil
	}
	cls := classifyScriptPath(u.Path)
	if cls == "" {
		return nil
	}
	return &DynamicPart{
		Page:   base.Path,
		Kind:   "script",
		Class:  cls,
		Target: u.Path,
		Detail: "script " + u.Path,
	}
}

// classifyScriptPath maps a script path to a dynamic class, or "" if it is
// not a recognized dynamic endpoint.
func classifyScriptPath(path string) string {
	lower := strings.ToLower(path)
	switch {
	case strings.Contains(lower, "wp-json"):
		return ClassAPI
	case strings.Contains(lower, "admin-ajax"):
		return ClassAJAX
	case strings.Contains(lower, "analytics") || strings.Contains(lower, "gtag") ||
		strings.Contains(lower, "googletagmanager") || strings.Contains(lower, "clarity") ||
		strings.Contains(lower, "tracking"):
		return ClassAnalytics
	case strings.Contains(lower, "geoip") || strings.Contains(lower, "ipinfo") ||
		strings.Contains(lower, "maxmind") || strings.Contains(lower, "location"):
		return ClassGeoIP
	}
	return ""
}

func isCommentForm(target, id, class string) bool {
	if strings.Contains(target, "wp-comments-post") {
		return true
	}
	lower := strings.ToLower(id + " " + class)
	return strings.Contains(lower, "comment")
}

func isSearchForm(id, class string, fields []string) bool {
	lower := strings.ToLower(id + " " + class)
	if strings.Contains(lower, "search") {
		return true
	}
	for _, f := range fields {
		if f == "s" || f == "search" || f == "search_query" {
			return true
		}
	}
	return false
}

func isContactForm(class string, fields []string, target string) bool {
	lower := strings.ToLower(class)
	if strings.Contains(lower, "wpcf7") || strings.Contains(lower, "contact") ||
		strings.Contains(lower, "contact-form-7") || strings.Contains(lower, "wpforms") {
		return true
	}
	// Contact forms typically have a message field plus an email field.
	hasMessage, hasEmail := false, false
	for _, f := range fields {
		switch strings.ToLower(f) {
		case "your-message", "message", "comment", "subject":
			hasMessage = true
		case "your-email", "email", "e-mail":
			hasEmail = true
		}
	}
	return hasMessage && hasEmail
}

func isSubscribeForm(class string, fields []string, target string) bool {
	lower := strings.ToLower(class + " " + target)
	if strings.Contains(lower, "subscribe") || strings.Contains(lower, "newsletter") ||
		strings.Contains(lower, "mc4wp") || strings.Contains(lower, "mailpoet") ||
		strings.Contains(lower, "newsletter") {
		return true
	}
	// A single email field with a subscribe-ish action.
	if strings.Contains(lower, "subscri") {
		return true
	}
	return false
}

// inputNames collects the name attributes of all inputs/selects/textareas in a form.
func inputNames(n *html.Node) []string {
	var names []string
	var walk func(c *html.Node)
	walk = func(c *html.Node) {
		for child := c.FirstChild; child != nil; child = child.NextSibling {
			if child.Type == html.ElementNode {
				switch child.Data {
				case "input", "select", "textarea":
					if name, ok := attr(child, "name"); ok && name != "" {
						names = append(names, name)
					}
				}
			}
			walk(child)
		}
	}
	walk(n)
	return names
}

func attr(n *html.Node, key string) (string, bool) {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val, true
		}
	}
	return "", false
}

func mustParse(s string) *url.URL {
	u, err := url.Parse(s)
	if err != nil {
		return &url.URL{}
	}
	return u
}
