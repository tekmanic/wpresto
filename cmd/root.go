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
	Short: "Scrape a local WordPress site into a minified static cache for nginx",
	Long: `wpresto crawls a WordPress site as an anonymous user, strips sensitive
admin/login/API/plugin content, minifies and compresses the result, and
publishes it atomically to a static filesystem served by nginx.

Trigger a build from cron, or from WordPress via the bundled "go live" plugin
(which calls the localhost hook endpoint).`,
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
