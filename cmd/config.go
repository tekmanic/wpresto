package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage the wpstatic configuration file",
}

var configInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Write a sample configuration file",
	RunE: func(cmd *cobra.Command, args []string) error {
		path := cfgFile
		if path == "" {
			path = "wpstatic.yaml"
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
	Args:  cobra.MaximumNArgs(1),
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
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		path := cfgFile
		if path == "" {
			path = "wpstatic.yaml"
		}
		v := viper.New()
		v.SetConfigFile(path)
		if err := v.ReadInConfig(); err != nil {
			return fmt.Errorf("read %s: %w (run 'wpstatic config init' first)", path, err)
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
		path = os.Getenv("WPSTATIC_CONFIG")
	}
	if path == "" {
		path = "wpstatic.yaml"
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
	v.SetEnvPrefix("WPSTATIC")
	return v, nil
}

const sampleConfig = `# wpstatic configuration
site:
  url: http://localhost:8081

output:
  dir: /var/www/wpstatic

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
  # secret: set via the WPSTATIC_HOOK_SECRET environment variable

user_agent: wpstatic/1.0 (+local static cache)
`
