package xp_inspect_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"

	"github.com/0xdevelop/xjb_pdf/xp_fonts"
	"github.com/0xdevelop/xjb_pdf/xp_inspect"
	"github.com/0xdevelop/xjb_pdf/xp_render"
	pdf "github.com/stephenafamo/goldmark-pdf"
)

func TestRenderedUnicodeAndPaintedImage(t *testing.T) {
	source := image.NewNRGBA(image.Rect(0, 0, 640, 360))
	expected := make([]byte, 0, 640*360*3)
	for y := 0; y < 360; y++ {
		for x := 0; x < 640; x++ {
			v := color.NRGBA{uint8(x % 256), uint8(y % 256), 123, 255}
			source.SetNRGBA(x, y, v)
			expected = append(expected, v.R, v.G, v.B)
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, source); err != nil {
		t.Fatal(err)
	}
	raw, err := xp_render.MarkdownToPDF(context.Background(), []byte("# 中文标题\n\n跨字节中文：靶点、归一、事实、证据账本、已知问题；CLAIM-001。\n\n![图](data:image/png;base64,"+base64.StdEncoding.EncodeToString(b.Bytes())+")"), xp_fonts.EmbeddedExtension{}, xp_render.ExtensionFunc(func(c *xp_render.Config) error {
		c.Page.Title = "检查产物"
		c.Prepare = append(c.Prepare, func(_ context.Context, w *pdf.Fpdf) error {
			unused := image.NewNRGBA(image.Rect(0, 0, 1, 1))
			unused.SetNRGBA(0, 0, color.NRGBA{1, 2, 3, 255})
			var unusedPNG bytes.Buffer
			if err := png.Encode(&unusedPNG, unused); err != nil {
				return err
			}
			w.Fpdf.RegisterImageReader("unused", "PNG", bytes.NewReader(unusedPNG.Bytes()))
			return w.Fpdf.Error()
		})
		return nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(raw, []byte("/Predictor 15")) {
		t.Fatal("fixture did not exercise Predictor 15")
	}
	decoded, err := xp_inspect.Inspect(context.Background(), raw, xp_inspect.ExtensionFunc(func(c *xp_inspect.Config) error { c.DecodeImages = true; return nil }))
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Title != "检查产物" {
		t.Fatal("metadata lost")
	}
	text := ""
	images := 0
	for _, p := range decoded.Pages {
		text += p.Text
		for _, font := range p.Fonts {
			if !font.Embedded {
				t.Fatal("embedded font not detected")
			}
		}
		for _, im := range p.Images {
			images++
			if im.Width != 640 || im.Height != 360 || im.ColorSpace != "DeviceRGB" || !bytes.Equal(im.Samples, expected) {
				t.Fatal("painted image pixels differ")
			}
		}
	}
	compact := strings.NewReplacer("\n", "", " ", "").Replace(text)
	for _, s := range []string{"中文标题", "跨字节中文：靶点、归一、事实、证据账本、已知问题；CLAIM-001。"} {
		if !strings.Contains(compact, s) {
			t.Fatal("Unicode text lost", s)
		}
	}
	if images != 1 {
		t.Fatal("unused resource counted as painted image", images)
	}
	plain, err := xp_inspect.Inspect(context.Background(), raw)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range plain.Pages {
		for _, im := range p.Images {
			if im.Samples != nil {
				t.Fatal("image decode option leaked")
			}
		}
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = xp_inspect.Inspect(canceled, raw); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation lost", err)
	}
	for _, bad := range [][]byte{nil, []byte("not pdf"), raw[:len(raw)/2]} {
		if _, err = xp_inspect.Inspect(context.Background(), bad); err == nil {
			t.Fatal("malformed PDF accepted")
		}
	}
	if _, err = xp_inspect.Inspect(context.Background(), raw, xp_inspect.ExtensionFunc(func(c *xp_inspect.Config) error { c.MaxPDFBytes = 10; return nil })); err == nil {
		t.Fatal("input limit ignored")
	}
	if _, err = xp_inspect.Inspect(context.Background(), raw, xp_inspect.ExtensionFunc(func(c *xp_inspect.Config) error { c.MaxStreamBytes = 10; return nil })); err == nil {
		t.Fatal("stream limit ignored")
	}
}

func TestLatinInspectAndExtensionFailure(t *testing.T) {
	raw, err := xp_render.MarkdownToPDF(context.Background(), []byte("# Latin document\n\nReadable text."))
	if err != nil {
		t.Fatal(err)
	}
	doc, err := xp_inspect.Inspect(context.Background(), raw)
	if err != nil || len(doc.Pages) != 1 || !strings.Contains(strings.Join(strings.Fields(doc.Pages[0].Text), ""), "Readabletext.") {
		t.Fatal("Latin extraction failed", err)
	}
	want := errors.New("extension error")
	if _, err = xp_inspect.Inspect(context.Background(), raw, xp_inspect.ExtensionFunc(func(*xp_inspect.Config) error { return want })); !errors.Is(err, want) {
		t.Fatal("extension error lost")
	}
	if _, err = xp_inspect.Inspect(context.Background(), raw, nil); err == nil {
		t.Fatal("nil extension accepted")
	}
}
