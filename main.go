// wpresto scrapes a local WordPress site into a minified, compressed static
// cache that nginx serves. Sensitive admin/login/API/plugin content is
// stripped. Builds are triggered by cron or by the WordPress "go live" plugin.
//
// The plugin inventory is blank-imported so its plugins register at startup;
// the build uses them to detect/replace dynamic parts and 'wpresto serve'
// runs them in a simple HTTP server.
package main

import (
	"github.com/tekmanic/wpresto/cmd"
	_ "github.com/tekmanic/wpresto/inventory"
)

func main() {
	cmd.Execute()
}
