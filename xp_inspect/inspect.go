// Package xp_inspect reads rendered PDF artifacts. Application release policy
// belongs to the caller; unsupported encodings return errors rather than pass.
package xp_inspect

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	cs "github.com/benoitkugler/pdf/contentstream"
	"github.com/benoitkugler/pdf/fonts/cmaps"
	"github.com/benoitkugler/pdf/fonts/simpleencodings"
	"github.com/benoitkugler/pdf/model"
	"github.com/benoitkugler/pdf/reader"
	"github.com/benoitkugler/pdf/reader/file"
	"github.com/benoitkugler/pdf/reader/parser"
)

type Config struct {
	MaxPDFBytes    int64
	MaxStreamBytes int64
	DecodeImages   bool
}
type Extension interface{ Extend(*Config) error }
type ExtensionFunc func(*Config) error

func (f ExtensionFunc) Extend(c *Config) error { return f(c) }

type Font struct {
	Name     string
	Embedded bool
}
type Image struct {
	Width, Height    int
	BitsPerComponent uint8
	ColorSpace       string
	Samples          []byte
}
type Page struct {
	MediaBox [4]float64
	Text     string
	Fonts    []Font
	Images   []Image
}
type Document struct {
	Title string
	Pages []Page
}

type contextReader struct {
	context.Context
	*bytes.Reader
}

func (r contextReader) Read(b []byte) (int, error) {
	if e := r.Err(); e != nil {
		return 0, e
	}
	return r.Reader.Read(b)
}
func (r contextReader) Seek(n int64, w int) (int64, error) {
	if e := r.Err(); e != nil {
		return 0, e
	}
	return r.Reader.Seek(n, w)
}

func decodeStream(ctx context.Context, s model.Stream, limit int64) ([]byte, error) {
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	rd, err := s.Filter.DecodeReader(bytes.NewReader(s.Content))
	if err != nil {
		return nil, err
	}
	raw, err := io.ReadAll(io.LimitReader(rd, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > limit {
		return nil, errors.New("xp_inspect: decoded stream limit exceeded")
	}
	return raw, ctx.Err()
}

type decoder struct {
	font    Font
	wide    bool
	unicode map[model.CID][]rune
	simple  map[byte]rune
}

func newDecoder(ctx context.Context, f *model.FontDict, limit int64) (decoder, error) {
	if f == nil || f.Subtype == nil {
		return decoder{}, errors.New("xp_inspect: missing font")
	}
	d := decoder{font: Font{Name: string(f.Subtype.FontName())}}
	switch sub := f.Subtype.(type) {
	case model.FontType0:
		d.wide = true
		d.font.Embedded = sub.DescendantFonts.FontDescriptor.FontFile != nil
		if encoding, ok := sub.Encoding.(model.CMapEncodingPredefined); !ok || encoding != "Identity-H" {
			return d, errors.New("xp_inspect: unsupported composite encoding")
		}
	case model.FontType1:
		d.font.Embedded = sub.FontDescriptor.FontFile != nil
	case model.FontTrueType:
		d.font.Embedded = sub.FontDescriptor.FontFile != nil
	default:
		return d, errors.New("xp_inspect: unsupported font subtype")
	}
	if f.ToUnicode != nil {
		raw, e := decodeStream(ctx, f.ToUnicode.Stream, limit)
		if e != nil {
			return d, e
		}
		cmap, e := cmaps.ParseUnicodeCMap(raw)
		if e != nil {
			return d, e
		}
		if cmap.UseCMap != "" {
			return d, errors.New("xp_inspect: unresolved base Unicode CMap")
		}
		d.unicode = map[model.CID][]rune{}
		for _, mapping := range cmap.Mappings {
			switch m := mapping.(type) {
			case cmaps.ToUnicodePair:
				d.unicode[m.From] = m.Dest
			case cmaps.ToUnicodeTranslation:
				for code := int(m.From); code <= int(m.To); code++ {
					d.unicode[model.CID(code)] = []rune{m.Dest + rune(code-int(m.From))}
				}
			case cmaps.ToUnicodeArray:
				if len(m.Runes) != int(m.To)-int(m.From)+1 {
					return d, errors.New("xp_inspect: invalid Unicode range")
				}
				for code := int(m.From); code <= int(m.To); code++ {
					d.unicode[model.CID(code)] = m.Runes[code-int(m.From)]
				}
			default:
				return d, errors.New("xp_inspect: unsupported Unicode mapping")
			}
		}
	} else if simple, ok := f.Subtype.(model.FontSimple); ok {
		encoding := simple.SimpleEncoding()
		name, ok := encoding.(model.SimpleEncodingPredefined)
		if encoding != nil && !ok {
			return d, errors.New("xp_inspect: unsupported simple encoding")
		}
		switch name {
		case "", "StandardEncoding":
			d.simple = simpleencodings.AdobeStandard.ByteToRune()
		case "WinAnsiEncoding":
			d.simple = simpleencodings.WinAnsi.ByteToRune()
		case "MacRomanEncoding":
			d.simple = simpleencodings.MacRoman.ByteToRune()
		default:
			return d, errors.New("xp_inspect: unknown simple encoding")
		}
	} else {
		return d, errors.New("xp_inspect: Unicode mapping missing")
	}
	return d, nil
}
func (d decoder) decode(raw []byte) (string, error) {
	var out strings.Builder
	stride := 1
	if d.wide {
		stride = 2
	}
	if len(raw)%stride != 0 {
		return "", errors.New("xp_inspect: truncated character code")
	}
	for i := 0; i < len(raw); i += stride {
		code := model.CID(raw[i])
		if d.wide {
			code = code<<8 | model.CID(raw[i+1])
		}
		if d.unicode != nil {
			r, ok := d.unicode[code]
			if !ok || len(r) == 0 {
				return "", fmt.Errorf("xp_inspect: unmapped character code %d", code)
			}
			out.WriteString(string(r))
		} else {
			r, ok := d.simple[byte(code)]
			if !ok {
				return "", fmt.Errorf("xp_inspect: unmapped byte %d", code)
			}
			out.WriteRune(r)
		}
	}
	return out.String(), nil
}

// Inspect reads actual page content, including only images painted by Do.
// DecodeImages adds decoded samples; no network, host fonts, OCR or files are used.
func Inspect(ctx context.Context, raw []byte, extensions ...Extension) (out Document, err error) {
	defer func() {
		if v := recover(); v != nil {
			out = Document{}
			err = fmt.Errorf("xp_inspect: malformed PDF: %v", v)
		}
	}()
	config := Config{MaxPDFBytes: 64 << 20, MaxStreamBytes: 64 << 20}
	for _, e := range extensions {
		if e == nil {
			return out, errors.New("xp_inspect: nil extension")
		}
		if err = e.Extend(&config); err != nil {
			return out, err
		}
	}
	if len(raw) == 0 || config.MaxPDFBytes <= 0 || config.MaxStreamBytes <= 0 || int64(len(raw)) > config.MaxPDFBytes {
		return out, errors.New("xp_inspect: invalid input or limits")
	}
	if err = ctx.Err(); err != nil {
		return out, err
	}
	parsed, err := file.Read(contextReader{ctx, bytes.NewReader(raw)}, nil)
	if err != nil {
		return out, err
	}
	pdf, encrypted, err := reader.ProcessContext(parsed)
	if err != nil {
		return out, err
	}
	if encrypted != nil {
		return out, errors.New("xp_inspect: encrypted documents unsupported")
	}
	out.Title = pdf.Trailer.Info.Title
	maps, err := pageUnicodeMaps(parsed)
	if err != nil {
		return out, err
	}
	cache := map[*model.FontDict]decoder{}
	for pageIndex, p := range inheritedPages(pdf.Catalog.Pages, nil, nil) {
		if err = ctx.Err(); err != nil {
			return out, err
		}
		if p.MediaBox == nil || p.Resources == nil {
			return out, errors.New("xp_inspect: page geometry/resources missing")
		}
		page := Page{MediaBox: [4]float64{float64(p.MediaBox.Llx), float64(p.MediaBox.Lly), float64(p.MediaBox.Urx), float64(p.MediaBox.Ury)}}
		var content bytes.Buffer
		for _, stream := range p.Contents {
			b, e := decodeStream(ctx, stream.Stream, config.MaxStreamBytes)
			if e != nil {
				return out, e
			}
			if int64(content.Len()+len(b)) > config.MaxStreamBytes {
				return out, errors.New("xp_inspect: page content limit exceeded")
			}
			content.Write(b)
			content.WriteByte('\n')
		}
		operations, e := parser.ParseContent(content.Bytes(), p.Resources.ColorSpace)
		if e != nil {
			return out, e
		}
		var text strings.Builder
		var active decoder
		selected := false
		used := map[model.Name]bool{}
		type textState struct {
			decoder  decoder
			selected bool
		}
		stack := []textState{}
		appendText := func(raw string) error {
			if !selected {
				return errors.New("xp_inspect: text without selected font")
			}
			s, e := active.decode([]byte(raw))
			if e == nil {
				text.WriteString(s)
			}
			return e
		}
		for _, operation := range operations {
			if err = ctx.Err(); err != nil {
				return out, err
			}
			switch op := operation.(type) {
			case cs.OpSave:
				stack = append(stack, textState{active, selected})
			case cs.OpRestore:
				if len(stack) == 0 {
					return out, errors.New("xp_inspect: unbalanced graphics state")
				}
				state := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				active, selected = state.decoder, state.selected
			case cs.OpSetFont:
				f := p.Resources.Font[op.Font]
				var ok bool
				active, ok = cache[f]
				if !ok {
					font := f
					if f != nil && f.ToUnicode == nil {
						if unicode, ok := maps[pageIndex][op.Font]; ok {
							copy := *f
							copy.ToUnicode = unicode
							font = &copy
						}
					}
					active, err = newDecoder(ctx, font, config.MaxStreamBytes)
					if err != nil {
						return out, err
					}
					cache[f] = active
				}
				selected = true
				if !used[op.Font] {
					page.Fonts = append(page.Fonts, active.font)
					used[op.Font] = true
				}
			case cs.OpBeginText, cs.OpTextNextLine, cs.OpTextMove, cs.OpTextMoveSet:
				text.WriteByte('\n')
			case cs.OpShowText:
				if err = appendText(op.Text); err != nil {
					return out, err
				}
			case cs.OpMoveShowText:
				text.WriteByte('\n')
				if err = appendText(op.Text); err != nil {
					return out, err
				}
			case cs.OpMoveSetShowText:
				text.WriteByte('\n')
				if err = appendText(op.Text); err != nil {
					return out, err
				}
			case cs.OpShowSpaceText:
				for _, piece := range op.Texts {
					if err = appendText(string(piece.CharCodes)); err != nil {
						return out, err
					}
				}
			case cs.OpXObject:
				switch object := p.Resources.XObject[op.XObject].(type) {
				case *model.XObjectImage:
					im := Image{Width: object.Width, Height: object.Height, BitsPerComponent: object.BitsPerComponent}
					if name, ok := object.ColorSpace.(model.ColorSpaceName); ok {
						im.ColorSpace = string(name)
					}
					if config.DecodeImages {
						if object.SMask != nil || object.Mask != nil || len(object.Decode) != 0 {
							return out, errors.New("xp_inspect: image mask/decode unsupported")
						}
						im.Samples, err = decodeStream(ctx, object.Stream, config.MaxStreamBytes)
						if err != nil {
							return out, err
						}
					}
					page.Images = append(page.Images, im)
				default:
					return out, errors.New("xp_inspect: unsupported drawing object")
				}
			}
		}
		page.Text = text.String()
		out.Pages = append(out.Pages, page)
	}
	if len(out.Pages) == 0 {
		return out, errors.New("xp_inspect: document has no pages")
	}
	return out, nil
}

func inheritedPages(tree model.PageTree, resources *model.ResourcesDict, box *model.Rectangle) []model.PageObject {
	if tree.Resources != nil {
		resources = tree.Resources
	}
	if tree.MediaBox != nil {
		box = tree.MediaBox
	}
	out := []model.PageObject{}
	for _, node := range tree.Kids {
		switch page := node.(type) {
		case *model.PageTree:
			out = append(out, inheritedPages(*page, resources, box)...)
		case *model.PageObject:
			copied := *page
			if copied.Resources == nil {
				copied.Resources = resources
			}
			if copied.MediaBox == nil {
				copied.MediaBox = box
			}
			out = append(out, copied)
		}
	}
	return out
}

// Read ToUnicode from the same parsed objects: the upstream high-level model
// omits streams without UseCMap. No source objects or PDF bytes are changed.
func pageUnicodeMaps(pdf file.PDFFile) ([]map[model.Name]*model.UnicodeCMap, error) {
	dict := func(o model.Object) model.ObjDict { d, _ := pdf.ResolveObject(o).(model.ObjDict); return d }
	out := []map[model.Name]*model.UnicodeCMap{}
	var walk func(model.Object, model.Object, int) error
	walk = func(o model.Object, resources model.Object, depth int) error {
		if depth > 64 {
			return errors.New("xp_inspect: page tree depth exceeded")
		}
		d := dict(o)
		if r, ok := d["Resources"]; ok {
			resources = r
		}
		if typ, _ := d["Type"].(model.ObjName); typ == "Pages" {
			kids, _ := pdf.ResolveObject(d["Kids"]).(model.ObjArray)
			for _, kid := range kids {
				if err := walk(kid, resources, depth+1); err != nil {
					return err
				}
			}
			return nil
		}
		fonts := dict(dict(resources)["Font"])
		maps := map[model.Name]*model.UnicodeCMap{}
		for name, object := range fonts {
			font := dict(object)
			if raw, ok := pdf.ResolveObject(font["ToUnicode"]).(model.ObjStream); ok {
				filters, err := parser.ParseFilters(raw.Args["Filter"], raw.Args["DecodeParms"], func(o model.Object) (model.Object, error) { return pdf.ResolveObject(o), nil })
				if err != nil {
					return err
				}
				if _, has := raw.Args["UseCMap"]; has {
					return errors.New("xp_inspect: base Unicode CMap unsupported")
				}
				maps[model.Name(name)] = &model.UnicodeCMap{Stream: model.Stream{Content: raw.Content, Filter: filters}}
			}
		}
		out = append(out, maps)
		return nil
	}
	root := dict(pdf.Root)
	if err := walk(root["Pages"], nil, 0); err != nil {
		return nil, err
	}
	return out, nil
}
