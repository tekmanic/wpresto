// Package contactform is a wpresto inventory plugin that replaces a WordPress
// contact form. It serves POST /wpresto/contact, validates the submitted
// fields, and records the submission (to stdout by default) instead of posting
// to the WordPress backend.
//
// It is written in AWS Lambda Go HTTP-handler style: the Handle method has the
// exact signature the AWS Lambda runtime uses for API Gateway HTTP proxy
// integrations, so the same code deploys unchanged to Lambda.
package contactform

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

const name = "contact-form"

// Route is the path the plugin serves.
const Route = "/wpresto/contact"

type contactForm struct{}

func (contactForm) Name() string { return name }
func (contactForm) Description() string {
	return "Replaces a WordPress contact form (POST /wpresto/contact)"
}
func (contactForm) Kinds() []string { return []string{"contact-form"} }
func (contactForm) Routes() []plugin.Route {
	return []plugin.Route{{Pattern: Route, Methods: []string{"POST"}}}
}

// Handle processes a contact-form submission.
func (contactForm) Handle(ctx context.Context, req *http.Request) (*http.Response, error) {
	if req.Method != http.MethodPost {
		return jsonResp(http.StatusMethodNotAllowed, map[string]string{"error": "POST only"})
	}
	body, err := io.ReadAll(io.LimitReader(req.Body, 1<<20))
	if err != nil {
		return jsonResp(http.StatusBadRequest, map[string]string{"error": "read body: " + err.Error()})
	}
	// Accept both urlencoded form posts and JSON bodies.
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

	name := strings.TrimSpace(fields["name"])
	email := strings.TrimSpace(fields["email"])
	message := strings.TrimSpace(fields["message"])
	if name == "" || email == "" || message == "" {
		return jsonResp(http.StatusUnprocessableEntity,
			map[string]string{"error": "name, email, and message are required"})
	}
	if !strings.Contains(email, "@") {
		return jsonResp(http.StatusUnprocessableEntity, map[string]string{"error": "invalid email"})
	}

	// "Send" the submission. In production this would call an email API or
	// write to a queue; here we log it so the behavior is observable.
	fmt.Printf("wpresto contact-form: from=%s name=%s message_bytes=%d\n", email, name, len(message))

	return jsonResp(http.StatusOK, map[string]string{"status": "ok", "message": "thanks, we'll be in touch"})
}

func init() { plugin.Register(contactForm{}) }

// parseForm parses an application/x-www-form-urlencoded body into a map.
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
