package deck

import "strings"

// Themes are the built-in themes. The web app's theme picker lists the same
// names and colours (web/lib/slides/themes.ts).
var Themes = map[string]Theme{
	"light": {Name: "Light", Fonts: map[string]string{"heading": "sans", "body": "sans"},
		Colors: map[string]string{"background": "#FFFFFF", "text": "#1F2328", "heading": "#111418", "accent": "#3B6FE0", "accent2": "#E8A33D", "muted": "#6B7280", "surface": "#F3F5F9"}},
	"dark": {Name: "Dark", Fonts: map[string]string{"heading": "sans", "body": "sans"},
		Colors: map[string]string{"background": "#14151A", "text": "#E6E7EB", "heading": "#FFFFFF", "accent": "#8B9CFF", "accent2": "#F2B35E", "muted": "#9AA0AE", "surface": "#1F2129"}},
	"paper": {Name: "Paper", Fonts: map[string]string{"heading": "serif", "body": "serif"},
		Colors: map[string]string{"background": "#FBF8F1", "text": "#2B2723", "heading": "#1A1612", "accent": "#A2462E", "accent2": "#3F6B5C", "muted": "#7A7268", "surface": "#F1EBDD"}},
	"contrast": {Name: "Contrast", Fonts: map[string]string{"heading": "sans", "body": "serif"},
		Colors: map[string]string{"background": "#FFFFFF", "text": "#000000", "heading": "#000000", "accent": "#D7263D", "accent2": "#1B998B", "muted": "#4A4A4A", "surface": "#F0F0F0"}},
}

// ThemeNamed is a copy of a built-in theme; light when the name is unknown.
func ThemeNamed(name string) Theme {
	t, ok := Themes[strings.ToLower(name)]
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
