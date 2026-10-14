package xp_render

import pdf "github.com/stephenafamo/goldmark-pdf"

// TableLayout configures the existing table renderer for one document.
// Zero values retain the original layout; application thresholds and styles
// belong to the caller. Configure runs before measuring each table.
type TableLayout struct {
	LandscapeMinColumns int
	KeepRowsTogether    bool
	Configure           func(*pdf.Writer, *pdf.TableData) error
}
