package xp_render_test

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"image/color"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/0xdevelop/xjb_pdf/xp_fonts"
	"github.com/0xdevelop/xjb_pdf/xp_render"
	pdf "github.com/stephenafamo/goldmark-pdf"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/util"
)

var artifactOutput = flag.String("pdf-output", "", "optional path for visual review of the extension fixture")

func TestCoreAndExtensionFailures(t *testing.T) {
	ctx := context.Background()
	out, err := xp_render.MarkdownToPDF(ctx, []byte("# Latin document\n\nPlain text."))
	if err != nil || !bytes.HasPrefix(out, []byte("%PDF")) || bytes.Contains(out, []byte("/Type0")) {
		t.Fatalf("built-in font render: err=%v", err)
	}
	want := errors.New("extension error")
	_, err = xp_render.MarkdownToPDF(ctx, []byte("text"), xp_render.ExtensionFunc(func(*xp_render.Config) error { return want }))
	if !errors.Is(err, want) {
		t.Fatalf("extension error lost: %v", err)
	}
	_, err = xp_render.MarkdownToPDF(ctx, []byte("text"), xp_render.ExtensionFunc(func(c *xp_render.Config) error {
		c.Styles.H3.Font = pdf.FontRoboto
		return nil
	}))
	if err == nil {
		t.Fatal("remote font slot accepted")
	}
	want = errors.New("prepare error")
	_, err = xp_render.MarkdownToPDF(ctx, []byte("text"), xp_render.ExtensionFunc(func(c *xp_render.Config) error {
		c.Prepare = append(c.Prepare, func(context.Context, *pdf.Fpdf) error { return want })
		return nil
	}))
	if !errors.Is(err, want) {
		t.Fatalf("prepare error lost: %v", err)
	}
	_, err = xp_render.MarkdownToPDF(ctx, []byte("text"), xp_render.FontFaces{Family: "Broken", Regular: []byte("bad"), Bold: []byte("bad")})
	if err == nil {
		t.Fatal("invalid font accepted")
	}
	ctx, cancel := context.WithCancel(ctx)
	_, err = xp_render.MarkdownToPDF(ctx, []byte("text"), xp_render.ExtensionFunc(func(c *xp_render.Config) error {
		c.Prepare = append(c.Prepare, func(context.Context, *pdf.Fpdf) error { cancel(); return nil })
		return nil
	}))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel lost: %v", err)
	}
}

type headingExtension struct{}

func (headingExtension) RegisterFuncs(reg pdf.NodeRendererFuncRegisterer) {
	reg.Register(ast.KindHeading, func(w *pdf.Writer, _ []byte, _ ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		pdf.SetStyle(w.Pdf, *w.Styles.H1)
		w.Pdf.WriteText(20, "CUSTOM_HEADING")
		w.Pdf.BR(20)
		return ast.WalkSkipChildren, nil
	})
}

func TestNodeExtensionAndConfigIsolation(t *testing.T) {
	plugin := xp_render.ExtensionFunc(func(c *xp_render.Config) error {
		c.Page.Orientation = "L"
		c.NodeRenderers = append(c.NodeRenderers, util.Prioritized(headingExtension{}, 10))
		c.Prepare = append(c.Prepare, func(_ context.Context, w *pdf.Fpdf) error { w.Fpdf.SetCompression(false); return nil })
		return nil
	})
	out, err := xp_render.MarkdownToPDF(context.Background(), []byte("# ORIGINAL_HEADING\n\nBody."), plugin)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(out, []byte("CUSTOM_HEADING")) || bytes.Contains(out, []byte("ORIGINAL_HEADING")) || !bytes.Contains(out, []byte("/MediaBox [0 0 841.89 595.28]")) {
		t.Fatal("node/page extension not applied")
	}
	out, err = xp_render.MarkdownToPDF(context.Background(), []byte("# Ordinary heading"))
	if err != nil || !bytes.Contains(out, []byte("/MediaBox [0 0 595.28 841.89]")) {
		t.Fatalf("config leaked between calls: %v", err)
	}
}

func TestConcurrentTables(t *testing.T) {
	const count = 8
	var wg sync.WaitGroup
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := fmt.Sprintf("OWNER_%02d", i)
			var doc strings.Builder
			doc.WriteString("| OWNER_HEADER | VALUE_HEADER |\n|---|---|\n")
			for n := 0; n < 140; n++ {
				fmt.Fprintf(&doc, "| %s | ROW_%03d |\n", id, n)
			}
			out, err := xp_render.MarkdownToPDF(context.Background(), []byte(doc.String()), xp_render.ExtensionFunc(func(c *xp_render.Config) error {
				c.Prepare = append(c.Prepare, func(_ context.Context, w *pdf.Fpdf) error { w.Fpdf.SetCompression(false); return nil })
				return nil
			}))
			if err != nil {
				t.Errorf("render %d: %v", i, err)
				return
			}
			if !bytes.Contains(out, []byte(id)) || !bytes.Contains(out, []byte("ROW_139")) || bytes.Count(out, []byte("OWNER_HEADER")) < 2 {
				t.Errorf("missing rows/repeated headers: %d", i)
			}
			for other := 0; other < count; other++ {
				if other != i && bytes.Contains(out, []byte(fmt.Sprintf("OWNER_%02d", other))) {
					t.Errorf("table contents crossed documents %d/%d", i, other)
				}
			}
		}(i)
	}
	wg.Wait()
}

func TestCJKLayoutExtension(t *testing.T) {
	faces, err := xp_fonts.EmbeddedFontFaces()
	if err != nil || faces.Family != xp_fonts.EmbeddedFamily || len(faces.Regular) < 12 || len(faces.Bold) < 12 {
		t.Fatal("embedded font data unavailable", err)
	}
	other, err := xp_fonts.EmbeddedFontFaces()
	if err != nil || !bytes.Equal(faces.Regular, other.Regular) || !bytes.Equal(faces.Bold, other.Bold) {
		t.Fatal("font bytes changed between consumers", err)
	}
	other.Regular[0] ^= 0xff
	other.Bold[0] ^= 0xff
	if bytes.Equal(faces.Regular, other.Regular) || bytes.Equal(faces.Bold, other.Bold) {
		t.Fatal("consumers share mutable font bytes")
	}
	plugin := xp_render.ExtensionFunc(func(c *xp_render.Config) error {
		c.Page.Title = "通用插件渲染验收"
		c.Styles.Normal.Size, c.Styles.Normal.Spacing = 10, 7
		c.Styles.THeader.Size, c.Styles.THeader.Spacing = 9, 5
		c.Styles.TBody.Size, c.Styles.TBody.Spacing = 9, 5
		c.Styles.H1.TextColor = color.RGBA{R: 107, B: 161, A: 255}
		c.Prepare = append(c.Prepare, func(_ context.Context, w *pdf.Fpdf) error {
			f := w.Fpdf
			f.SetMargins(40, 45, 40)
			f.SetAutoPageBreak(true, 40)
			f.SetFillColor(7, 6, 11)
			width, height := f.GetPageSize()
			f.Rect(0, 0, width, height, "F")
			f.SetFont(xp_fonts.EmbeddedFamily, "B", 30)
			f.SetTextColor(255, 255, 255)
			f.SetXY(40, 180)
			f.MultiCell(width-80, 45, "通用插件渲染验收\n中文字体与自定义封面", "", "L", false)
			f.SetFooterFunc(func() {
				f.SetY(-30)
				f.SetFont(xp_fonts.EmbeddedFamily, "", 8)
				f.SetTextColor(90, 90, 90)
				f.CellFormat(0, 12, fmt.Sprintf("插件页脚 · 第 %d 页", f.PageNo()), "", 0, "C", false, 0, "")
			})
			f.AddPage()
			return nil
		})
		return nil
	})
	var doc strings.Builder
	doc.WriteString("# 中文正文与表格\n\n这是字体、版式和页面扩展的验收样本。扩展本身由调用项目提供，不属于渲染器内置品牌。\n\n| 编号 | 内容 | 结论 |\n|---|---|---|\n")
	for i := 0; i < 100; i++ {
		fmt.Fprintf(&doc, "| CASE-%03d | %s | 数据完整，等待业务审查。 |\n", i, strings.Repeat("中文表格换行及跨页检查：每个单元格保持自己的列位置。", 1+i%3))
	}
	out, err := xp_render.MarkdownToPDF(context.Background(), []byte(doc.String()), faces, plugin)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(out, []byte("/Type0")) {
		t.Fatal("CJK face not embedded")
	}
	if *artifactOutput != "" {
		if err := os.WriteFile(*artifactOutput, out, 0600); err != nil {
			t.Fatal(err)
		}
	}
}
