package deck

import (
	"slices"
	"testing"
)

// Every built-in theme's text, heading and muted colours meet WCAG AA (4.5:1)
// on both its background and its surface.
func TestBuiltInThemesMeetAAContrast(t *testing.T) {
	if len(Themes) < 10 || len(ThemeOrder) != len(Themes) {
		t.Fatalf("%d themes, %d in ThemeOrder; want at least 10, all ordered", len(Themes), len(ThemeOrder))
	}
	for _, key := range ThemeOrder {
		th, ok := Themes[key]
		if !ok {
			t.Fatalf("ThemeOrder names unknown theme %q", key)
		}
		if len(th.Colors) != len(ThemeColors) || !slices.Contains([]string{"sans", "serif", "mono"}, th.Fonts["heading"]) || !slices.Contains([]string{"sans", "serif", "mono"}, th.Fonts["body"]) {
			t.Errorf("%s: incomplete colours or fonts", key)
		}
		for _, fg := range []string{"text", "heading", "muted"} {
			for _, bg := range []string{"background", "surface"} {
				if r := Contrast(th.Colors[fg], th.Colors[bg]); r < 4.5 {
					t.Errorf("%s: %s on %s is %.2f:1, below 4.5:1", key, fg, bg, r)
				}
			}
		}
	}
}

func TestContrast(t *testing.T) {
	if r := Contrast("#000000", "#FFFFFF"); r < 20.99 || r > 21.01 {
		t.Fatalf("black on white = %v", r)
	}
	if r := Contrast("#777777", "#FFFFFF"); r < 4.47 || r > 4.49 {
		t.Fatalf("#777 on white = %v", r)
	}
}
