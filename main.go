// wpresto scrapes a local WordPress site into a minified, compressed static
// cache that nginx serves. Sensitive admin/login/API/plugin content is
// stripped. Builds are triggered by cron or by the WordPress "go live" plugin.
package main

import "github.com/tekmanic/wpresto/cmd"

func main() {
	cmd.Execute()
}
