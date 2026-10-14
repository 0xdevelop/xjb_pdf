package xp_render

import (
	"context"
	"errors"
	"github.com/phpdave11/gofpdf"
	"math"
	"strings"

	pdf "github.com/stephenafamo/goldmark-pdf"
	"github.com/yuin/goldmark/ast"
	east "github.com/yuin/goldmark/extension/ast"
)

// The upstream table renderer stores its current table in package variables.
// Rendering the whole node here keeps layout state local to this document.
type tableRenderer struct {
	ctx             context.Context
	layout          TableLayout
	baseOrientation string
}

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
	if r.layout.Configure != nil {
		if err := r.layout.Configure(w, data); err != nil {
			return ast.WalkStop, err
		}
	}
	if err := validateStyles(w.Styles); err != nil {
		return ast.WalkStop, err
	}
	width, height := f.GetPageSize()
	size := gofpdf.SizeType{Wd: min(width, height), Ht: max(width, height)}
	base := strings.ToUpper(r.baseOrientation)
	if base == "" {
		base = "P"
	}
	orientation := base
	if r.layout.LandscapeMinColumns == 0 {
		if width > height {
			orientation = "L"
		} else {
			orientation = "P"
		}
	}
	if r.layout.LandscapeMinColumns > 0 && len(data.Headers) >= r.layout.LandscapeMinColumns {
		orientation = "L"
	}
	left, top, _, bottom := f.GetMargins()
	newPage := func(direction string) { f.AddPageFormat(direction, size); f.SetXY(left, top) }
	current := "P"
	if width > height {
		current = "L"
	}
	if current != orientation {
		newPage(orientation)
	}

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
	firstHeight := headerHeight + bodyLine + 2*padding
	if r.layout.KeepRowsTogether && len(data.Rows) > 0 {
		_, count := wrap(data.Rows[0], *w.Styles.TBody, false)
		complete := headerHeight + float64(count)*bodyLine + 2*padding
		if complete <= pageHeight-top-bottom {
			firstHeight = complete
		}
	}
	if f.GetY()+firstHeight > pageHeight-bottom {
		newPage(orientation)
	}
	draw(header, 0, headerLines, *w.Styles.THeader, true)
	for _, row := range data.Rows {
		if err := r.ctx.Err(); err != nil {
			return ast.WalkStop, err
		}
		lines, count := wrap(row, *w.Styles.TBody, false)
		_, pageHeight = f.GetPageSize()
		rowHeight := float64(count)*bodyLine + 2*padding
		if r.layout.KeepRowsTogether && rowHeight <= pageHeight-top-bottom-headerHeight && f.GetY()+rowHeight > pageHeight-bottom {
			newPage(orientation)
			draw(header, 0, headerLines, *w.Styles.THeader, true)
		}
		for start := 0; start < count; {
			_, pageHeight = f.GetPageSize()
			available := int(math.Floor((pageHeight - bottom - f.GetY() - 2*padding) / bodyLine))
			if available < 1 {
				newPage(orientation)
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
	if r.layout.LandscapeMinColumns > 0 && orientation != base {
		next := node.NextSibling()
		for next != nil && next.Kind() == ast.KindHTMLBlock {
			next = next.NextSibling()
		}
		if next != nil {
			newPage(base)
		}
	}

	if err := f.Error(); err != nil {
		return ast.WalkStop, err
	}
	return ast.WalkSkipChildren, nil
}
