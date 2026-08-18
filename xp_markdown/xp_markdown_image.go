package xp_markdown

import (
	"strings"

	"github.com/stephenafamo/goldmark-pdf"
	"github.com/yuin/goldmark/ast"
)

// offlineImagePriority outranks the renderer's built-in node renderers, which
// register at 1000. The renderer sorts its node-renderer slice ascending and
// then registers it back to front, so the lowest number registers last and its
// funcs are the ones left in the dispatch table.
const offlineImagePriority = 100

// offlineImageRenderer replaces the built-in image renderer. The built-in one
// resolves an image destination through a filesystem that has an http-backed
// layer merged in unconditionally, so a markdown document could make the
// renderer issue outbound requests to any host it names.
type offlineImageRenderer struct{}

// RegisterFuncs implements pdf.NodeRenderer.
func (offlineImageRenderer) RegisterFuncs(reg pdf.NodeRendererFuncRegisterer) {
	reg.Register(ast.KindImage, renderImageOffline)
}

// renderImageOffline draws images the document already carries and drops the
// ones that would need a fetch. A dropped image leaves its slot empty; it is
// never an error, because a report that names an unreachable illustration is
// still a valid report.
func renderImageOffline(w *pdf.Writer, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}

	image, ok := node.(*ast.Image)
	if !ok {
		return ast.WalkContinue, nil
	}
	destination := string(image.Destination)
	if !strings.HasPrefix(destination, "data:image/") {
		w.LogDebug("Image (skipped, not embedded in document)", destination)
		return ast.WalkContinue, nil
	}

	file, err := w.ImageFS.Open(destination)
	if err != nil {
		w.LogDebug("Image (unreadable)", err.Error())
		return ast.WalkContinue, nil
	}
	defer file.Close()

	// Only the inline data-URI layer reports its own type. Anything else would
	// have to be sniffed from the reader, which consumes bytes the image
	// registration still needs, so it is refused rather than guessed at.
	typed, ok := file.(interface{ MimeType() string })
	if !ok {
		w.LogDebug("Image (unknown type)", destination)
		return ast.WalkContinue, nil
	}
	format := strings.TrimPrefix(typed.MimeType(), "image/")
	if format == "" {
		w.LogDebug("Image (unknown type)", destination)
		return ast.WalkContinue, nil
	}

	pageWidth, _ := w.Pdf.GetPageSize()
	marginLeft, _, marginRight, _ := w.Pdf.GetMargins()
	maxWidth := pageWidth - (marginLeft * 2) - (marginRight * 2)

	w.Pdf.RegisterImage(destination, format, file)
	w.Pdf.UseImage(destination, marginLeft*2, w.Pdf.GetY(), maxWidth, 0)

	return ast.WalkContinue, nil
}
