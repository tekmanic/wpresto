// Package urlrewrite is a wpresto inventory plugin that rewrites
// legacy/changed URLs to their current paths, so old links keep working on
// the static site. It serves GET /wpresto/rewrite/* and answers with a 302
// redirect to the mapped target path.
//
// It is written in AWS Lambda Go HTTP-handler style: the Handle method has the
// exact signature the AWS Lambda runtime uses for API Gateway HTTP proxy
// integrations, so the same code deploys unchanged to Lambda.
//
// The rewrite map below is a stub; in production it would be loaded from
// config (e.g. a JSON file or environment variable).
package urlrewrite

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/tekmanic/wpresto/internal/plugin"
)

const name = "urlrewrite"

// Route is the path prefix the plugin serves. The trailing "*" makes it a
// prefix match.
const Route = "/wpresto/rewrite/*"

// rewrites maps legacy paths to their current paths.
var rewrites = map[string]string{
	"/wpresto/rewrite/old-about":   "/about",
	"/wpresto/rewrite/old-contact": "/contact",
	"/wpresto/rewrite/team":        "/about/team",
}

type urlRewrite struct{}

func (urlRewrite) Name() string { return name }
func (urlRewrite) Description() string {
	return "Rewrites legacy/changed URLs to their current paths (GET /wpresto/rewrite/*)"
}
func (urlRewrite) Kinds() []string { return []string{"urlrewrite"} }
func (urlRewrite) Routes() []plugin.Route {
	return []plugin.Route{{Pattern: Route, Methods: []string{"GET"}}}
}

// Handle rewrites a legacy URL to its current path.
func (urlRewrite) Handle(ctx context.Context, req *http.Request) (*http.Response, error) {
	if req.Method != http.MethodGet {
		return jsonResp(http.StatusMethodNotAllowed, map[string]string{"error": "GET only"})
	}
	target, ok := rewrites[req.URL.Path]
	if !ok {
		return jsonResp(http.StatusNotFound, map[string]string{"error": "no rewrite for " + req.URL.Path})
	}
	fmt.Printf("wpresto urlrewrite: %s -> %s\n", req.URL.Path, target)
	return &http.Response{
		StatusCode: http.StatusFound,
		Header:     http.Header{"Location": []string{target}},
		Body:       io.NopCloser(strings.NewReader("")),
	}, nil
}

func init() { plugin.Register(urlRewrite{}) }

func jsonResp(code int, v any) (*http.Response, error) {
	data, _ := json.Marshal(v)
	return &http.Response{
		StatusCode: code,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(string(data))),
	}, nil
}
