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
