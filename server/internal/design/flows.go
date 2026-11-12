package design

import (
	"fmt"
	"strings"
)

// FlowFrame is a screen of the prototype: a top-level frame, or the
// top-level node holding a hotspot that isn't inside a frame.
type FlowFrame struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Page string `json:"page"`
	Type string `json:"type"`
}

// FlowLink is a prototype link, with the screen its hotspot sits on.
type FlowLink struct {
	ID         string `json:"id"`
	From       string `json:"from"`
	FromName   string `json:"from_name,omitempty"`
	Screen     string `json:"screen"`
	To         string `json:"to"`
	Trigger    string `json:"trigger"`
	DelayMS    *int   `json:"delay_ms,omitempty"`
	Transition string `json:"transition"`
	DurationMS *int   `json:"duration_ms,omitempty"`
}

// Flows is design_flows' answer (ADR-designs §3.9): the prototype graph as
// JSON and as a Mermaid flowchart. Broken links (whose target was removed)
// are listed by id and left out of the graph, as Present skips them.
type Flows struct {
	Frames  []FlowFrame `json:"frames"`
	Links   []FlowLink  `json:"links"`
	Broken  []string    `json:"broken_links,omitempty"`
	Mermaid string      `json:"mermaid"`
}

// FlowsOf builds the prototype graph of a normalised design. With page (an
// id or a name), only that page's frames and the links leaving them.
func FlowsOf(d Doc, page string) (Flows, error) {
	out := Flows{Frames: []FlowFrame{}, Links: []FlowLink{}}
	if page != "" {
		found := false
		for _, p := range pages(d) {
			if p["id"] == page || p["name"] == page {
				page, found = p["id"].(string), true
				break
			}
		}
		if !found {
			return out, invalid("page %s doesn't exist", page)
		}
	}
	type spot struct {
		node   map[string]any
		screen map[string]any
		page   string
	}
	where := map[string]spot{}
	for _, p := range pages(d) {
		pid := p["id"].(string)
		for _, top := range children(p) {
			var mark func(n map[string]any)
			mark = func(n map[string]any) {
				where[n["id"].(string)] = spot{node: n, screen: top, page: pid}
				for _, c := range children(n) {
					mark(c)
				}
			}
			mark(top)
		}
	}
	screens := map[string]bool{}
	addScreen := func(n map[string]any, pid string) {
		id := n["id"].(string)
		if screens[id] {
			return
		}
		screens[id] = true
		name, _ := n["name"].(string)
		out.Frames = append(out.Frames, FlowFrame{ID: id, Name: name, Page: pid, Type: n["type"].(string)})
	}
	// Every top-level frame of the page(s) is a screen, linked or not, in page order.
	for _, p := range pages(d) {
		pid := p["id"].(string)
		if page != "" && pid != page {
			continue
		}
		for _, n := range children(p) {
			if n["type"] == "frame" {
				addScreen(n, pid)
			}
		}
	}
	for _, l := range links(d) {
		id := l["id"].(string)
		from, ok := where[l["from"].(string)]
		to, okTo := where[l["to"].(string)]
		if page != "" && (!ok || from.page != page) {
			continue
		}
		if !ok || !okTo {
			out.Broken = append(out.Broken, id)
			continue
		}
		addScreen(from.screen, from.page)
		// A target on another page shows up as a screen of the graph too.
		addScreen(to.node, to.page)
		fl := FlowLink{ID: id, From: l["from"].(string), Screen: from.screen["id"].(string), To: l["to"].(string),
			Trigger: str0(l["trigger"]), Transition: str0(l["transition"]), DelayMS: intPtr(l["delay_ms"]), DurationMS: intPtr(l["duration_ms"])}
		fl.FromName, _ = from.node["name"].(string)
		out.Links = append(out.Links, fl)
	}
	out.Mermaid = flowsMermaid(out)
	return out, nil
}

func str0(v any) string { s, _ := v.(string); return s }

func intPtr(v any) *int {
	f, ok := v.(float64)
	if !ok {
		return nil
	}
	i := int(f)
	return &i
}

// mermaidText quotes a label for Mermaid: in double quotes, with # and
// quotes as entities (Mermaid reads "#…;" as one) and line breaks as spaces.
func mermaidText(s string) string {
	s = strings.NewReplacer("#", "#35;", `"`, "#quot;", "\n", " ", "\r", " ").Replace(s)
	return `"` + s + `"`
}

func flowsMermaid(f Flows) string {
	var b strings.Builder
	b.WriteString("flowchart LR\n")
	for _, s := range f.Frames {
		name := s.Name
		if name == "" {
			name = s.ID
		}
		fmt.Fprintf(&b, "  %s[%s]\n", s.ID, mermaidText(name))
	}
	for _, l := range f.Links {
		label := l.Trigger
		if l.FromName != "" {
			label = l.FromName + ": " + label
		}
		if l.Trigger == "after-delay" && l.DelayMS != nil {
			label += fmt.Sprintf(" %d ms", *l.DelayMS)
		}
		label += " · " + l.Transition
		fmt.Fprintf(&b, "  %s -->|%s| %s\n", l.Screen, mermaidText(label), l.To)
	}
	return strings.TrimSuffix(b.String(), "\n")
}
