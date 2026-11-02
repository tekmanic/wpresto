// Package subscribe replaces a WordPress newsletter/subscribe form with a
// standalone Lambda-style handler. It accepts a single email field (POST
// /wpresto/subscribe), validates it, and "subscribes" by logging the email.
package subscribe

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/tekmanic/wpresto/internal/plugin"
)

const name = "subscribe"
const Route = "/wpresto/subscribe"

type subscribe struct{}

func (subscribe) Name() string { return name }
func (subscribe) Description() string {
	return "Replaces a WordPress newsletter/subscribe form (POST /wpresto/subscribe)"
}
func (subscribe) Kinds() []string { return []string{"subscribe"} }
func (subscribe) Routes() []plugin.Route {
	return []plugin.Route{{Pattern: Route, Methods: []string{"POST"}}}
}

func (subscribe) Handle(ctx context.Context, req *http.Request) (*http.Response, error) {
	if req.Method != http.MethodPost {
		return jsonResp(http.StatusMethodNotAllowed, map[string]string{"error": "POST only"})
	}
	body, err := io.ReadAll(io.LimitReader(req.Body, 1<<20))
	if err != nil {
		return jsonResp(http.StatusBadRequest, map[string]string{"error": "read body: " + err.Error()})
	}
	var fields map[string]string
	if ct := req.Header.Get("Content-Type"); strings.Contains(ct, "json") {
		var m map[string]any
		if err := json.Unmarshal(body, &m); err != nil {
			return jsonResp(http.StatusBadRequest, map[string]string{"error": "bad json"})
		}
		fields = make(map[string]string, len(m))
		for k, v := range m {
			fields[k] = fmt.Sprintf("%v", v)
		}
	} else {
		vals, err := parseForm(body)
		if err != nil {
			return jsonResp(http.StatusBadRequest, map[string]string{"error": "bad form"})
		}
		fields = vals
	}
	email := strings.TrimSpace(fields["email"])
	if email == "" {
		email = strings.TrimSpace(fields["EMAIL"])
	}
	if email == "" {
		email = strings.TrimSpace(fields["email_address"])
	}
	if email == "" || !strings.Contains(email, "@") {
		return jsonResp(http.StatusUnprocessableEntity, map[string]string{"error": "a valid email is required"})
	}
	fmt.Printf("wpresto subscribe: email=%s\n", email)
	return jsonResp(http.StatusOK, map[string]string{"status": "ok", "message": "you're subscribed"})
}

func init() { plugin.Register(subscribe{}) }

func parseForm(body []byte) (map[string]string, error) {
	vals, err := url.ParseQuery(string(body))
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(vals))
	for k, v := range vals {
		if len(v) > 0 {
			out[k] = v[0]
		}
	}
	return out, nil
}

func jsonResp(code int, v any) (*http.Response, error) {
	data, _ := json.Marshal(v)
	return &http.Response{
		StatusCode: code,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(string(data))),
	}, nil
}
