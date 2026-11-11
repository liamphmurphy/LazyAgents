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

// imageSlot is an empty image placeholder: a text element with the image
// role and no text, so it draws nothing in presentation, PDF and PPTX. The
// editor shows it as a "click to add image" box and swaps in an image
// element with the same frame and role.
func imageSlot(f Frame) Element {
	return textEl("image", f, "middle", para("", 40, false, "body", "muted", ""))
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
			imageSlot(Frame{1000, 120, 800, 840}),
		}
	case "section":
		return []Element{heading("", 128, Frame{160, 380, 1600, 320}, "middle", "center")}
	case "quote":
		q := para("", 72, false, "heading", "heading", "")
		q.Runs[0].Italic = true
		return []Element{
			textEl("body", Frame{200, 220, 1520, 480}, "bottom", q),
			textEl("subtitle", Frame{200, 740, 1520, 100}, "top", para("", 44, false, "body", "muted", "")),
		}
	case "big-number":
		return []Element{
			heading("", 280, Frame{160, 220, 1600, 420}, "bottom", "center"),
			textEl("subtitle", Frame{160, 680, 1600, 160}, "top", para("", 56, false, "body", "muted", "center")),
		}
	case "three-column":
		return []Element{heading("", 80, Frame{120, 80, 1680, 160}, "middle", ""), body("body", Frame{120, 280, 520, 700}), body("body2", Frame{700, 280, 520, 700}), body("body3", Frame{1280, 280, 520, 700})}
	case "comparison":
		return []Element{
			heading("", 80, Frame{120, 80, 1680, 160}, "middle", ""),
			textEl("label", Frame{120, 280, 810, 90}, "middle", para("", 52, true, "heading", "accent", "")),
			textEl("label2", Frame{990, 280, 810, 90}, "middle", para("", 52, true, "heading", "accent", "")),
			body("body", Frame{120, 390, 810, 590}),
			body("body2", Frame{990, 390, 810, 590}),
		}
	case "image-full":
		return []Element{
			imageSlot(Frame{0, 0, 1920, 1080}),
			textEl("title", Frame{120, 780, 1680, 200}, "bottom", para("", 96, true, "heading", "#FFFFFF", "")),
		}
	case "image-left":
		return []Element{
			imageSlot(Frame{120, 120, 800, 840}),
			heading("", 72, Frame{1000, 120, 800, 300}, "top", ""),
			textEl("body", Frame{1000, 460, 800, 480}, "top", para("", 40, false, "body", "text", "")),
		}
	case "agenda":
		return []Element{
			heading("", 80, Frame{120, 80, 1680, 160}, "middle", ""),
			textEl("body", Frame{120, 280, 1680, 700}, "top", Paragraph{Bullet: "number", Runs: []Run{{Text: "", Size: 52, Font: "body", Color: "text"}}}),
		}
	case "closing":
		if title == "" {
			title = "Thank you"
		}
		return []Element{
			heading(title, 128, Frame{160, 300, 1600, 280}, "bottom", "center"),
			textEl("subtitle", Frame{160, 620, 1600, 140}, "top", para("", 44, false, "body", "muted", "center")),
		}
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
