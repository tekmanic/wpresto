package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/tekmanic/wpresto/internal/hook"
)

var hookCmd = &cobra.Command{
	Use:   "hook",
	Short: "Run the localhost trigger endpoint for the WordPress go-live plugin",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := loadConfig()
		if err != nil {
			return err
		}
		if !cfg.Hook.Enabled {
			return fmt.Errorf("hook is disabled (set hook.enabled=true)")
		}
		buildFn := func(ctx context.Context, buildID string) error {
			return RunBuild(ctx, cfg)
		}
		srv := hook.New(cfg.Hook.Listen, cfg.Hook.Secret, buildFn)
		fmt.Printf("wpresto: hook listening on %s\n", cfg.Hook.Listen)
		return srv.ServeHTTP(cmd.Context())
	},
}

func init() {
	rootCmd.AddCommand(hookCmd)
}
