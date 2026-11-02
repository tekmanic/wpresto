// Package inventory is the wpresto plugin inventory. It blank-imports every
// plugin so their init() functions register them with the plugin registry at
// startup. Adding a new plugin means: (1) create its package under
// inventory/<name>/, (2) add a blank import here. The plugin is then compiled
// into the wpresto binary and available to the build (for detection/matching)
// and to 'wpresto serve' (for serving).
package inventory

import (
	// Plugin inventory — each blank import registers the plugin.
	_ "github.com/tekmanic/wpresto/inventory/analytics"
	_ "github.com/tekmanic/wpresto/inventory/contactform"
	_ "github.com/tekmanic/wpresto/inventory/geoip"
	_ "github.com/tekmanic/wpresto/inventory/subscribe"
	_ "github.com/tekmanic/wpresto/inventory/urlrewrite"
)
