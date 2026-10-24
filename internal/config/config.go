// Package config loads and validates wpstatic configuration.
//
// Precedence (highest wins): CLI flag > environment variable > config file > default.
// Secrets (hook secret) should be supplied via environment, never committed to a file.
package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Config is the root configuration for wpstatic.
type Config struct {
	Site      SiteConfig     `mapstructure:"site"`
	Output    OutputConfig   `mapstructure:"output"`
	Crawl     CrawlConfig    `mapstructure:"crawl"`
	Strip     StripConfig    `mapstructure:"strip"`
	Minify    MinifyConfig   `mapstructure:"minify"`
	Compress  CompressConfig `mapstructure:"compress"`
	Hook      HookConfig     `mapstructure:"hook"`
	UserAgent string         `mapstructure:"user_agent"`
}

// SiteConfig describes the WordPress origin to scrape.
type SiteConfig struct {
	// URL is the base URL of the WordPress site, e.g. http://localhost:8081.
	URL string `mapstructure:"url"`
}

// OutputConfig describes where the static filesystem is written.
type OutputConfig struct {
	// Dir is the static filesystem root. The live tree is published to <Dir>/live.
	Dir string `mapstructure:"dir"`
}

// CrawlConfig controls the crawler.
type CrawlConfig struct {
	// Concurrency is the number of parallel fetches.
	Concurrency int `mapstructure:"concurrency"`
	// Timeout is the per-request timeout.
	Timeout time.Duration `mapstructure:"timeout"`
	// MaxPages caps the number of HTML pages crawled (safety valve).
	MaxPages int `mapstructure:"max_pages"`
	// MaxAssetBytes caps the size of a single downloaded asset.
	MaxAssetBytes int64 `mapstructure:"max_asset_bytes"`
}

// StripConfig controls what is removed from the public HTML.
type StripConfig struct {
	// Blocklist is a list of path prefixes that are never fetched or crawled.
	Blocklist []string `mapstructure:"blocklist"`
	// StripForms controls form removal: "all", "wp-endpoints", or "none".
	StripForms string `mapstructure:"strip_forms"`
	// StripGeneratorMeta removes the WordPress <meta name="generator"> tag.
	StripGeneratorMeta bool `mapstructure:"strip_generator_meta"`
	// QueryParams is a list of query-parameter names stripped from every URL.
	QueryParams []string `mapstructure:"query_params"`
}

// MinifyConfig controls minification.
type MinifyConfig struct {
	Enabled bool `mapstructure:"enabled"`
	HTML    bool `mapstructure:"html"`
	CSS     bool `mapstructure:"css"`
	JS      bool `mapstructure:"js"`
}

// CompressConfig controls compression.
type CompressConfig struct {
	// Gzip writes a .gz sibling for each file so nginx can use gzip_static.
	Gzip bool `mapstructure:"gzip"`
}

// HookConfig controls the localhost trigger endpoint used by the WP plugin.
type HookConfig struct {
	Enabled bool   `mapstructure:"enabled"`
	Listen  string `mapstructure:"listen"`
	// Secret is the shared secret required by the hook. Prefer the environment.
	Secret string `mapstructure:"secret"`
}

// DefaultBlocklist returns the default sensitive path prefixes.
func DefaultBlocklist() []string {
	return []string{
		"/wp-admin/",
		"/wp-login.php",
		"/wp-json/",
		"/xmlrpc.php",
		"/wp-cron.php",
		"/wp-activate.php",
		"/wp-signup.php",
		"/wp-links-opml.php",
		"/wp-trackback.php",
		"/wp-comments-post.php",
		"/wp-mail.php",
		"/wp-pass.php",
		"/wp-register.php",
		"/wp-settings.php",
		"/readme.html",
		"/license.txt",
		"/wp-content/mu-plugins/",
		"/wp-content/debug.log",
		"/wp-config.php",
		"/wp-config.",
		"/.env",
		"/.git/",
		"/.svn/",
		"/.htaccess",
		"/wp-includes/wp-db.php",
		"/wp-includes/class-wpdb.php",
	}
}

// DefaultQueryParams returns the default query-parameter names to strip.
func DefaultQueryParams() []string {
	return []string{
		"utm_source", "utm_medium", "utm_campaign", "utm_term", "utm_content", "utm_id",
		"fbclid", "gclid", "ref", "replytocom", "cpage",
		"_wpnonce", "_wp_http_referer", "share",
		"add-to-cart", "orderby", "order", "product-page",
	}
}

// Default returns a Config populated with sensible defaults.
func Default() *Config {
	return &Config{
		Site:      SiteConfig{URL: "http://localhost:8081"},
		Output:    OutputConfig{Dir: "/var/www/wpstatic"},
		Crawl:     CrawlConfig{Concurrency: 8, Timeout: 30 * time.Second, MaxPages: 500, MaxAssetBytes: 20 << 20},
		Strip:     StripConfig{Blocklist: DefaultBlocklist(), StripForms: "wp-endpoints", StripGeneratorMeta: true, QueryParams: DefaultQueryParams()},
		Minify:    MinifyConfig{Enabled: true, HTML: true, CSS: true, JS: true},
		Compress:  CompressConfig{Gzip: true},
		Hook:      HookConfig{Enabled: false, Listen: "127.0.0.1:9090", Secret: ""},
		UserAgent: "wpstatic/1.0 (+local static cache)",
	}
}

// Load reads configuration from the given file path (if non-empty), the
// environment, and returns a validated *Config.
func Load(path string) (*Config, error) {
	v := viper.New()
	v.SetConfigType("yaml")

	// Bind defaults so every key has a value even if absent from the file.
	def := Default()
	v.SetDefault("site.url", def.Site.URL)
	v.SetDefault("output.dir", def.Output.Dir)
	v.SetDefault("crawl.concurrency", def.Crawl.Concurrency)
	v.SetDefault("crawl.timeout", def.Crawl.Timeout.String())
	v.SetDefault("crawl.max_pages", def.Crawl.MaxPages)
	v.SetDefault("crawl.max_asset_bytes", def.Crawl.MaxAssetBytes)
	v.SetDefault("strip.blocklist", def.Strip.Blocklist)
	v.SetDefault("strip.strip_forms", def.Strip.StripForms)
	v.SetDefault("strip.strip_generator_meta", def.Strip.StripGeneratorMeta)
	v.SetDefault("strip.query_params", def.Strip.QueryParams)
	v.SetDefault("minify.enabled", def.Minify.Enabled)
	v.SetDefault("minify.html", def.Minify.HTML)
	v.SetDefault("minify.css", def.Minify.CSS)
	v.SetDefault("minify.js", def.Minify.JS)
	v.SetDefault("compress.gzip", def.Compress.Gzip)
	v.SetDefault("hook.enabled", def.Hook.Enabled)
	v.SetDefault("hook.listen", def.Hook.Listen)
	v.SetDefault("hook.secret", def.Hook.Secret)
	v.SetDefault("user_agent", def.UserAgent)

	// Environment overrides (WPSTATIC_SITE_URL, WPSTATIC_HOOK_SECRET, ...).
	v.SetEnvPrefix("WPSTATIC")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	if path != "" {
		v.SetConfigFile(path)
		if err := v.ReadInConfig(); err != nil {
			return nil, fmt.Errorf("read config %q: %w", path, err)
		}
	}

	cfg := &Config{}
	if err := v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// Validate checks the configuration for correctness.
func (c *Config) Validate() error {
	if c.Site.URL == "" {
		return fmt.Errorf("site.url is required")
	}
	if !strings.HasPrefix(c.Site.URL, "http://") && !strings.HasPrefix(c.Site.URL, "https://") {
		return fmt.Errorf("site.url must start with http:// or https://")
	}
	if c.Output.Dir == "" {
		return fmt.Errorf("output.dir is required")
	}
	if c.Crawl.Concurrency < 1 {
		return fmt.Errorf("crawl.concurrency must be >= 1")
	}
	if c.Crawl.Timeout <= 0 {
		return fmt.Errorf("crawl.timeout must be > 0")
	}
	if c.Crawl.MaxPages < 1 {
		return fmt.Errorf("crawl.max_pages must be >= 1")
	}
	switch c.Strip.StripForms {
	case "all", "wp-endpoints", "none":
	default:
		return fmt.Errorf("strip.strip_forms must be one of: all, wp-endpoints, none")
	}
	if c.Hook.Enabled && c.Hook.Secret == "" {
		return fmt.Errorf("hook.secret is required when hook.enabled is true")
	}
	return nil
}
