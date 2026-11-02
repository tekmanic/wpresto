// Package analytics replaces third-party analytics beacons (Google Analytics,
// Google Tag Manager, etc.) with a local wpresto handler. It follows the
// Lambda-style pattern of the other inventory plugins: a single POST endpoint
// that accepts a small JSON event, records it locally (a log line), and
// returns 204 No Content.
package analytics

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/tekmanic/wpresto/internal/plugin"
)

const name = "analytics"
const Route = "/wpresto/analytics"

// event is a single JSON analytics event accepted by the beacon endpoint.
type event struct {
	Path     string `json:"path"`
	Referrer string `json:"referrer"`
	Title    string `json:"title"`
	Session  string `json:"session"`
}

type Analytics struct{}

func (Analytics) Name() string { return name }
func (Analytics) Description() string {
	return "Replaces third-party analytics beacons (POST /wpresto/analytics)"
}
func (Analytics) Kinds() []string { return []string{"analytics"} }
func (Analytics) Routes() []plugin.Route {
	return []plugin.Route{{Pattern: Route, Methods: []string{"POST"}}}
}

func (Analytics) Handle(ctx context.Context, req *http.Request) (*http.Response, error) {
	if req.Method != http.MethodPost {
		return jsonResp(http.StatusMethodNotAllowed, map[string]string{"error": "POST only"})
	}
	body, err := io.ReadAll(io.LimitReader(req.Body, 1<<20))
	if err != nil {
		return jsonResp(http.StatusBadRequest, map[string]string{"error": "read body: " + err.Error()})
	}
	var ev event
	if err := json.Unmarshal(body, &ev); err != nil {
		return jsonResp(http.StatusBadRequest, map[string]string{"error": "bad json"})
	}
	// Record the event locally.
	fmt.Printf("wpresto analytics: path=%s referrer=%s session=%s\n", ev.Path, ev.Referrer, ev.Session)
	return &http.Response{
		StatusCode: 204,
		Header:     http.Header{},
		Body:       io.NopCloser(strings.NewReader("")),
	}, nil
}

func init() { plugin.Register(Analytics{}) }

func jsonResp(code int, v any) (*http.Response, error) {
	data, _ := json.Marshal(v)
	return &http.Response{
		StatusCode: code,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(string(data))),
	}, nil
}
