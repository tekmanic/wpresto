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
	Long: `Run the localhost trigger endpoint used by the bundled WordPress "go live"
plugin. Requires hook.enabled=true in the config (and a secret).

Endpoints (on hook.listen, default 127.0.0.1:9090):
  POST /go-live   with header X-WPresto-Secret: <hook.secret>
      202 {"build_id":"<hex>","status":"running"}   build accepted
      409 {"error":"build already running"}         build in flight
      401 {"error":"unauthorized"}                  bad/missing secret
  GET /status     (no auth)
      {"build_id":"...","status":"idle|running|success|error","error":"..."}

Each accepted /go-live request runs a full build (identical to
'wpresto build'). Only one build runs at a time; concurrent requests get 409.
The server runs in the foreground until interrupted — run it under systemd
or in a tmux session.

Setup:
  1. wpresto config set hook.enabled true
  2. export WPRESTO_HOOK_SECRET='a-long-random-string'
  3. wpresto hook
  4. Install the bundled plugin (plugin/wpresto-golive/wpresto-golive.php)
     into wp-content/plugins/, then set the Hook URL and Hook secret on
     Settings -> wpresto Go Live. Click "Go Live" in the admin bar (or
     "Go Live Now" on the settings page) to trigger a rebuild.`,
	Example: `  # run in the foreground (or under systemd)
  WPRESTO_HOOK_SECRET=... wpresto hook

  # trigger a build manually
  curl -X POST -H "X-WPresto-Secret: ..." http://127.0.0.1:9090/go-live
  curl http://127.0.0.1:9090/status`,
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
