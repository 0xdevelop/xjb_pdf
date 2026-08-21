// Package xp_fonts owns the TrueType faces the renderer draws with and the
// registration step that binds them to a PDF writer.
//
// The faces are embedded rather than loaded at runtime: goldmark-pdf resolves
// any style slot it was not given an explicit face for to a Google-hosted font
// and downloads it while rendering. Owning every slot here is what keeps
// rendering offline.
package xp_fonts

import (
	"embed"
	"fmt"

	"github.com/stephenafamo/goldmark-pdf"
)

// embeddedFaces holds the shipped font files. They must carry glyf/loca
// outlines: the PDF writer parses TrueType tables directly and cannot read
// CFF/OpenType (.otf) or variable-font sources.
//
//go:embed *.ttf
var embeddedFaces embed.FS

const (
	// EmbeddedFamily is the family name every face is registered and selected
	// under. Callers pass it nowhere — EmbeddedFace carries it — but the
	// renderer resolves styles by family string, so registration and selection
	// must agree.
	EmbeddedFamily = "XjbPDFSans"

	faceFileRegular = "NotoSansSC-Regular.ttf"
	faceFileBold    = "NotoSansSC-Bold.ttf"
)

// RegisterEmbeddedFaces loads the embedded faces into writer for all four
// styles the renderer can select ("", "B", "I", "BI"). CJK type design carries no italic
// cut, so the italic slots reuse the upright faces; leaving a slot
// unregistered makes the writer fail the moment markdown emphasis is rendered.
//
// The PDF writer latches parse failures internally instead of returning them,
// so the latched error is read back here: a rejected font must surface as an
// error rather than as a document with blank glyphs.
func RegisterEmbeddedFaces(writer *pdf.Fpdf) error {
	regular, err := embeddedFaces.ReadFile(faceFileRegular)
	if err != nil {
		return fmt.Errorf("xp_fonts: read embedded face %s: %w", faceFileRegular, err)
	}
	bold, err := embeddedFaces.ReadFile(faceFileBold)
	if err != nil {
		return fmt.Errorf("xp_fonts: read embedded face %s: %w", faceFileBold, err)
	}

	faces := []struct {
		style string
		data  []byte
	}{
		{pdf.FontStyleRegular, regular},
		{pdf.FontStyleItalic, regular},
		{pdf.FontStyleBold, bold},
		{pdf.FontStyleBoldItalic, bold},
	}
	for _, face := range faces {
		if err := writer.AddFont(EmbeddedFamily, face.style, face.data); err != nil {
			return fmt.Errorf("xp_fonts: register style %q: %w", face.style, err)
		}
	}
	if err := writer.Fpdf.Error(); err != nil {
		return fmt.Errorf("xp_fonts: embedded face rejected by pdf writer: %w", err)
	}

	return nil
}

// EmbeddedFace is the descriptor handed to every style slot of the renderer.
// FontTypeCustom marks the face as already registered on the writer, which is
// what stops the renderer from resolving and downloading a Google font.
func EmbeddedFace() pdf.Font {
	return pdf.Font{
		CanUseForText: true,
		CanUseForCode: true,
		Family:        EmbeddedFamily,
		Type:          pdf.FontTypeCustom,
	}
}
