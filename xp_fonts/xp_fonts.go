// Package xp_fonts supplies the optional embedded Noto Sans SC font extension.
package xp_fonts

import (
	"embed"
	"fmt"

	"github.com/0xdevelop/xjb_pdf/xp_render"
	pdf "github.com/stephenafamo/goldmark-pdf"
)

//go:embed *.ttf
var embeddedFaces embed.FS

const EmbeddedFamily = "XjbPDFSans"

// EmbeddedFontFaces returns independent copies of the bundled offline TrueType faces.
func EmbeddedFontFaces() (xp_render.FontFaces, error) {
	regular, err := embeddedFaces.ReadFile("NotoSansSC-Regular.ttf")
	if err != nil {
		return xp_render.FontFaces{}, fmt.Errorf("xp_fonts: read regular face: %w", err)
	}
	bold, err := embeddedFaces.ReadFile("NotoSansSC-Bold.ttf")
	if err != nil {
		return xp_render.FontFaces{}, fmt.Errorf("xp_fonts: read bold face: %w", err)
	}
	return xp_render.FontFaces{Family: EmbeddedFamily, Regular: regular, Bold: bold}, nil
}

// EmbeddedExtension binds every style slot to the bundled offline CJK faces.
type EmbeddedExtension struct{}

func (EmbeddedExtension) Extend(c *xp_render.Config) error {
	faces, err := EmbeddedFontFaces()
	if err != nil {
		return err
	}
	return faces.Extend(c)
}

func RegisterEmbeddedFaces(writer *pdf.Fpdf) error {
	faces, err := EmbeddedFontFaces()
	if err != nil {
		return err
	}
	return faces.Register(writer)
}

func EmbeddedFace() pdf.Font { return xp_render.FontFaces{Family: EmbeddedFamily}.Face() }
