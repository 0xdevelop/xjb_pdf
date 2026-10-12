package xp_render

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"

	pdf "github.com/stephenafamo/goldmark-pdf"
)

// FontFaces is an offline extension for static TrueType data supplied by the
// caller. Missing italic faces reuse their upright counterparts.
type FontFaces struct {
	Family                            string
	Regular, Bold, Italic, BoldItalic []byte
}

func (f FontFaces) Extend(c *Config) error {
	if f.Family == "" || len(f.Regular) == 0 || len(f.Bold) == 0 {
		return errors.New("xp_render: font family, regular and bold faces are required")
	}
	setFonts(&c.Styles, f.Face(), f.Face())
	c.Prepare = append(c.Prepare, func(_ context.Context, writer *pdf.Fpdf) error { return f.Register(writer) })
	return nil
}

func (f FontFaces) Face() pdf.Font {
	return pdf.Font{CanUseForText: true, CanUseForCode: true, Family: f.Family, Type: pdf.FontTypeCustom}
}

func (f FontFaces) Register(writer *pdf.Fpdf) (err error) {
	defer func() {
		if failure := recover(); failure != nil {
			err = fmt.Errorf("xp_render: rejected TrueType font: %v", failure)
		}
	}()
	if f.Family == "" || len(f.Regular) == 0 || len(f.Bold) == 0 {
		return errors.New("xp_render: font family, regular and bold faces are required")
	}
	if len(f.Italic) == 0 {
		f.Italic = f.Regular
	}
	if len(f.BoldItalic) == 0 {
		f.BoldItalic = f.Bold
	}
	for _, face := range []struct {
		style string
		data  []byte
	}{
		{pdf.FontStyleRegular, f.Regular}, {pdf.FontStyleItalic, f.Italic},
		{pdf.FontStyleBold, f.Bold}, {pdf.FontStyleBoldItalic, f.BoldItalic},
	} {
		if err := validateTTF(face.data); err != nil {
			return fmt.Errorf("xp_render: font %q: %w", face.style, err)
		}
		if err := writer.AddFont(f.Family, face.style, face.data); err != nil {
			return fmt.Errorf("xp_render: register font %q: %w", face.style, err)
		}
		writer.Fpdf.SetFont(f.Family, face.style, 12)
		if err := writer.Fpdf.Error(); err != nil {
			return fmt.Errorf("xp_render: rejected font %q: %w", face.style, err)
		}
	}
	return nil
}

func validateTTF(data []byte) error {
	if len(data) < 12 || binary.BigEndian.Uint32(data[:4]) != 0x00010000 {
		return errors.New("static TrueType font required")
	}
	count := int(binary.BigEndian.Uint16(data[4:6]))
	if count == 0 || 12+16*count > len(data) {
		return errors.New("invalid TrueType table directory")
	}
	tables := make(map[string]bool, count)
	for i := 0; i < count; i++ {
		entry := data[12+16*i : 12+16*(i+1)]
		tag := string(entry[:4])
		offset, length := uint64(binary.BigEndian.Uint32(entry[8:12])), uint64(binary.BigEndian.Uint32(entry[12:16]))
		if offset+length > uint64(len(data)) {
			return errors.New("TrueType table exceeds font data")
		}
		if tag == "fvar" {
			return errors.New("variable fonts are not supported")
		}
		tables[tag] = true
	}
	for _, tag := range []string{"glyf", "loca", "head", "hhea", "hmtx", "maxp", "cmap"} {
		if !tables[tag] {
			return fmt.Errorf("missing TrueType table %s", tag)
		}
	}
	return nil
}
