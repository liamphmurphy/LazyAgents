// Package deck is the Slides deck schema (ADR: Slides deck model and export):
// validation, normalisation and the ops a persona edits a deck with. A deck
// is the JSON body of a canvas of kind "deck"; this package knows nothing
// about storage.
package deck

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/liamphmurphy/vesper/server/internal/apperr"
)

// Schema is the version this package reads and writes.
const Schema = 1

const (
	Width, Height  = 1920, 1080
	MaxSlides      = 200
	MaxElements    = 100
	MaxTextRunes   = 5000
	MaxParagraphs  = 200
	MaxNotesRunes  = 10000
	maxCoordinate  = 10000
	minFontSize    = 8
	maxFontSize    = 400
	maxStrokeWidth = 100
)

type Frame struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	W float64 `json:"w"`
	H float64 `json:"h"`
}

type Run struct {
	Text      string  `json:"text"`
	Size      float64 `json:"size"`
	Bold      bool    `json:"bold,omitempty"`
	Italic    bool    `json:"italic,omitempty"`
	Underline bool    `json:"underline,omitempty"`
	Color     string  `json:"color,omitempty"`
	Font      string  `json:"font,omitempty"`
}

type Paragraph struct {
	Runs        []Run   `json:"runs"`
	Align       string  `json:"align,omitempty"`
	Bullet      string  `json:"bullet,omitempty"`
	Level       int     `json:"level,omitempty"`
	SpaceBefore float64 `json:"spaceBefore,omitempty"`
}

type TextBox struct {
	Paragraphs []Paragraph `json:"paragraphs"`
	VAlign     string      `json:"valign,omitempty"`
}

type Crop struct {
	L float64 `json:"l"`
	T float64 `json:"t"`
	R float64 `json:"r"`
	B float64 `json:"b"`
}

// Element is every element type in one struct; fields that don't apply to
// its type are refused by Normalize.
type Element struct {
	ID          string      `json:"id"`
	Type        string      `json:"type"`
	Frame       Frame       `json:"frame"`
	Rotation    float64     `json:"rotation,omitempty"`
	Placeholder string      `json:"placeholder,omitempty"`
	Paragraphs  []Paragraph `json:"paragraphs,omitempty"`
	VAlign      string      `json:"valign,omitempty"`
	Shape       string      `json:"shape,omitempty"`
	Fill        string      `json:"fill,omitempty"`
	Stroke      string      `json:"stroke,omitempty"`
	StrokeWidth float64     `json:"strokeWidth,omitempty"`
	Radius      float64     `json:"radius,omitempty"`
	Text        *TextBox    `json:"text,omitempty"`
	EndArrow    bool        `json:"endArrow,omitempty"`
	StartArrow  bool        `json:"startArrow,omitempty"`
	Asset       string      `json:"asset,omitempty"`
	Crop        *Crop       `json:"crop,omitempty"`
	Alt         string      `json:"alt,omitempty"`
}

type Background struct {
	Color string `json:"color"`
}

type Slide struct {
	ID         string      `json:"id"`
	Layout     string      `json:"layout"`
	Background *Background `json:"background,omitempty"`
	Notes      string      `json:"notes,omitempty"`
	Hidden     bool        `json:"hidden,omitempty"`
	Elements   []Element   `json:"elements"`
}

type Theme struct {
	Name   string            `json:"name"`
	Fonts  map[string]string `json:"fonts"`
	Colors map[string]string `json:"colors"`
}

type Asset struct {
	Mime string `json:"mime"`
	W    int    `json:"w"`
	H    int    `json:"h"`
	Alt  string `json:"alt,omitempty"`
}

type Size struct {
	W int `json:"w"`
	H int `json:"h"`
}

type Deck struct {
	Schema int              `json:"schema"`
	Size   Size             `json:"size"`
	Theme  Theme            `json:"theme"`
	Assets map[string]Asset `json:"assets"`
	Slides []Slide          `json:"slides"`
}

var (
	ThemeColors = []string{"background", "text", "heading", "accent", "accent2", "muted", "surface"}
	Slots       = []string{"sans", "serif", "mono"}
	Layouts     = []string{"title", "title-body", "two-column", "image-right", "section", "blank"}
	hexColor    = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)
	slideID     = regexp.MustCompile(`^s_[a-z0-9_]{1,40}$`)
	elementID   = regexp.MustCompile(`^e_[a-z0-9_]{1,40}$`)
	AssetID     = regexp.MustCompile(`^a_[a-z0-9_]{1,40}$`)
)

func in(v string, set ...string) bool {
	for _, s := range set {
		if v == s {
			return true
		}
	}
	return false
}

// NewID mints an id with prefix ("s", "e" or "a") and 10 random base32 characters.
func NewID(prefix string) string {
	const alphabet = "abcdefghijklmnopqrstuvwxyz234567"
	b := make([]byte, 10)
	_, _ = rand.Read(b)
	for i := range b {
		b[i] = alphabet[int(b[i])%len(alphabet)]
	}
	return prefix + "_" + string(b)
}

func invalid(format string, args ...any) error {
	return apperr.Invalid("body", format, args...)
}

func bytesReader(b []byte) *bytes.Reader { return bytes.NewReader(b) }

func round(v float64) float64 { return math.Round(v*100) / 100 }

func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

func checkColor(theme Theme, c, where string, required bool) error {
	if c == "" {
		if required {
			return invalid("%s: a colour is required", where)
		}
		return nil
	}
	if hexColor.MatchString(c) {
		return nil
	}
	if _, ok := theme.Colors[c]; ok {
		return nil
	}
	return invalid("%s: colour %q is neither #RRGGBB nor a theme colour (%s)", where, c, strings.Join(ThemeColors, ", "))
}

func checkNumber(v *float64, lo, hi float64, where string) error {
	if !finite(*v) || *v < lo || *v > hi {
		return invalid("%s must be between %g and %g", where, lo, hi)
	}
	*v = round(*v)
	return nil
}

func normalizeParagraphs(theme Theme, ps []Paragraph, where string) error {
	if len(ps) == 0 {
		return invalid("%s: give at least one paragraph", where)
	}
	if len(ps) > MaxParagraphs {
		return invalid("%s: at most %d paragraphs", where, MaxParagraphs)
	}
	runes := 0
	for i := range ps {
		p := &ps[i]
		w := fmt.Sprintf("%s paragraph %d", where, i+1)
		if !in(p.Align, "", "left", "center", "right") {
			return invalid("%s: align must be left, center or right", w)
		}
		if p.Align == "left" {
			p.Align = ""
		}
		if !in(p.Bullet, "", "disc", "number") {
			return invalid("%s: bullet must be disc or number", w)
		}
		if p.Level < 0 || p.Level > 4 {
			return invalid("%s: level must be 0 to 4", w)
		}
		if err := checkNumber(&p.SpaceBefore, 0, 400, w+" spaceBefore"); err != nil {
			return err
		}
		if len(p.Runs) == 0 {
			return invalid("%s: give at least one run (an empty paragraph is one run with empty text)", w)
		}
		for j := range p.Runs {
			r := &p.Runs[j]
			rw := fmt.Sprintf("%s run %d", w, j+1)
			if !utf8.ValidString(r.Text) || strings.ContainsAny(r.Text, "\n\r") {
				return invalid("%s: text must be valid UTF-8 without line breaks (start a new paragraph instead)", rw)
			}
			runes += utf8.RuneCountInString(r.Text)
			if err := checkNumber(&r.Size, minFontSize, maxFontSize, rw+" size"); err != nil {
				return err
			}
			if err := checkColor(theme, r.Color, rw, false); err != nil {
				return err
			}
			if !in(r.Font, "", "heading", "body", "sans", "serif", "mono") {
				return invalid("%s: font must be heading, body, sans, serif or mono", rw)
			}
		}
	}
	if runes > MaxTextRunes {
		return invalid("%s: at most %d characters of text", where, MaxTextRunes)
	}
	return nil
}

func normalizeElement(d *Deck, e *Element, where string) error {
	if !elementID.MatchString(e.ID) {
		return invalid("%s: id %q must look like e_ followed by 1–40 lowercase letters, digits or underscores", where, e.ID)
	}
	f := &e.Frame
	for _, c := range []struct {
		v  *float64
		lo float64
		n  string
	}{{&f.X, -maxCoordinate, "x"}, {&f.Y, -maxCoordinate, "y"}, {&f.W, 0, "w"}, {&f.H, 0, "h"}} {
		if err := checkNumber(c.v, c.lo, maxCoordinate, where+" frame."+c.n); err != nil {
			return err
		}
	}
	if err := checkNumber(&e.Rotation, -360, 360, where+" rotation"); err != nil {
		return err
	}
	if !in(e.Placeholder, "", "title", "subtitle", "body", "body2", "image") {
		return invalid("%s: unknown placeholder %q", where, e.Placeholder)
	}
	if err := checkNumber(&e.StrokeWidth, 0, maxStrokeWidth, where+" strokeWidth"); err != nil {
		return err
	}
	if err := checkNumber(&e.Radius, 0, 1000, where+" radius"); err != nil {
		return err
	}
	notFor := func(cond bool, field string) error {
		if cond {
			return invalid("%s: a %s element has no %s", where, e.Type, field)
		}
		return nil
	}
	switch e.Type {
	case "text":
		if f.W < 1 || f.H < 1 {
			return invalid("%s: a text box needs a width and height", where)
		}
		if !in(e.VAlign, "", "top", "middle", "bottom") {
			return invalid("%s: valign must be top, middle or bottom", where)
		}
		for _, c := range []error{notFor(e.Shape != "" || e.Fill != "" || e.Stroke != "", "shape, fill or stroke"), notFor(e.Text != nil, "text (use paragraphs)"), notFor(e.Asset != "" || e.Crop != nil, "asset or crop")} {
			if c != nil {
				return c
			}
		}
		return normalizeParagraphs(d.Theme, e.Paragraphs, where)
	case "shape":
		if !in(e.Shape, "rect", "roundRect", "ellipse") {
			return invalid("%s: shape must be rect, roundRect or ellipse", where)
		}
		if err := notFor(len(e.Paragraphs) > 0, "paragraphs (put them in text)"); err != nil {
			return err
		}
		if err := notFor(e.Asset != "" || e.Crop != nil, "asset or crop"); err != nil {
			return err
		}
		if err := checkColor(d.Theme, e.Fill, where+" fill", false); err != nil {
			return err
		}
		if err := checkColor(d.Theme, e.Stroke, where+" stroke", false); err != nil {
			return err
		}
		if e.Text != nil {
			if !in(e.Text.VAlign, "", "top", "middle", "bottom") {
				return invalid("%s: valign must be top, middle or bottom", where)
			}
			return normalizeParagraphs(d.Theme, e.Text.Paragraphs, where+" text")
		}
		return nil
	case "line":
		if err := checkColor(d.Theme, e.Stroke, where+" stroke", true); err != nil {
			return err
		}
		if e.StrokeWidth <= 0 {
			return invalid("%s: a line needs a strokeWidth", where)
		}
		return notFor(e.Fill != "" || e.Text != nil || len(e.Paragraphs) > 0 || e.Asset != "", "fill, text or asset")
	case "image":
		if _, ok := d.Assets[e.Asset]; !ok {
			return invalid("%s: asset %q isn't in the deck's assets", where, e.Asset)
		}
		if f.W < 1 || f.H < 1 {
			return invalid("%s: an image needs a width and height", where)
		}
		if e.Crop != nil {
			c := e.Crop
			for _, v := range []*float64{&c.L, &c.T, &c.R, &c.B} {
				if err := checkNumber(v, 0, 0.95, where+" crop"); err != nil {
					return err
				}
			}
			if c.L+c.R > 0.95 || c.T+c.B > 0.95 {
				return invalid("%s: the crop leaves nothing to show", where)
			}
			if *c == (Crop{}) {
				e.Crop = nil
			}
		}
		if utf8.RuneCountInString(e.Alt) > 500 {
			return invalid("%s: alt text is at most 500 characters", where)
		}
		return notFor(e.Fill != "" || e.Stroke != "" || e.Text != nil || len(e.Paragraphs) > 0, "fill, stroke or text")
	default:
		return invalid("%s: type must be text, shape, line or image", where)
	}
}

// Parse decodes a deck strictly: unknown fields are refused, so a typo never
// silently drops content.
func Parse(body string) (Deck, error) {
	var d Deck
	dec := json.NewDecoder(strings.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&d); err != nil {
		return d, invalid("not a valid deck: %v", err)
	}
	if dec.More() {
		return d, invalid("not a valid deck: trailing data")
	}
	return d, nil
}

// Normalize validates a deck body and returns it in canonical form.
func Normalize(body string) (string, error) {
	d, err := Parse(body)
	if err != nil {
		return "", err
	}
	if err := d.Validate(); err != nil {
		return "", err
	}
	return d.Encode()
}

// Validate checks the whole deck and normalises it in place.
func (d *Deck) Validate() error {
	if d.Schema != Schema {
		return invalid("schema must be %d (got %d)", Schema, d.Schema)
	}
	if d.Size != (Size{Width, Height}) {
		return invalid("size must be %dx%d", Width, Height)
	}
	t := &d.Theme
	if n := utf8.RuneCountInString(t.Name); n > 60 {
		return invalid("theme name is at most 60 characters")
	}
	for _, k := range []string{"heading", "body"} {
		if !in(t.Fonts[k], Slots...) {
			return invalid("theme.fonts.%s must be sans, serif or mono", k)
		}
	}
	if len(t.Fonts) != 2 {
		return invalid("theme.fonts has exactly heading and body")
	}
	for _, k := range ThemeColors {
		if !hexColor.MatchString(t.Colors[k]) {
			return invalid("theme.colors.%s must be #RRGGBB", k)
		}
		t.Colors[k] = strings.ToUpper(t.Colors[k])
	}
	if len(t.Colors) != len(ThemeColors) {
		return invalid("theme.colors has exactly %s", strings.Join(ThemeColors, ", "))
	}
	if d.Assets == nil {
		d.Assets = map[string]Asset{}
	}
	for id, a := range d.Assets {
		if !AssetID.MatchString(id) {
			return invalid("asset id %q must look like a_ followed by 1–40 lowercase letters, digits or underscores", id)
		}
		if !in(a.Mime, "image/png", "image/jpeg") || a.W < 1 || a.H < 1 || a.W > 20000 || a.H > 20000 {
			return invalid("asset %s: needs mime image/png or image/jpeg and its pixel size", id)
		}
	}
	if len(d.Slides) == 0 {
		return invalid("a deck needs at least one slide")
	}
	if len(d.Slides) > MaxSlides {
		return invalid("a deck has at most %d slides", MaxSlides)
	}
	seen := map[string]bool{}
	for i := range d.Slides {
		s := &d.Slides[i]
		where := fmt.Sprintf("slide %d", i+1)
		if !slideID.MatchString(s.ID) {
			return invalid("%s: id %q must look like s_ followed by 1–40 lowercase letters, digits or underscores", where, s.ID)
		}
		if seen[s.ID] {
			return invalid("%s: id %s is used twice", where, s.ID)
		}
		seen[s.ID] = true
		if !in(s.Layout, Layouts...) {
			return invalid("%s: layout must be one of %s", where, strings.Join(Layouts, ", "))
		}
		if s.Background != nil {
			if err := checkColor(d.Theme, s.Background.Color, where+" background", true); err != nil {
				return err
			}
		}
		if utf8.RuneCountInString(s.Notes) > MaxNotesRunes {
			return invalid("%s: notes are at most %d characters", where, MaxNotesRunes)
		}
		if len(s.Elements) > MaxElements {
			return invalid("%s: at most %d elements", where, MaxElements)
		}
		if s.Elements == nil {
			s.Elements = []Element{}
		}
		for j := range s.Elements {
			e := &s.Elements[j]
			if seen[e.ID] {
				return invalid("%s element %d: id %s is used twice", where, j+1, e.ID)
			}
			seen[e.ID] = true
			if err := normalizeElement(d, e, fmt.Sprintf("%s element %d (%s)", where, j+1, e.ID)); err != nil {
				return err
			}
		}
	}
	return nil
}

// Encode is the canonical JSON of a validated deck.
func (d Deck) Encode() (string, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(d); err != nil {
		return "", err
	}
	return strings.TrimSuffix(b.String(), "\n"), nil
}
