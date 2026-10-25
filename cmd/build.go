package cmd

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"github.com/tekmanic/wpresto/internal/config"
	"github.com/tekmanic/wpresto/internal/crawl"
	"github.com/tekmanic/wpresto/internal/minify"
	"github.com/tekmanic/wpresto/internal/publish"
	"github.com/tekmanic/wpresto/internal/strip"
)

var buildCmd = &cobra.Command{
	Use:   "build",
	Short: "Crawl the site and publish a new static build",
	Long: `Crawl the configured WordPress site and publish a new static build.

What happens:
  1. Creates a staging dir <output.dir>/.staging-<buildID>
  2. Crawls site.url as an anonymous user (pages + assets, same host only,
     capped by crawl.max_pages)
  3. Strips the WordPress surface (admin/login/API links, generator meta,
     tracking params) and rewrites same-host URLs to relative
  4. Minifies HTML/CSS/JS and writes .gz siblings (compress.gzip)
  5. Atomically renames staging to <output.dir>/live; the previous build is
     moved aside and removed. On failure the staging dir is cleaned up and
     the existing live tree is untouched.
  6. Writes <output.dir>/live/build-info.json (build id, counts, duration)

Output on success (stdout):
  build <id> complete: N pages, M assets, B bytes in D (skipped S, errors E)

Exit code: 0 on success, 1 on failure.

site.url and output.dir are required (config file or env). All other keys
have sane defaults; see 'wpresto config get' for the effective values.`,
	Example: `  # one-off build with an explicit config file
  wpresto build -c /etc/wpresto/wpresto.yaml

  # no config file: everything via env
  WPRESTO_SITE_URL=http://example.com WPRESTO_OUTPUT_DIR=/var/www/wpresto wpresto build

  # cron entry (every 15 minutes)
  */15 * * * * cd /opt/wpresto && /usr/local/bin/wpresto build >> /var/log/wpresto.log 2>&1`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := loadConfig()
		if err != nil {
			return err
		}
		return RunBuild(cmd.Context(), cfg)
	},
}

func init() {
	rootCmd.AddCommand(buildCmd)
}

// loadConfig resolves the config file path and loads it.
func loadConfig() (*config.Config, error) {
	path := cfgFile
	if path == "" {
		path = os.Getenv("WPRESTO_CONFIG")
	}
	if path == "" {
		path = "wpresto.yaml"
	}
	return config.Load(path)
}

// newBuildID returns a random 16-hex-char build identifier.
func newBuildID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

// RunBuild performs a full crawl + publish cycle.
func RunBuild(ctx context.Context, cfg *config.Config) error {
	base, err := url.Parse(cfg.Site.URL)
	if err != nil {
		return fmt.Errorf("parse site.url: %w", err)
	}
	if base.Scheme == "" || base.Host == "" {
		return fmt.Errorf("site.url %q is not a valid URL", cfg.Site.URL)
	}

	buildID := newBuildID()
	staging := filepath.Join(cfg.Output.Dir, ".staging-"+buildID)
	if err := os.MkdirAll(staging, 0o755); err != nil {
		return fmt.Errorf("create staging dir: %w", err)
	}
	// Clean up the staging dir if the build fails; on success it is renamed.
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.RemoveAll(staging)
		}
	}()

	if verbose {
		fmt.Printf("wpresto: building %s from %s -> %s\n", buildID, cfg.Site.URL, cfg.Output.Dir)
	}

	started := time.Now()
	stripOpt := strip.Options{
		Blocklist:          cfg.Strip.Blocklist,
		StripForms:         cfg.Strip.StripForms,
		StripGeneratorMeta: cfg.Strip.StripGeneratorMeta,
		QueryParams:        cfg.Strip.QueryParams,
	}
	minOpt := minify.Options{
		Enabled: cfg.Minify.Enabled,
		HTML:    cfg.Minify.HTML,
		CSS:     cfg.Minify.CSS,
		JS:      cfg.Minify.JS,
	}

	cr := crawl.New(base, cfg.UserAgent, cfg.Crawl, stripOpt, minOpt, cfg.Compress.Gzip, staging, nil)
	res, err := cr.Run(ctx)
	if err != nil {
		return fmt.Errorf("crawl: %w", err)
	}

	info := &publish.BuildInfo{
		BuildID:    buildID,
		StartedAt:  started,
		FinishedAt: time.Now(),
		Duration:   time.Since(started),
		Pages:      res.Pages,
		Assets:     res.Assets,
		Bytes:      res.Bytes,
		SiteURL:    cfg.Site.URL,
	}

	if err := publish.Publish(cfg.Output.Dir, staging, info); err != nil {
		return fmt.Errorf("publish: %w", err)
	}
	cleanup = false

	fmt.Printf("build %s complete: %d pages, %d assets, %d bytes in %s (skipped %d, errors %d)\n",
		buildID, res.Pages, res.Assets, res.Bytes, info.Duration.Round(time.Millisecond), res.Skipped, res.Errors)
	return nil
}
