// Package xp_render owns the shared Markdown-to-PDF pipeline. Font assets and
// application layouts are supplied by extensions rather than embedded here.
package xp_render

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	pdf "github.com/stephenafamo/goldmark-pdf"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/util"
)

// Extension configures one render. Instances should not retain its mutable Config.
type Extension interface {
	Extend(*Config) error
}

type ExtensionFunc func(*Config) error

func (f ExtensionFunc) Extend(c *Config) error { return f(c) }

// Config belongs to a single call. Prepare runs after writer creation and before
// Markdown layout, in extension order; it can register fonts or draw a cover.
type Config struct {
	Tables        TableLayout
	Page          pdf.FpdfConfig
	Styles        pdf.Styles
	NodeRenderers util.PrioritizedSlice
	Prepare       []func(context.Context, *pdf.Fpdf) error
}

func defaultConfig() Config {
	c := Config{Page: pdf.FpdfConfig{Orientation: "P", PaperSize: "A4"}, Styles: pdf.DefaultStyles()}
	setFonts(&c.Styles, pdf.FontHelvetica, pdf.FontCourier)
	return c
}

// MarkdownToPDF renders CommonMark + GFM without fetching fonts or images.
// The default uses PDF's built-in Latin fonts; CJK requires a font extension.
func MarkdownToPDF(ctx context.Context, markdown []byte, extensions ...Extension) ([]byte, error) {
	if len(bytes.TrimSpace(markdown)) == 0 {
		return nil, errors.New("xp_render: empty markdown input")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	c := defaultConfig()
	for i, e := range extensions {
		if e == nil {
			return nil, fmt.Errorf("xp_render: nil extension at %d", i)
		}
		if err := e.Extend(&c); err != nil {
			return nil, fmt.Errorf("xp_render: extension %d: %w", i, err)
		}
	}
	if c.Tables.LandscapeMinColumns < 0 || c.Tables.LandscapeMinColumns == 1 {
		return nil, errors.New("xp_render: invalid landscape column threshold")
	}
	if err := validateStyles(c.Styles); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	writer := pdf.NewFpdf(ctx, c.Page, nil)
	for i, prepare := range c.Prepare {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if prepare == nil {
			return nil, fmt.Errorf("xp_render: nil prepare hook at %d", i)
		}
		if err := prepare(ctx, writer); err != nil {
			return nil, fmt.Errorf("xp_render: prepare %d: %w", i, err)
		}
	}
	if err := writer.Fpdf.Error(); err != nil {
		return nil, fmt.Errorf("xp_render: prepare writer: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	config := pdf.DefaultConfig()
	config.Context, config.PDF, config.Styles = ctx, writer, c.Styles
	config.NodeRenderers = append(config.NodeRenderers, util.Prioritized(noFetchImageRenderer{}, imageRendererPriority), util.Prioritized(tableRenderer{ctx: ctx, layout: c.Tables, baseOrientation: c.Page.Orientation}, imageRendererPriority))
	config.NodeRenderers = append(config.NodeRenderers, c.NodeRenderers...)
	engine := goldmark.New(goldmark.WithExtensions(extension.GFM), goldmark.WithRenderer(pdf.New(pdf.WithConfig(config))))
	var out bytes.Buffer
	if err := engine.Convert(markdown, &out); err != nil {
		return nil, fmt.Errorf("xp_render: render markdown: %w", err)
	}
	if err := writer.Fpdf.Error(); err != nil {
		return nil, fmt.Errorf("xp_render: pdf writer: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func textStyles(s pdf.Styles) []*pdf.Style {
	return []*pdf.Style{s.H1, s.H2, s.H3, s.H4, s.H5, s.H6, s.Normal, s.Blockquote, s.THeader, s.TBody}
}

func setFonts(s *pdf.Styles, text, code pdf.Font) {
	for _, style := range textStyles(*s) {
		style.Font = text
	}
	s.CodeFont = code
}

func validateStyles(s pdf.Styles) error {
	fonts := []pdf.Font{s.CodeFont}
	for i, style := range textStyles(s) {
		if style == nil || style.Size <= 0 || style.Size+style.Spacing <= 0 || style.TextColor == nil || style.FillColor == nil {
			return fmt.Errorf("xp_render: invalid style at %d", i)
		}
		fonts = append(fonts, style.Font)
	}
	for _, font := range fonts {
		if font.Family == "" || (font.Type != pdf.FontTypeCustom && font.Type != pdf.FontTypeInbuilt) {
			return errors.New("xp_render: each font must be registered locally or PDF built-in")
		}
	}
	return nil
}
