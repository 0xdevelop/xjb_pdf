// Package xjb_pdf is the module facade: it re-exports the capabilities that
// live in the xp_* packages so callers have a single import.
package xjb_pdf

import (
	"context"

	"github.com/0xdevelop/xjb_pdf/xp_config"
	"github.com/0xdevelop/xjb_pdf/xp_markdown"
	"github.com/0xdevelop/xjb_pdf/xp_render"
)

func ModuleVersion() string {
	return xp_config.ProjectVersion
}

// MarkdownToPDF renders CommonMark + GFM markdown into PDF bytes. Fonts, page
// size and margins are fixed inside the module; the caller supplies content
// only. See xp_markdown.MarkdownToPDF for the behaviour this delegates to.
func MarkdownToPDF(ctx context.Context, markdown []byte) ([]byte, error) {
	return xp_markdown.MarkdownToPDF(ctx, markdown)
}

// MarkdownToPDFWithExtensions preserves embedded CJK defaults while applying caller plugins.
// Import xp_render directly to omit the bundled font assets.
func MarkdownToPDFWithExtensions(ctx context.Context, markdown []byte, extensions ...xp_render.Extension) ([]byte, error) {
	return xp_markdown.MarkdownToPDFWithExtensions(ctx, markdown, extensions...)
}
