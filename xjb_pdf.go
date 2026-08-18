// Package xjb_pdf is the module facade: it re-exports the capabilities that
// live in the xp_* packages so callers have a single import.
package xjb_pdf

import (
	"context"

	"github.com/0xdevelop/xjb_pdf/xp_config"
	"github.com/0xdevelop/xjb_pdf/xp_markdown"
)

func GetVersion() string {
	return xp_config.ProjectVersion
}

// RenderMarkdown renders CommonMark + GFM markdown into PDF bytes. Fonts, page
// size and margins are fixed inside the module; the caller supplies content
// only. See xp_markdown.Render for the behaviour this delegates to.
func RenderMarkdown(ctx context.Context, markdown []byte) ([]byte, error) {
	return xp_markdown.Render(ctx, markdown)
}
