package design

import "github.com/liamphmurphy/vesper/server/internal/apperr"

// Starters a new design can start from (ADR §3.9 design_create), in chooser order.
var Starters = []string{"blank", "mobile-flow", "web-flow", "diagram", "wireframe"}

func text(t string, size float64, weight float64, w float64) map[string]any {
	run := map[string]any{"text": t, "size": size}
	if weight != 400 {
		run["weight"] = weight
	}
	return map[string]any{"id": NewID("n"), "type": "text", "x": 0.0, "y": 0.0, "w": w, "h": size * 1.2,
		"sizing": map[string]any{"h": "hug"}, "paragraphs": []any{map[string]any{"runs": []any{run}}}}
}

func frame(name, preset string, x, w, h float64, kids ...any) map[string]any {
	f := map[string]any{"id": NewID("n"), "type": "frame", "name": name, "x": x, "y": 0.0, "w": w, "h": h,
		"fill": "#FFFFFF", "clip": true, "children": append([]any{}, kids...),
		"auto_layout": map[string]any{"direction": "vertical", "gap": 16.0, "padding": []any{24.0, 24.0, 24.0, 24.0}}}
	if preset != "" {
		f["preset"] = preset
	}
	return f
}

func box(name string, w, h float64) map[string]any {
	return map[string]any{"id": NewID("n"), "type": "shape", "shape": "roundRect", "name": name, "x": 0.0, "y": 0.0, "w": w, "h": h,
		"radius": 8.0, "fill": "#E8E8EE"}
}

func flow(preset string, w, h float64) []any {
	names := []string{"Start", "Details", "Done"}
	var out []any
	for i, n := range names {
		out = append(out, frame(n, preset, float64(i)*(w+120), w, h, text(n, 28, 700, w-48), text("Describe this step.", 16, 400, w-48), box("Button", w-48, 48)))
	}
	return out
}

func linksBetween(frames []any) []any {
	var out []any
	for i := 0; i+1 < len(frames); i++ {
		out = append(out, map[string]any{"id": NewID("k"), "from": frames[i].(map[string]any)["id"], "to": frames[i+1].(map[string]any)["id"],
			"trigger": "click", "transition": "slide-left"})
	}
	return out
}

// Starter builds a starter design with fresh ids; "" is blank.
func Starter(name string) (string, error) {
	page := map[string]any{"id": NewID("p"), "name": "Page 1", "children": []any{}}
	d := Doc{"schema": 1.0, "mode": "light", "token_sets": map[string]any{}, "assets": map[string]any{}, "links": []any{}, "pages": []any{page}}
	switch name {
	case "", "blank":
	case "mobile-flow":
		fs := flow("iphone-15", 393, 852)
		page["name"], page["children"], d["links"] = "Flow", fs, linksBetween(fs)
	case "web-flow":
		fs := flow("web", 1280, 800)
		page["name"], page["children"], d["links"] = "Flow", fs, linksBetween(fs)
	case "diagram":
		a, b, c := box("Start", 160, 64), box("Decide", 160, 64), box("Finish", 160, 64)
		b["shape"], b["x"] = "diamond", 280.0
		c["x"] = 560.0
		for _, s := range []map[string]any{a, b, c} {
			s["text"] = map[string]any{"valign": "middle", "paragraphs": []any{map[string]any{"align": "center", "runs": []any{map[string]any{"text": s["name"], "size": 16.0}}}}}
		}
		conn := func(x, y map[string]any) map[string]any {
			return map[string]any{"id": NewID("n"), "type": "connector", "from": map[string]any{"node": x["id"]}, "to": map[string]any{"node": y["id"]}, "route": "orthogonal", "end_arrow": "arrow"}
		}
		section := map[string]any{"id": NewID("n"), "type": "section", "name": "Diagram", "x": 0.0, "y": 0.0, "w": 800.0, "h": 240.0,
			"children": []any{a, b, c, conn(a, b), conn(b, c)}}
		for _, s := range []map[string]any{a, b, c} {
			s["y"] = 88.0
			s["x"] = s["x"].(float64) + 40
		}
		page["name"], page["children"] = "Diagram", []any{section}
	case "wireframe":
		page["name"] = "Wireframe"
		page["children"] = []any{frame("Home", "desktop", 0, 1440, 900, box("Header", 1392, 64), box("Hero", 1392, 360), box("Content", 1392, 280))}
	default:
		return "", apperr.Invalid("starter", "use one of blank, mobile-flow, web-flow, diagram or wireframe")
	}
	return Finish(d)
}
