package cmd

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("wpresto %s (%s %s %s)\n", Version, runtime.GOOS, runtime.GOARCH, runtime.Version())
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
