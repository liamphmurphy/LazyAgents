package deck

import (
	"encoding/json"
	"math"
	"strconv"
	"strings"
)

// Themes are the built-in themes. The web app's theme picker lists the same
// names and colours (web/lib/slides/themes.ts).
var Themes = map[string]Theme{
	"light": {Name: "Light", Fonts: map[string]string{"heading": "sans", "body": "sans"},
		Colors: map[string]string{"background": "#FFFFFF", "text": "#1F2328", "heading": "#111418", "accent": "#3B6FE0", "accent2": "#E8A33D", "muted": "#5F6673", "surface": "#F3F5F9"}},
	"dark": {Name: "Dark", Fonts: map[string]string{"heading": "sans", "body": "sans"},
		Colors: map[string]string{"background": "#14151A", "text": "#E6E7EB", "heading": "#FFFFFF", "accent": "#8B9CFF", "accent2": "#F2B35E", "muted": "#9AA0AE", "surface": "#1F2129"}},
	"paper": {Name: "Paper", Fonts: map[string]string{"heading": "serif", "body": "serif"},
		Colors: map[string]string{"background": "#FBF8F1", "text": "#2B2723", "heading": "#1A1612", "accent": "#A2462E", "accent2": "#3F6B5C", "muted": "#655D53", "surface": "#F1EBDD"}},
	"contrast": {Name: "Contrast", Fonts: map[string]string{"heading": "sans", "body": "serif"},
		Colors: map[string]string{"background": "#FFFFFF", "text": "#000000", "heading": "#000000", "accent": "#D7263D", "accent2": "#1B998B", "muted": "#4A4A4A", "surface": "#F0F0F0"}},
	"dusk": {Name: "Dusk", Fonts: map[string]string{"heading": "sans", "body": "sans"},
		Colors: map[string]string{"background": "#1B1830", "text": "#E4E0F5", "heading": "#FFFFFF", "accent": "#B79CFF", "accent2": "#F4A261", "muted": "#A9A3C4", "surface": "#262240"}},
	"ocean": {Name: "Ocean", Fonts: map[string]string{"heading": "sans", "body": "sans"},
		Colors: map[string]string{"background": "#0B2540", "text": "#D6E4F0", "heading": "#FFFFFF", "accent": "#4CC9F0", "accent2": "#FFD166", "muted": "#9DB4CB", "surface": "#13355A"}},
	"forest": {Name: "Forest", Fonts: map[string]string{"heading": "serif", "body": "sans"},
		Colors: map[string]string{"background": "#F3F6EF", "text": "#1E2A20", "heading": "#14301B", "accent": "#2F6B3A", "accent2": "#B5651D", "muted": "#56635A", "surface": "#E3EADC"}},
	"sunset": {Name: "Sunset", Fonts: map[string]string{"heading": "sans", "body": "sans"},
		Colors: map[string]string{"background": "#FFF4EC", "text": "#3A1F2B", "heading": "#5A1A33", "accent": "#D1495B", "accent2": "#7B4FA0", "muted": "#7A5560", "surface": "#FBE3D3"}},
	"mono": {Name: "Mono", Fonts: map[string]string{"heading": "mono", "body": "sans"},
		Colors: map[string]string{"background": "#FAFAFA", "text": "#1A1A1A", "heading": "#000000", "accent": "#111111", "accent2": "#777777", "muted": "#5C5C5C", "surface": "#EDEDED"}},
	"blueprint": {Name: "Blueprint", Fonts: map[string]string{"heading": "mono", "body": "sans"},
		Colors: map[string]string{"background": "#123A6B", "text": "#E8F1FB", "heading": "#FFFFFF", "accent": "#9BD1FF", "accent2": "#FFE08A", "muted": "#B4CBE6", "surface": "#1B4A82"}},
}

// ThemeOrder is the order the theme menu lists the built-in themes in.
var ThemeOrder = []string{"light", "dark", "paper", "contrast", "dusk", "ocean", "forest", "sunset", "mono", "blueprint"}

// ThemeNamed is a copy of a built-in theme; light when the name is unknown.
func ThemeNamed(name string) Theme {
	t, ok := Themes[strings.ToLower(strings.TrimSpace(name))]
	if !ok {
		t = Themes["light"]
	}
	c := Theme{Name: t.Name, Fonts: map[string]string{}, Colors: map[string]string{}}
	for k, v := range t.Fonts {
		c.Fonts[k] = v
	}
	for k, v := range t.Colors {
		c.Colors[k] = v
	}
	return c
}

func para(text string, size float64, bold bool, font, color, align string) Paragraph {
	return Paragraph{Align: align, Runs: []Run{{Text: text, Size: size, Bold: bold, Font: font, Color: color}}}
}

func textEl(placeholder string, f Frame, valign string, ps ...Paragraph) Element {
	return Element{ID: NewID("e"), Type: "text", Placeholder: placeholder, Frame: f, VAlign: valign, Paragraphs: ps}
}

// Placeholders are the elements a layout starts with. Text is the prompt the
// editor shows until the person types; title is used for the title slide.
func Placeholders(layout, title string) []Element {
	heading := func(t string, size float64, f Frame, valign, align string) Element {
		return textEl("title", f, valign, para(t, size, true, "heading", "heading", align))
	}
	body := func(p string, f Frame) Element {
		return textEl(p, f, "top", Paragraph{Bullet: "disc", Runs: []Run{{Text: "", Size: 44, Font: "body", Color: "text"}}})
	}
	switch layout {
	case "title":
		if title == "" {
			title = "Title"
		}
		return []Element{
			heading(title, 112, Frame{160, 300, 1600, 280}, "bottom", ""),
			textEl("subtitle", Frame{160, 600, 1600, 120}, "top", para("", 44, false, "body", "muted", "")),
		}
	case "title-body":
		return []Element{heading("", 80, Frame{120, 80, 1680, 160}, "middle", ""), body("body", Frame{120, 280, 1680, 700})}
	case "two-column":
		return []Element{heading("", 80, Frame{120, 80, 1680, 160}, "middle", ""), body("body", Frame{120, 280, 810, 700}), body("body2", Frame{990, 280, 810, 700})}
	case "image-right":
		return []Element{
			heading("", 72, Frame{120, 120, 800, 300}, "top", ""),
			textEl("body", Frame{120, 460, 800, 480}, "top", para("", 40, false, "body", "text", "")),
		}
	case "section":
		return []Element{heading("", 128, Frame{160, 380, 1600, 320}, "middle", "center")}
	}
	return []Element{}
}

// NewSlide is an empty slide with a layout's placeholders.
func NewSlide(layout, title string) Slide {
	if !in(layout, Layouts...) {
		layout = "title-body"
	}
	return Slide{ID: NewID("s"), Layout: layout, Elements: Placeholders(layout, title)}
}

// New is a deck with one title slide.
func New(title, theme string) Deck {
	return Deck{Schema: Schema, Size: Size{Width, Height}, Theme: ThemeNamed(theme), Assets: map[string]Asset{}, Slides: []Slide{NewSlide("title", title)}}
}

// Contrast is the WCAG contrast ratio of two #RRGGBB colours (1 to 21).
func Contrast(a, b string) float64 {
	la, lb := luminance(a), luminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

func luminance(hex string) float64 {
	var rgb [3]float64
	for i := range rgb {
		v, _ := strconv.ParseUint(hex[1+2*i:3+2*i], 16, 8)
		c := float64(v) / 255
		if c <= 0.03928 {
			c /= 12.92
		} else {
			c = math.Pow((c+0.055)/1.055, 2.4)
		}
		rgb[i] = c
	}
	return 0.2126*rgb[0] + 0.7152*rgb[1] + 0.0722*rgb[2]
}

// SavedTheme is a theme a person saved, from its stored name, fonts and colours.
func SavedTheme(name string, fonts, colors []byte) (Theme, error) {
	t := Theme{Name: name}
	if err := json.Unmarshal(fonts, &t.Fonts); err != nil {
		return t, err
	}
	if err := json.Unmarshal(colors, &t.Colors); err != nil {
		return t, err
	}
	return t, t.Validate()
}
