package xp_markdown

import (
	"bytes"
	"io"
	"math"
	"strings"

	"github.com/stephenafamo/goldmark-pdf"
	"github.com/yuin/goldmark/ast"
)

// dataURIImagePrefix marks the one image form that is part of the document
// itself: the bytes travel inside the markdown, so drawing them needs no I/O.
const dataURIImagePrefix = "data:image/"

// imageRendererPriority outranks the renderer's built-in node renderers, which
// register at 1000. The renderer sorts its node-renderer slice ascending and
// then registers it back to front, so the lowest number registers last and its
// funcs are the ones left in the dispatch table.
const imageRendererPriority = 100

// noFetchImageRenderer replaces the built-in image renderer. The built-in one
// resolves an image destination through a filesystem that has an http-backed
// layer merged in unconditionally, so a markdown document could make the
// renderer issue outbound requests to any host it names.
type noFetchImageRenderer struct{}

// RegisterFuncs implements pdf.NodeRenderer.
func (noFetchImageRenderer) RegisterFuncs(reg pdf.NodeRendererFuncRegisterer) {
	reg.Register(ast.KindImage, renderImageWithoutFetching)
}

// renderImageWithoutFetching draws images the document already carries and
// turns the rest into text, never into a request. An http/https destination
// becomes a clickable link: PDF has no mechanism that displays a picture from
// a URL at viewing time, so the honest rendering of a remote image is its alt
// text pointing at where the picture lives. Anything else — a relative path, a
// scheme this renderer will not put in a link annotation — falls back to plain
// alt text.
func renderImageWithoutFetching(w *pdf.Writer, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}

	image, ok := node.(*ast.Image)
	if !ok {
		return ast.WalkContinue, nil
	}

	destination := string(image.Destination)
	switch {
	case strings.HasPrefix(destination, dataURIImagePrefix):
		if drawEmbeddedImage(w, destination) {
			return ast.WalkSkipChildren, nil
		}
	case isLinkableRemoteImage(destination):
		writeRemoteImageAsLink(w, imageAltText(image, source), destination)
		return ast.WalkSkipChildren, nil
	}

	// Nothing was drawn or linked, so the child text nodes stay in the walk
	// and the alt text takes the image's place in the flow.
	w.LogDebug("Image (alt text only)", destination)

	return ast.WalkContinue, nil
}

// drawEmbeddedImage places the picture a data URI carries onto the page and
// reports whether it made it there. A destination that decodes to something
// the PDF writer refuses — a truncated PNG, an interlaced one, an SVG payload —
// is not an error: the document still renders, with alt text where the picture
// would have been.
func drawEmbeddedImage(w *pdf.Writer, dataURI string) bool {
	// The writer latches failures internally instead of returning them, so a
	// refused image would otherwise surface as a failed render several nodes
	// later. Reading the latch around the registration keeps the failure local
	// to this image; a latch that was already set belongs to an earlier
	// failure and must be left for Render to report.
	writer, ok := w.Pdf.(*pdf.Fpdf)
	if !ok || writer.Fpdf.Error() != nil {
		return false
	}

	data, format, ok := readEmbeddedImage(w, dataURI)
	if !ok {
		return false
	}

	w.Pdf.RegisterImage(dataURI, format, bytes.NewReader(data))
	if err := writer.Fpdf.Error(); err != nil {
		writer.Fpdf.ClearError()
		w.LogDebug("Image (rejected by pdf writer)", err.Error())
		return false
	}
	info := writer.Fpdf.GetImageInfo(dataURI)
	if info == nil {
		return false
	}
	width, height := info.Extent()
	if width <= 0 || height <= 0 {
		return false
	}

	pageWidth, pageHeight := w.Pdf.GetPageSize()
	marginLeft, marginTop, marginRight, marginBottom := w.Pdf.GetMargins()
	maxWidth := pageWidth - marginLeft - marginRight
	maxHeight := pageHeight - marginTop - marginBottom

	// Shrink to fit, never enlarge: an icon authored at 16px is meant to be an
	// icon, and blowing it up to the text column would be a rendering decision
	// the document never asked for.
	if scale := math.Min(maxWidth/width, maxHeight/height); scale < 1 {
		width, height = width*scale, height*scale
	}

	// The image is drawn from the current position downwards, so a line still
	// holding text has to be closed first or the picture lands on top of it.
	lineHeight := w.Styles.Normal.Size + w.Styles.Normal.Spacing
	if w.Pdf.GetX() > marginLeft {
		w.Pdf.BR(lineHeight)
	}
	w.Pdf.UseImage(dataURI, marginLeft, w.Pdf.GetY(), width, height)
	w.Pdf.BR(lineHeight)

	return true
}

// readEmbeddedImage returns the decoded bytes of a data URI together with the
// image format the PDF writer needs to be told.
func readEmbeddedImage(w *pdf.Writer, dataURI string) ([]byte, string, bool) {
	file, err := w.ImageFS.Open(dataURI)
	if err != nil {
		w.LogDebug("Image (unreadable)", err.Error())
		return nil, "", false
	}
	defer file.Close()

	// Only the inline data-URI layer reports its own type. Anything else would
	// have to be sniffed from the reader, which consumes bytes the image
	// registration still needs, so it is refused rather than guessed at.
	typed, ok := file.(interface{ MimeType() string })
	if !ok {
		w.LogDebug("Image (unknown type)", dataURI)
		return nil, "", false
	}
	format := strings.TrimPrefix(typed.MimeType(), "image/")
	if format == "" {
		w.LogDebug("Image (unknown type)", dataURI)
		return nil, "", false
	}

	data, err := io.ReadAll(file)
	if err != nil {
		w.LogDebug("Image (unreadable)", err.Error())
		return nil, "", false
	}

	return data, format, true
}

// writeRemoteImageAsLink writes the alt text as a link annotation pointing at
// the image. Clicking it opens the address in the reader's browser; the
// picture is not in the document and rendering it never contacted the host.
func writeRemoteImageAsLink(w *pdf.Writer, altText, destination string) {
	label := altText
	if label == "" {
		label = destination
	}

	style := *w.GetLinkStyle()
	pdf.SetStyle(w.Pdf, style)
	w.Pdf.WriteExternalLink(style.Size+style.Spacing, label, destination)
	w.LogDebug("Image (not in document, written as link)", destination)
}

// isLinkableRemoteImage reports whether a destination may become a link
// annotation. Only http and https qualify: markdown is caller-supplied content,
// and a scheme like javascript: would otherwise let a document plant an action
// that some readers offer to run.
func isLinkableRemoteImage(destination string) bool {
	lowered := strings.ToLower(destination)

	return strings.HasPrefix(lowered, "http://") || strings.HasPrefix(lowered, "https://")
}

// imageAltText collects the literal text under an image node. Node.Text is
// deprecated upstream, and the children of an image are the plain text and
// string nodes the alt text was parsed into.
func imageAltText(image *ast.Image, source []byte) string {
	var text strings.Builder
	for child := image.FirstChild(); child != nil; child = child.NextSibling() {
		switch node := child.(type) {
		case *ast.Text:
			text.Write(node.Segment.Value(source))
		case *ast.String:
			text.Write(node.Value)
		}
	}

	return text.String()
}
