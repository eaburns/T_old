// Package font provides an interface for fonts.
package font

import (
	"image"
	"image/color"
	"image/draw"
	"io"
	"io/ioutil"
	"os"

	"github.com/eaburns/truefont/freetype"
	"github.com/eaburns/truefont/freetype/geom"
	"github.com/eaburns/truefont/freetype/truetype"
)

const (
	ptInch = 72.0
	pxInch = 96.0
	// Number of raster units in a pixel.
	// 64.0 is the number that freetype uses.
	unitsPx = 64.0
)

// Fix32 is a 24.8 fixed-point number.
type Fix32 int32

// Int returns the integer portion of a Fix32.
func (f Fix32) Int() int { return int(f >> 8) }

// String returns the string representation of the Fix32.
func (f Fix32) String() string { return geom.Fix32(f).String() }

// Point is a point in 2D space
// with Fix32 coordinates.
type Point struct {
	X, Y Fix32
}

// A Glyph is the font's representation of a rune.
type Glyph uint16

// Glyphs is a sequence of Glyphs.
type Glyphs struct {
	// The point of Glyphs is to hide typetype.Index.
	// []Glyph isn't convertable to []truetype.Index,
	// without copying, so we wrap a []truetype.Index
	// in a struct with simple methods to use it.
	glyphs []truetype.Index
}

// Append appends a glyph to the end of a Glyphs.
func (gs *Glyphs) Append(g Glyph) {
	gs.glyphs = append(gs.glyphs, truetype.Index(g))
}

// Font is an interface for accessing font metrics
// and rendering text in sized fonts.
type Font interface {
	// Glyph returns the font's Glyph for a given rune.
	Glyph(rune) Glyph
	// Ascent returns the ascent of the Sized font.
	// That is the distance from the baseline
	// to the top of the font's ascenders.
	Ascent() Fix32
	// Height returns the height of the Sized font.
	// The height is the minimum line spacing for the font.
	Height() Fix32
	// Advance returns the advance width of a glyph.
	// The advance is horizontal distance between the pen's
	// initial position to the position of the next glyph.
	Advance(Glyph) Fix32
	// Kerning returns the kerning between two glyphs.
	// The kerning is the offset used to adjust the advance
	// between particular pairs of glyphs.
	Kerning(Glyph, Glyph) Fix32
	// DrawGlyphs draws glyphs in the font to the image.
	// The point is the intersection of the baseline
	// with the left-most edge of the text.
	DrawGlyphs(draw.Image, Glyphs, color.Color, Point)
}

// LoadTTF returns a truetype.Font loaded
// from the TTF file at the given path.
func LoadTTF(path string) (*truetype.Font, error) {
	r, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	f, err := ReadTTF(r)
	if err != nil {
		return nil, err
	}
	return f, nil
}

// ReadTTF returns a truetype.Font read
// from TTF data read from an io.Reader.
func ReadTTF(r io.Reader) (*truetype.Font, error) {
	d, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}
	f, err := freetype.ParseFont(d)
	if err != nil {
		return nil, err
	}
	return f, nil
}

// A font implements the Font interface
// backed by a truetype.Font.
type font struct {
	ttf   *truetype.Font
	size  int
	scale int32

	// freetype.Context caches the rendered glyphs.
	ctx *freetype.Context
}

// New returns a new Font.
func New(ttf *truetype.Font, size int) Font {
	ctx := freetype.NewContext()
	ctx.SetFont(ttf)
	ctx.SetFontSize(float64(size))
	ctx.SetDPI(pxInch)
	return &font{
		ttf:   ttf,
		size:  size,
		scale: int32(float64(size)*pxInch*(unitsPx/ptInch)) << 2,
		ctx:   ctx,
	}
}

func (f *font) Glyph(r rune) Glyph {
	return Glyph(f.ttf.Index(r))
}

func (f *font) Ascent() Fix32 {
	return Fix32(f.ttf.HMetric(f.scale).Ascent)
}

func (f *font) Height() Fix32 {
	m := f.ttf.HMetric(f.scale)
	return Fix32(m.Ascent - m.Descent + m.LineGap)
}

func (f *font) Advance(g Glyph) Fix32 {
	return Fix32(f.ttf.GlyphHMetric(f.scale, truetype.Index(g)).AdvanceWidth)
}

func (f *font) Kerning(p, c Glyph) Fix32 {
	return Fix32(f.ttf.Kerning(f.scale, truetype.Index(p), truetype.Index(c)))
}

func (f *font) DrawGlyphs(dst draw.Image, gs Glyphs, c color.Color, p Point) {
	f.ctx.SetSrc(image.NewUniform(c))
	f.ctx.SetDst(dst)
	f.ctx.SetClip(dst.Bounds())
	gp := geom.Point{X: geom.Fix32(p.X), Y: geom.Fix32(p.Y)}
	if _, err := f.ctx.DrawGlyphs(gs.glyphs, gp); err != nil {
		panic(err)
	}
}

// Width returns the width of a glyph string in a font.
func Width(f Font, gs Glyphs) Fix32 {
	var w Fix32
	for i, g := range gs.glyphs {
		if i > 0 {
			w += f.Kerning(Glyph(gs.glyphs[i-1]), Glyph(g))
		}
		w += f.Advance(Glyph(g))
	}
	return w
}

// Kerning returns the kerning
// between the last glyph in gs
// and the glyph g.
func Kerning(f Font, gs Glyphs, g Glyph) Fix32 {
	if len(gs.glyphs) == 0 {
		return 0
	}
	p := Glyph(gs.glyphs[len(gs.glyphs)-1])
	return f.Kerning(p, g)
}
