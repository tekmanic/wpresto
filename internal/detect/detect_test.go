package detect

import (
	"bytes"
	"net/url"
	"testing"

	"golang.org/x/net/html"
)

func base(t *testing.T, path string) *url.URL {
	t.Helper()
	u, err := url.Parse("http://example.com" + path)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func TestDetectContactForm(t *testing.T) {
	raw := []byte(`<html><body>
		<form class="wpcf7" action="/contact" method="post">
			<input name="your-name" />
			<input name="your-email" type="email" />
			<textarea name="your-message"></textarea>
			<button>Send</button>
		</form>
	</body></html>`)
	parts := Detect(raw, base(t, "/contact"))
	if len(parts) != 1 {
		t.Fatalf("got %d parts; want 1", len(parts))
	}
	if parts[0].Class != ClassContactForm {
		t.Errorf("class = %q; want %q", parts[0].Class, ClassContactForm)
	}
	if parts[0].Kind != "form" {
		t.Errorf("kind = %q; want form", parts[0].Kind)
	}
}

func TestDetectCommentForm(t *testing.T) {
	raw := []byte(`<html><body>
		<form id="commentform" action="/wp-comments-post.php" method="post">
			<textarea name="comment"></textarea>
		</form>
	</body></html>`)
	parts := Detect(raw, base(t, "/some-post"))
	if len(parts) != 1 {
		t.Fatalf("got %d parts; want 1", len(parts))
	}
	if parts[0].Class != ClassCommentForm {
		t.Errorf("class = %q; want %q", parts[0].Class, ClassCommentForm)
	}
}

func TestDetectSearchForm(t *testing.T) {
	raw := []byte(`<html><body>
		<form class="search-form" role="search" action="/" method="get">
			<input name="s" type="search" />
		</form>
	</body></html>`)
	parts := Detect(raw, base(t, "/"))
	if len(parts) != 1 {
		t.Fatalf("got %d parts; want 1", len(parts))
	}
	if parts[0].Class != ClassSearch {
		t.Errorf("class = %q; want %q", parts[0].Class, ClassSearch)
	}
}

func TestDetectSubscribeForm(t *testing.T) {
	raw := []byte(`<html><body>
		<form class="mc4wp-form" action="/subscribe" method="post">
			<input name="EMAIL" type="email" />
			<button>Subscribe</button>
		</form>
	</body></html>`)
	parts := Detect(raw, base(t, "/"))
	if len(parts) != 1 {
		t.Fatalf("got %d parts; want 1", len(parts))
	}
	if parts[0].Class != ClassSubscribe {
		t.Errorf("class = %q; want %q", parts[0].Class, ClassSubscribe)
	}
}

func TestDetectAnalyticsScript(t *testing.T) {
	raw := []byte(`<html><head>
		<script src="/wp-content/plugins/analytics/gtag.js"></script>
	</head><body></body></html>`)
	parts := Detect(raw, base(t, "/"))
	if len(parts) != 1 {
		t.Fatalf("got %d parts; want 1", len(parts))
	}
	if parts[0].Class != ClassAnalytics {
		t.Errorf("class = %q; want %q", parts[0].Class, ClassAnalytics)
	}
	if parts[0].Kind != "script" {
		t.Errorf("kind = %q; want script", parts[0].Kind)
	}
}

func TestDetectExternalScriptIgnored(t *testing.T) {
	// External (CDN) scripts are not part of this site's dynamic surface.
	raw := []byte(`<html><head>
		<script src="https://cdn.example.com/analytics.js"></script>
	</head><body></body></html>`)
	parts := Detect(raw, base(t, "/"))
	if len(parts) != 0 {
		t.Fatalf("got %d parts; want 0 (external script ignored)", len(parts))
	}
}

func TestDetectNoDynamic(t *testing.T) {
	raw := []byte(`<html><body><p>just text</p></body></html>`)
	parts := Detect(raw, base(t, "/"))
	if len(parts) != 0 {
		t.Fatalf("got %d parts; want 0", len(parts))
	}
}

func TestClassifyFormContactByFields(t *testing.T) {
	// No plugin class marker, but message + email fields => contact form.
	raw := []byte(`<html><body>
		<form action="/send" method="post">
			<input name="name" />
			<input name="email" type="email" />
			<textarea name="message"></textarea>
		</form>
	</body></html>`)
	doc, err := html.Parse(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	var form *html.Node
	var find func(n *html.Node)
	find = func(n *html.Node) {
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if c.Type == html.ElementNode && c.Data == "form" {
				form = c
				return
			}
			find(c)
		}
	}
	find(doc)
	if form == nil {
		t.Fatal("no form found")
	}
	if got := ClassifyForm(form, base(t, "/send")); got != ClassContactForm {
		t.Errorf("ClassifyForm = %q; want %q", got, ClassContactForm)
	}
}
