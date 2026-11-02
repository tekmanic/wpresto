// Package plugin defines the wpresto plugin system: a small set of
// Lambda-style handlers that replace the dynamic parts of a WordPress site
// (contact forms, subscribe forms, analytics, geoip, url rewrites) so the
// static cache can serve them without the WordPress backend.
//
// A plugin is AWS Lambda Go "HTTP handler" style code:
//
//	func Handler(ctx context.Context, req *http.Request) (*http.Response, error)
//
// The exact signature used by the AWS Lambda Go runtime for API Gateway HTTP
// proxy integrations. wpresto wraps each plugin's Handler in a simple HTTP
// server (see Server) so the same handler code runs unchanged on this host or
// on AWS Lambda.
package plugin

import (
	"context"
	"net/http"
)

// Handler is the AWS Lambda Go HTTP handler signature. A plugin's Handle
// method has this shape: it receives an HTTP request and returns an HTTP
// response (or an error, which the server turns into a 500).
type Handler func(ctx context.Context, req *http.Request) (*http.Response, error)

// Route describes a URL pattern a plugin serves on the plugin server.
type Route struct {
	// Pattern is a path or path prefix the plugin handles, e.g.
	// "/wpresto/contact" (exact) or "/wpresto/rewrite/*" (prefix).
	Pattern string
	// Methods are the HTTP methods handled (e.g. ["POST"]). Empty means all.
	Methods []string
}

// Plugin is a single dynamic-content handler backed by a Lambda-style Handler.
type Plugin interface {
	// Name is the unique plugin identifier (e.g. "contact-form").
	Name() string
	// Description is a human-readable summary of what the plugin replaces.
	Description() string
	// Kinds returns the dynamic-part classes this plugin handles, e.g.
	// ["contact-form"]. Used by the build to match detected parts to plugins.
	Kinds() []string
	// Routes returns the URL patterns the plugin serves on the plugin server.
	Routes() []Route
	// Handle processes a request and returns a response (Lambda style).
	Handle(ctx context.Context, req *http.Request) (*http.Response, error)
}

// NewFuncPlugin adapts a bare Handler function into a Plugin. It is the
// simplest way to define a plugin in the inventory.
type FuncPlugin struct {
	NameStr string
	Desc    string
	KindsL  []string
	RoutesL []Route
	Handler Handler
}

// Name implements Plugin.
func (f FuncPlugin) Name() string { return f.NameStr }

// Description implements Plugin.
func (f FuncPlugin) Description() string { return f.Desc }

// Kinds implements Plugin.
func (f FuncPlugin) Kinds() []string { return f.KindsL }

// Routes implements Plugin.
func (f FuncPlugin) Routes() []Route { return f.RoutesL }

// Handle implements Plugin.
func (f FuncPlugin) Handle(ctx context.Context, req *http.Request) (*http.Response, error) {
	return f.Handler(ctx, req)
}
