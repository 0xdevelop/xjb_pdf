package xp_render

import (
	"context"
	"errors"
	"math"

	pdf "github.com/stephenafamo/goldmark-pdf"
	"github.com/yuin/goldmark/ast"
	east "github.com/yuin/goldmark/extension/ast"
)

// The upstream table renderer stores its current table in package variables.
// Rendering the whole node here keeps layout state local to this document.
type tableRenderer struct{ ctx context.Context }

func (r tableRenderer) RegisterFuncs(reg pdf.NodeRendererFuncRegisterer) {
	reg.Register(east.KindTable, r.render)
}

func (r tableRenderer) render(w *pdf.Writer, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	writer, ok := w.Pdf.(*pdf.Fpdf)
	if !ok {
		return ast.WalkStop, errors.New("xp_render: table requires Fpdf writer")
	}
	f := writer.Fpdf
	data := pdf.CollectTableData(w, source, node)
	pdf.SetStyle(w.Pdf, *w.Styles.THeader)
	widths := pdf.CalculateOptimalColumnWidths(w, data)
	if len(widths) == 0 {
		return ast.WalkSkipChildren, nil
	}
	const padding = 3.0
	for _, width := range widths {
		if width <= 2*padding {
			return ast.WalkStop, errors.New("xp_render: table columns too narrow for page")
		}
	}
	left, top, _, bottom := f.GetMargins()
	f.Ln(w.Styles.Normal.Size + w.Styles.Normal.Spacing)
	f.SetX(left)
	wrap := func(row []string, style pdf.Style, bold bool) ([][]string, int) {
		pdf.SetStyle(w.Pdf, style)
		if bold {
			f.SetFont(style.Font.Family, pdf.FontStyleBold, style.Size)
		}
		lines, count := make([][]string, len(widths)), 1
		for i, width := range widths {
			if i < len(row) {
				lines[i] = f.SplitText(row[i], width-2*padding)
			}
			count = max(count, len(lines[i]))
		}
		return lines, count
	}
	draw := func(lines [][]string, start, count int, style pdf.Style, bold bool) {
		pdf.SetStyle(w.Pdf, style)
		if bold {
			f.SetFont(style.Font.Family, pdf.FontStyleBold, style.Size)
		}
		lineHeight, x, y := style.Size+style.Spacing, left, f.GetY()
		height := float64(count)*lineHeight + 2*padding
		for i, width := range widths {
			f.Rect(x, y, width, height, "F")
			for line := 0; line < count && start+line < len(lines[i]); line++ {
				f.SetXY(x+padding, y+padding+float64(line)*lineHeight)
				f.CellFormat(width-2*padding, lineHeight, lines[i][start+line], "", 0, "L", false, 0, "")
			}
			x += width
		}
		f.SetXY(left, y+height)
	}
	header, headerLines := wrap(data.Headers, *w.Styles.THeader, true)
	headerHeight := float64(headerLines)*(w.Styles.THeader.Size+w.Styles.THeader.Spacing) + 2*padding
	_, pageHeight := f.GetPageSize()
	bodyLine := w.Styles.TBody.Size + w.Styles.TBody.Spacing
	if headerHeight+bodyLine+2*padding > pageHeight-top-bottom {
		return ast.WalkStop, errors.New("xp_render: table header cannot fit page")
	}
	if f.GetY()+headerHeight+bodyLine+2*padding > pageHeight-bottom {
		f.AddPage()
	}
	draw(header, 0, headerLines, *w.Styles.THeader, true)
	for _, row := range data.Rows {
		if err := r.ctx.Err(); err != nil {
			return ast.WalkStop, err
		}
		lines, count := wrap(row, *w.Styles.TBody, false)
		for start := 0; start < count; {
			_, pageHeight = f.GetPageSize()
			available := int(math.Floor((pageHeight - bottom - f.GetY() - 2*padding) / bodyLine))
			if available < 1 {
				f.AddPage()
				draw(header, 0, headerLines, *w.Styles.THeader, true)
				_, pageHeight = f.GetPageSize()
				available = int(math.Floor((pageHeight - bottom - f.GetY() - 2*padding) / bodyLine))
				if available < 1 {
					return ast.WalkStop, errors.New("xp_render: table body cannot fit page after header")
				}
			}
			n := min(count-start, available)
			draw(lines, start, n, *w.Styles.TBody, false)
			start += n
		}
	}
	pdf.SetStyle(w.Pdf, *w.Styles.Normal)
	f.Ln(w.Styles.Normal.Size + w.Styles.Normal.Spacing)
	if err := f.Error(); err != nil {
		return ast.WalkStop, err
	}
	return ast.WalkSkipChildren, nil
}
