package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage the wpresto configuration file",
	Long: `Manage the wpresto configuration file (YAML).

Config file resolution: -c/--config flag > $WPRESTO_CONFIG > ./wpresto.yaml
Every key can be overridden via env: WPRESTO_ prefix with '.' -> '_'
(e.g. WPRESTO_SITE_URL, WPRESTO_OUTPUT_DIR, WPRESTO_HOOK_SECRET).
Precedence: CLI flag > env > config file > built-in default.

Config keys:
  site.url                    WordPress site to crawl (required)
  output.dir                  Directory the live tree is published to
                              (the site is served from <dir>/live)
  crawl.concurrency           Parallel fetches (default 8)
  crawl.timeout               Per-request timeout, e.g. 30s
  crawl.max_pages             Page cap (default 500)
  crawl.max_asset_bytes       Skip assets larger than this (default 20 MiB)
  strip.strip_forms           all | wp-endpoints | none
  strip.strip_generator_meta  bool (default true)
  strip.blocklist             Override the sensitive-path list
  strip.query_params          Override the tracking-param list
  minify.enabled/html/css/js  bools (default all true)
  compress.gzip               Write .gz siblings (default true)
  hook.enabled/listen         Localhost trigger endpoint (see 'wpresto hook')
  hook.secret                 Set via WPRESTO_HOOK_SECRET env (required when
                              hook.enabled=true)
  user_agent                  UA string used by the crawler`,
}

var configInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Write a sample configuration file",
	Long: `Write a sample wpresto.yaml with all keys and their defaults.
Refuses to overwrite an existing file.`,
	Example: `  wpresto config init
  wpresto config init -c /etc/wpresto/wpresto.yaml`,
	RunE: func(cmd *cobra.Command, args []string) error {
		path := cfgFile
		if path == "" {
			path = "wpresto.yaml"
		}
		if _, err := os.Stat(path); err == nil {
			return fmt.Errorf("%s already exists", path)
		}
		return os.WriteFile(path, []byte(sampleConfig), 0o644)
	},
}

var configGetCmd = &cobra.Command{
	Use:   "get [key]",
	Short: "Print a config value (or the whole config)",
	Long: `Print the effective config value for a key (defaults + file + env merged).
With no key, prints every key. Useful for verifying what 'wpresto build'
will actually use.`,
	Example: `  wpresto config get
  wpresto config get site.url
  wpresto config get output.dir`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		v, err := viperForConfig()
		if err != nil {
			return err
		}
		if len(args) == 0 {
			for _, k := range v.AllKeys() {
				fmt.Printf("%s = %v\n", k, v.Get(k))
			}
			return nil
		}
		fmt.Println(v.Get(args[0]))
		return nil
	},
}

var configSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set a config value in the config file",
	Long: `Set a config value in the config file (creates the file's structure as
needed; run 'wpresto config init' first for a commented sample).
Values are written as strings; use dotted keys (e.g. site.url).`,
	Example: `  wpresto config set site.url http://example.com
  wpresto config set output.dir /var/www/wpresto
  wpresto config set hook.enabled true`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		path := cfgFile
		if path == "" {
			path = "wpresto.yaml"
		}
		v := viper.New()
		v.SetConfigFile(path)
		if err := v.ReadInConfig(); err != nil {
			return fmt.Errorf("read %s: %w (run 'wpresto config init' first)", path, err)
		}
		v.Set(args[0], args[1])
		if err := v.WriteConfig(); err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}
		fmt.Printf("set %s = %s\n", args[0], args[1])
		return nil
	},
}

func init() {
	configCmd.AddCommand(configInitCmd, configGetCmd, configSetCmd)
	rootCmd.AddCommand(configCmd)
}

// viperForConfig loads defaults + file + env into a viper for reading.
func viperForConfig() (*viper.Viper, error) {
	path := cfgFile
	if path == "" {
		path = os.Getenv("WPRESTO_CONFIG")
	}
	if path == "" {
		path = "wpresto.yaml"
	}
	v := viper.New()
	v.SetConfigType("yaml")
	if path != "" {
		v.SetConfigFile(path)
		if err := v.ReadInConfig(); err != nil {
			if os.IsNotExist(err) {
				// No file; just return defaults via env.
			} else {
				return nil, err
			}
		}
	}
	v.SetEnvPrefix("WPRESTO")
	return v, nil
}

const sampleConfig = `# wpresto configuration
site:
  url: http://localhost:8081

output:
  dir: /var/www/wpresto

crawl:
  concurrency: 8
  timeout: 30s
  max_pages: 500
  max_asset_bytes: 20971520

strip:
  strip_forms: wp-endpoints   # all | wp-endpoints | none
  strip_generator_meta: true
  # blocklist:                # override the default sensitive-path list
  #   - /wp-admin/
  # query_params:             # override the default tracking-param list
  #   - utm_source

minify:
  enabled: true
  html: true
  css: true
  js: true

compress:
  gzip: true

hook:
  enabled: false
  listen: 127.0.0.1:9090
  # secret: set via the WPRESTO_HOOK_SECRET environment variable

user_agent: wpresto/1.0 (+local static cache)
`
