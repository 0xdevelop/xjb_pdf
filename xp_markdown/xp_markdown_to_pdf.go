// Package xp_markdown renders markdown source into a PDF document. It owns the
// layout pass and binds every style slot to the embedded faces from xp_fonts.
//
// The module root re-exports MarkdownToPDF under the same name; import
// this package directly only when the root facade is not wanted.
package xp_markdown

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	"github.com/0xdevelop/xjb_pdf/xp_fonts"
	"github.com/stephenafamo/goldmark-pdf"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/util"
)

// Page geometry of the rendered document. Fixed rather than configurable:
// every one of the renderer's style slots has to stay bound to the embedded
// faces, and an options struct that exposes styles is the one way a caller
// could reintroduce a downloaded font.
const (
	renderPageOrientation = "P" // portrait
	renderPageSize        = "A4"
)

// MarkdownToPDF renders CommonMark + GFM markdown into PDF bytes: the layout
// decisions — fonts, page size, margins — are package-internal, and the caller
// supplies content only.
//
// Rendering makes no network calls: the embedded CJK-capable faces are
// registered on the writer before the first draw call, and the image renderer
// is replaced by one that only draws images the document carries inline (see
// renderImageWithoutFetching). An image the markdown points at by http/https
// URL is written as its alt text linking to that address, so the picture is
// reachable from the document without the render ever contacting the host.
//
// ctx is checked before the render starts and is carried into the renderer.
// The layout pass itself is synchronous and cannot be interrupted mid-document.
func MarkdownToPDF(ctx context.Context, markdown []byte) ([]byte, error) {
	if len(bytes.TrimSpace(markdown)) == 0 {
		return nil, errors.New("xp_markdown: empty markdown input")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	writer := pdf.NewFpdf(ctx, pdf.FpdfConfig{
		Orientation: renderPageOrientation,
		PaperSize:   renderPageSize,
	}, nil)
	if err := xp_fonts.RegisterEmbeddedFaces(writer); err != nil {
		return nil, err
	}

	// All three options together cover the renderer's eleven style slots:
	// headings plus table header, body plus blockquote plus table body, and
	// the code face. A slot left at its default resolves to a Google font.
	face := xp_fonts.EmbeddedFace()
	engine := goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		goldmark.WithRenderer(pdf.New(
			pdf.WithContext(ctx),
			pdf.WithPDF(writer),
			pdf.WithHeadingFont(face),
			pdf.WithBodyFont(face),
			pdf.WithCodeFont(face),
			pdf.WithNodeRenderers(util.Prioritized(noFetchImageRenderer{}, imageRendererPriority)),
		)),
	)

	var out bytes.Buffer
	if err := engine.Convert(markdown, &out); err != nil {
		return nil, fmt.Errorf("xp_markdown: render markdown: %w", err)
	}
	if err := writer.Fpdf.Error(); err != nil {
		return nil, fmt.Errorf("xp_markdown: pdf writer: %w", err)
	}

	return out.Bytes(), nil
}
