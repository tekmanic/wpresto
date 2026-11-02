package plugin

import (
	"github.com/tekmanic/wpresto/internal/detect"
)

// MatchResult describes the outcome of matching one detected dynamic part to
// a plugin.
type MatchResult struct {
	// Part is the detected dynamic part.
	Part detect.DynamicPart `json:"part"`
	// Plugin is the name of the plugin that handles the part (empty if none).
	Plugin string `json:"plugin,omitempty"`
	// Handled is true when a plugin covers the part.
	Handled bool `json:"handled"`
}

// Report summarizes the dynamic-part detection and matching for a build.
type Report struct {
	// Detected is the total number of dynamic parts found.
	Detected int `json:"detected"`
	// Handled is the number of parts covered by a plugin.
	Handled int `json:"handled"`
	// Flagged is the number of parts no plugin covers.
	Flagged int `json:"flagged"`
	// Parts is the per-part outcome.
	Parts []MatchResult `json:"parts"`
}

// Match matches a set of detected dynamic parts to plugins by class and
// returns a Report. Parts whose class no plugin handles are flagged.
func Match(parts []detect.DynamicPart) *Report {
	rep := &Report{Detected: len(parts), Parts: make([]MatchResult, 0, len(parts))}
	for _, p := range parts {
		mr := MatchResult{Part: p}
		if pl, ok := FindKind(p.Class); ok {
			mr.Plugin = pl.Name()
			mr.Handled = true
			rep.Handled++
		} else {
			rep.Flagged++
		}
		rep.Parts = append(rep.Parts, mr)
	}
	return rep
}
