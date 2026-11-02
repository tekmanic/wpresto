package cmd

import (
	"fmt"
	"net/url"

	"github.com/spf13/cobra"
	"github.com/tekmanic/wpresto/internal/plugin"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Run the plugin server that replaces dynamic parts of the site",
	Long: `Run the wpresto plugin server. It wraps the registered plugins (Lambda-style
handlers) in a simple HTTP server and serves them at their routes. Any request
that no plugin handles is proxied to the WordPress backend (plugins.fallback,
defaulting to site.url), so the plugin server can be nginx's single dynamic
entry point.

Requests are routed like this:
  1. If the path+method matches a plugin route -> run that plugin's handler.
  2. Otherwise -> proxy the request to the fallback (WordPress) backend.

This is what makes the static cache "dynamic-capable": forms and endpoints that
wpresto detected during a build are rewritten to point at plugin routes, and the
plugin server answers them without the WordPress backend.

Run it under systemd or in a tmux session (it runs in the foreground). Point
nginx's @dynamic fallback and /wpresto/ location at this server (see
examples/nginx.conf).

Setup:
  1. wpresto config set plugins.enabled true
  2. wpresto config set plugins.listen 127.0.0.1:9091
  3. (optional) wpresto config set plugins.fallback http://127.0.0.1:8081
  4. wpresto serve`,
	Example: `  # run in the foreground (or under systemd)
  wpresto serve

  # hit a plugin route directly
  curl -X POST -d "name=Ada&email=ada@example.com" http://127.0.0.1:9091/wpresto/contact

  # an unmatched request is proxied to WordPress
  curl http://127.0.0.1:9091/some/uncached/page`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := loadConfig()
		if err != nil {
			return err
		}
		if !cfg.Plugins.Enabled {
			return fmt.Errorf("plugins are disabled (set plugins.enabled=true)")
		}
		fallback := cfg.Plugins.Fallback
		if fallback == "" {
			fallback = cfg.Site.URL
		}
		fu, err := url.Parse(fallback)
		if err != nil {
			return fmt.Errorf("parse plugins.fallback: %w", err)
		}
		if fu.Scheme == "" || fu.Host == "" {
			return fmt.Errorf("plugins.fallback %q is not a valid URL", fallback)
		}

		// List the registered plugins so operators can see what is live.
		for _, p := range plugin.All() {
			routes := ""
			for i, r := range p.Routes() {
				if i > 0 {
					routes += ", "
				}
				m := "ANY"
				if len(r.Methods) > 0 {
					m = r.Methods[0]
				}
				routes += m + " " + r.Pattern
			}
			fmt.Printf("wpresto: plugin %-14s %s\n", p.Name(), routes)
		}

		srv := plugin.NewServer(cfg.Plugins.Listen, fu)
		fmt.Printf("wpresto: plugin server listening on %s (fallback %s)\n", cfg.Plugins.Listen, fallback)
		return srv.ServeHTTP(cmd.Context())
	},
}

func init() {
	rootCmd.AddCommand(serveCmd)
}
