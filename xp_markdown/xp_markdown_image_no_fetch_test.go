package xp_markdown

import (
	"bytes"
	"compress/zlib"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"sync/atomic"
	"testing"
)

// 1x1 PNG, the smallest thing that proves an inline image reached the page.
const inlinePNGDataURI = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=="

// drawnImageBox matches the placement operator gofpdf writes for every drawn
// image: width and height in points, then the position.
var drawnImageBox = regexp.MustCompile(`q ([0-9.]+) 0 0 ([0-9.]+) [0-9.]+ [0-9.]+ cm /I\w+ Do Q`)

// streamBody matches the payload of one PDF stream object.
var streamBody = regexp.MustCompile(`(?s)\nstream\r?\n(.*?)\r?\nendstream`)

func TestMarkdownToPDFNeverFetchesRemoteImages(t *testing.T) {
	var hits atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	markdown := fmt.Sprintf("# 远程图片\n\n![远程图](%s/report-figure.png)\n\n正文照常渲染。\n", server.URL)
	out, err := MarkdownToPDF(context.Background(), []byte(markdown))
	if err != nil {
		t.Fatalf("MarkdownToPDF: %v", err)
	}
	if !bytes.HasPrefix(out, []byte("%PDF")) {
		t.Fatal("output is not a PDF document")
	}
	if got := hits.Load(); got != 0 {
		t.Fatalf("renderer made %d request(s) to the image host, want 0", got)
	}
}

func TestMarkdownToPDFLinksRemoteImages(t *testing.T) {
	const address = "https://example.com/report-figure.png"

	out, err := MarkdownToPDF(context.Background(), []byte(fmt.Sprintf("![远程图](%s)\n", address)))
	if err != nil {
		t.Fatalf("MarkdownToPDF: %v", err)
	}
	if !bytes.Contains(out, []byte("/URI (")) {
		t.Fatal("no link annotation in output: the remote image left nothing to click")
	}
	if !bytes.Contains(out, []byte(address)) {
		t.Fatalf("link annotation does not carry %s", address)
	}
	if bytes.Contains(out, []byte("/Subtype /Image")) {
		t.Fatal("an image XObject reached the document: the remote picture was fetched")
	}
}

func TestMarkdownToPDFLinksRemoteImagesOnlyOverHTTP(t *testing.T) {
	out, err := MarkdownToPDF(context.Background(), []byte("![脚本图](javascript:alert%281%29)\n"))
	if err != nil {
		t.Fatalf("MarkdownToPDF: %v", err)
	}
	if bytes.Contains(out, []byte("javascript:")) {
		t.Fatal("a javascript: destination became a link annotation")
	}
}

func TestMarkdownToPDFDrawsInlineImages(t *testing.T) {
	markdown := fmt.Sprintf("# 内联图片\n\n![内联图](%s)\n", inlinePNGDataURI)
	out, err := MarkdownToPDF(context.Background(), []byte(markdown))
	if err != nil {
		t.Fatalf("MarkdownToPDF: %v", err)
	}
	if !bytes.Contains(out, []byte("/Subtype /Image")) {
		t.Fatal("no image XObject in output: the inline data URI was dropped")
	}
}

func TestMarkdownToPDFKeepsGoingPastAnUndecodableInlineImage(t *testing.T) {
	markdown := "![坏图](data:image/png;base64,QUJD)\n\n后续正文必须照常渲染。\n"
	out, err := MarkdownToPDF(context.Background(), []byte(markdown))
	if err != nil {
		t.Fatalf("MarkdownToPDF: %v", err)
	}
	if !bytes.HasPrefix(out, []byte("%PDF")) {
		t.Fatal("output is not a PDF document")
	}
	if bytes.Contains(out, []byte("/Subtype /Image")) {
		t.Fatal("an image XObject reached the document from bytes the writer rejected")
	}
}

func TestMarkdownToPDFDrawsInlineImagesAtTheirOwnSize(t *testing.T) {
	const sourcePixels = 16

	markdown := fmt.Sprintf("![小图](%s)\n", squarePNGDataURI(t, sourcePixels))
	out, err := MarkdownToPDF(context.Background(), []byte(markdown))
	if err != nil {
		t.Fatalf("MarkdownToPDF: %v", err)
	}

	match := drawnImageBox.FindStringSubmatch(contentStreams(t, out))
	if match == nil {
		t.Fatal("no image placement operator in the content streams")
	}
	width, err := strconv.ParseFloat(match[1], 64)
	if err != nil {
		t.Fatalf("parse placement width %q: %v", match[1], err)
	}
	if width > sourcePixels+0.5 {
		t.Fatalf("a %dpx image was drawn %.2fpt wide: it was scaled up to the text column", sourcePixels, width)
	}
}

// squarePNGDataURI builds an opaque PNG of the given side length and returns it
// as a data URI, so a size assertion does not depend on a base64 literal.
func squarePNGDataURI(t *testing.T, side int) string {
	t.Helper()

	img := image.NewRGBA(image.Rect(0, 0, side, side))
	for y := range side {
		for x := range side {
			img.Set(x, y, color.RGBA{R: 0x2f, G: 0x6f, B: 0xed, A: 0xff})
		}
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}

	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(encoded.Bytes())
}

// contentStreams concatenates every zlib-compressed stream of a PDF that
// decompresses cleanly, which is where the drawing operators live.
func contentStreams(t *testing.T, document []byte) string {
	t.Helper()

	var streams bytes.Buffer
	for _, match := range streamBody.FindAllSubmatch(document, -1) {
		reader, err := zlib.NewReader(bytes.NewReader(match[1]))
		if err != nil {
			continue
		}
		decompressed, err := io.ReadAll(reader)
		reader.Close()
		if err != nil {
			continue
		}
		streams.Write(decompressed)
	}

	return streams.String()
}
