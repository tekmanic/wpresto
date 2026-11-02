// Package geoip is a wpresto inventory plugin that serves client geolocation.
// It serves GET /wpresto/geoip and returns the client's IP along with region
// and country fields instead of routing the request to the WordPress backend.
//
// It is written in AWS Lambda Go HTTP-handler style: the Handle method has the
// exact signature the AWS Lambda runtime uses for API Gateway HTTP proxy
// integrations, so the same code deploys unchanged to Lambda.
//
// This is a stub: it does not call any external GeoIP service or database.
// Wire it to a real GeoIP database (e.g. MaxMind GeoLite2) for real lookups.
package geoip

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"

	"github.com/tekmanic/wpresto/internal/plugin"
)

const name = "geoip"

// Route is the path the plugin serves.
const Route = "/wpresto/geoip"

type geoIP struct{}

func (geoIP) Name() string { return name }
func (geoIP) Description() string {
	return "Serves client geolocation (GET /wpresto/geoip) without the WordPress backend"
}
func (geoIP) Kinds() []string { return []string{"geoip"} }
func (geoIP) Routes() []plugin.Route {
	return []plugin.Route{{Pattern: Route, Methods: []string{"GET"}}}
}

// Handle serves client geolocation for the requesting client.
func (geoIP) Handle(ctx context.Context, req *http.Request) (*http.Response, error) {
	if req.Method != http.MethodGet {
		return jsonResp(http.StatusMethodNotAllowed, map[string]string{"error": "GET only"})
	}
	ip := clientIP(req)
	fmt.Printf("wpresto geoip: ip=%s\n", ip)

	// Stub: no real GeoIP lookup. Wire a GeoIP database for real lookups.
	return jsonResp(http.StatusOK, map[string]string{
		"ip":      ip,
		"region":  "unknown",
		"country": "unknown",
		"note":    "stub — wire a GeoIP database for real lookups",
	})
}

// clientIP determines the client IP from the request. It prefers the first
// X-Forwarded-For value (set by a proxy/load balancer) and falls back to
// req.RemoteAddr with the port stripped.
func clientIP(req *http.Request) string {
	if xff := req.Header.Get("X-Forwarded-For"); xff != "" {
		if first := strings.TrimSpace(strings.Split(xff, ",")[0]); first != "" {
			return first
		}
	}
	host, _, err := net.SplitHostPort(req.RemoteAddr)
	if err != nil {
		return req.RemoteAddr
	}
	return host
}

func init() { plugin.Register(geoIP{}) }

func jsonResp(code int, v any) (*http.Response, error) {
	data, _ := json.Marshal(v)
	return &http.Response{
		StatusCode: code,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(string(data))),
	}, nil
}
