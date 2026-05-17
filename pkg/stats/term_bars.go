package stats

import "strings"

// renderBar draws a fixed-width horizontal score bar. On TTY the bar uses
// unicode block characters and is colored via the supplied styles; on Ascii
// it falls back to "[####    ]" which survives piping/grep cleanly.
//
// pct is in [0, 100]. zoneClass is the CSS-style class produced by
// zoneClass() (zone-hot / zone-warm / zone-ok / zone-none) and drives the
// fill color.
func renderBar(s *styles, pct int, width int, zoneClass string) string {
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	if width < 4 {
		width = 4
	}

	filled := (pct * width) / 100

	if !s.isTTY {
		return "[" + strings.Repeat("#", filled) + strings.Repeat(" ", width-filled) + "]"
	}

	fill := s.barStyle(zoneClass).Render(strings.Repeat("▇", filled))
	track := s.barTrack.Render(strings.Repeat("░", width-filled))
	return fill + track
}
