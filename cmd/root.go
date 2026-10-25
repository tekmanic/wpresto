// Package cmd implements the wpresto command-line interface.
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// Version is set at build time via -ldflags.
var Version = "dev"

var (
	cfgFile string
	verbose bool
)

var rootCmd = &cobra.Command{
	Use:   "wpresto",
	Short: "WordPress-to-static exporter: crawl a WP site into portable, deployable static files",
	Long: `wpresto is an open-source WordPress-to-static exporter that transforms a
WordPress site into portable, deployable static files.

Pipeline (one 'wpresto build'):
  1. CRAWL   Fetches the site as an anonymous visitor (no cookies, so the
             admin bar and logged-in content never enter the cache). BFS from
             the homepage, same-host only, with concurrency and page caps.
  2. STRIP   Removes the WordPress surface: /wp-admin/, /wp-login.php,
             /wp-json/, xmlrpc.php, generator meta, tracking query params.
             Same-host absolute URLs are rewritten to relative so the output
             is host-agnostic.
  3. MINIFY  Minifies HTML/CSS/JS and writes .gz siblings for nginx
             gzip_static.
  4. PUBLISH Atomically publishes to <output.dir>/live (staging dir + rename,
             previous build moved aside), so nginx never serves a half-written
             site. Writes <output.dir>/live/build-info.json.

The result is a plain static tree. Serve it with nginx: static hits from
<output.dir>/live, dynamic fallback to the WordPress backend for anything not
cached (see examples/nginx.conf in the repo).

QUICK START
  wpresto config init                # writes ./wpresto.yaml
  wpresto config set site.url http://your-site.example
  wpresto config set output.dir /var/www/wpresto
  wpresto build                      # crawl + publish
  wpresto hook                       # (optional) localhost trigger endpoint

CONFIG RESOLUTION
  Config file: -c/--config flag > $WPRESTO_CONFIG > ./wpresto.yaml
  Every key can also be set via env with the WPRESTO_ prefix and '.' -> '_'
  (e.g. WPRESTO_SITE_URL, WPRESTO_OUTPUT_DIR, WPRESTO_HOOK_SECRET).
  Precedence: CLI flag > env > config file > built-in default.
  Run 'wpresto config get' to see the effective values.

RUNNING IT
  Cron:      */15 * * * * wpresto build >> /var/log/wpresto.log 2>&1
  On demand: wpresto hook  (the bundled WordPress plugin's "Go Live" button
              POSTs to it; see 'wpresto hook --help')`,
	SilenceUsage:  true,
	SilenceErrors: true,
}

// Execute runs the root command.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "", "path to config file (default: $WPRESTO_CONFIG or ./wpresto.yaml)")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "enable verbose logging")
}
