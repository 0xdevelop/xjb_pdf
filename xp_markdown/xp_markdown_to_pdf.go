// Package xp_markdown preserves the embedded-CJK convenience entry point.
// Rendering is delegated to xp_render, including extension calls.
package xp_markdown

import (
	"context"

	"github.com/0xdevelop/xjb_pdf/xp_fonts"
	"github.com/0xdevelop/xjb_pdf/xp_render"
)

func MarkdownToPDF(ctx context.Context, markdown []byte) ([]byte, error) {
	return MarkdownToPDFWithExtensions(ctx, markdown)
}

// MarkdownToPDFWithExtensions applies caller extensions after the embedded
// font extension. Use xp_render directly to omit the bundled font assets.
func MarkdownToPDFWithExtensions(ctx context.Context, markdown []byte, extensions ...xp_render.Extension) ([]byte, error) {
	plugins := make([]xp_render.Extension, 0, len(extensions)+1)
	plugins = append(plugins, xp_fonts.EmbeddedExtension{})
	plugins = append(plugins, extensions...)
	return xp_render.MarkdownToPDF(ctx, markdown, plugins...)
}
