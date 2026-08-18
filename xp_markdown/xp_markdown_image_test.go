package xp_markdown

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

// 1x1 PNG, the smallest thing that proves an inline image reached the page.
const inlinePNGDataURI = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=="

func TestRenderNeverFetchesRemoteImages(t *testing.T) {
	var hits atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	markdown := fmt.Sprintf("# 远程图片\n\n![远程图](%s/report-figure.png)\n\n正文照常渲染。\n", server.URL)
	out, err := Render(context.Background(), []byte(markdown))
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !bytes.HasPrefix(out, []byte("%PDF")) {
		t.Fatal("output is not a PDF document")
	}
	if got := hits.Load(); got != 0 {
		t.Fatalf("renderer made %d request(s) to the image host, want 0", got)
	}
}

func TestRenderDrawsInlineImages(t *testing.T) {
	markdown := fmt.Sprintf("# 内联图片\n\n![内联图](%s)\n", inlinePNGDataURI)
	out, err := Render(context.Background(), []byte(markdown))
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !bytes.Contains(out, []byte("/Subtype /Image")) {
		t.Fatal("no image XObject in output: the inline data URI was dropped")
	}
}
