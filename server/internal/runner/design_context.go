package runner

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/liamphmurphy/vesper/server/internal/design"
	"github.com/liamphmurphy/vesper/server/internal/designs"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

// designContextTimeout bounds rendering and inspecting a run's linked frames.
const designContextTimeout = 90 * time.Second

// DesignRenderer draws and inspects design frames (designs.Service).
type DesignRenderer interface {
	Render(ctx context.Context, canvasID uuid.UUID, body, node, page string, scale float64) (designs.Rendered, error)
	Inspect(ctx context.Context, body, node, target string) (map[string]any, error)
}

// DesignFrameFiles are the design frames linked to a ticket (and, for an epic
// review, to any ticket of its epic), ADR-designs §3.11: for each, the frame
// as a PNG at 1× and a markdown file with its Inspect output (React) and
// linked code paths. At most 10 frames; a frame that can't be drawn or
// inspected gets a note in its markdown instead, and never fails the run.
func DesignFrameFiles(ctx context.Context, q *store.Queries, dr DesignRenderer, t store.Ticket) ([]ContextFile, error) {
	p := store.ListTicketDesignFramesParams{TicketID: t.ID}
	if t.EpicReview {
		p.EpicID = t.EpicID
	}
	rows, err := q.ListTicketDesignFrames(ctx, p)
	if err != nil || len(rows) == 0 {
		return nil, err
	}
	var out []ContextFile
	seen := map[string]bool{}
	for _, r := range rows {
		n, ok := design.NodeOf(r.Body, r.NodeID)
		if !ok {
			continue // the link outlived the node; the next save drops it
		}
		name, _ := n["name"].(string)
		if name == "" {
			name = r.NodeID
		}
		base := ".lazyagents/context/designs/" + slugPart(r.Title) + "-" + slugPart(name)
		for i := 2; seen[base]; i++ {
			base = fmt.Sprintf(".lazyagents/context/designs/%s-%s-%d", slugPart(r.Title), slugPart(name), i)
		}
		seen[base] = true

		var md strings.Builder
		fmt.Fprintf(&md, "# Design frame: %s\n\nFrom the design \"%s\" (canvas %s, node %s), linked to this ticket in the Designs app. Build what it shows; use the tokens it names rather than hex values.\n\n", name, r.Title, r.CanvasID, r.NodeID)
		if dr == nil {
			md.WriteString("> The design renderer isn't configured on this runner, so there's no PNG or Inspect output for this frame.\n")
			out = append(out, ContextFile{Path: base + ".md", Body: []byte(md.String())})
			continue
		}
		if img, err := dr.Render(ctx, r.CanvasID, r.Body, r.NodeID, "", 1); err != nil {
			fmt.Fprintf(&md, "> The frame couldn't be drawn (%s), so there's no PNG; work from the Inspect output below.\n\n", oneLine(err.Error()))
		} else {
			out = append(out, ContextFile{Path: base + ".png", Body: img.PNG})
			fmt.Fprintf(&md, "Image: `%s.png` (%d×%d at 1×).\n\n", base, img.W, img.H)
		}
		if paths, _ := n["linked_paths"].([]any); len(paths) > 0 {
			md.WriteString("Code it maps to:\n")
			for _, p := range paths {
				fmt.Fprintf(&md, "- `%v`\n", p)
			}
			md.WriteString("\n")
		}
		ins, err := dr.Inspect(ctx, r.Body, r.NodeID, "react")
		if err != nil {
			fmt.Fprintf(&md, "> Inspect output is unavailable (%s).\n", oneLine(err.Error()))
		} else {
			writeInspect(&md, ins)
		}
		out = append(out, ContextFile{Path: base + ".md", Body: []byte(md.String())})
	}
	return out, nil
}

func slugPart(s string) string {
	s = strings.Trim(fileSlug.ReplaceAllString(strings.ToLower(s), "-"), "-")
	if len(s) > 40 {
		s = strings.TrimRight(s[:40], "-")
	}
	if s == "" {
		s = "design"
	}
	return s
}

func oneLine(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > 200 {
		s = s[:200] + "…"
	}
	return s
}

func writeInspect(b *strings.Builder, ins map[string]any) {
	if box, ok := ins["box"].(map[string]any); ok {
		fmt.Fprintf(b, "Size: %v×%v px. Mode: %v.\n\n", box["w"], box["h"], ins["mode"])
	}
	if sp, ok := ins["spacing"].(map[string]any); ok {
		fmt.Fprintf(b, "Auto layout: %v, gap %v, padding %v (top, right, bottom, left), align %v, justify %v.\n\n", sp["direction"], sp["gap"], sp["padding"], sp["align"], sp["justify"])
	}
	if toks, ok := ins["tokens"].([]any); ok && len(toks) > 0 {
		b.WriteString("## Tokens\n\n| Token | Value | Used for |\n|---|---|---|\n")
		for _, t := range toks {
			m, _ := t.(map[string]any)
			uses, _ := m["used_for"].([]any)
			parts := make([]string, 0, len(uses))
			for _, u := range uses {
				parts = append(parts, fmt.Sprint(u))
			}
			fmt.Fprintf(b, "| `--%v` | %v | %s |\n", m["name"], orDash(m["value"]), strings.Join(parts, ", "))
		}
		b.WriteString("\n")
	}
	if types, ok := ins["type"].([]any); ok && len(types) > 0 {
		b.WriteString("## Type\n\n")
		for _, t := range types {
			m, _ := t.(map[string]any)
			style := "normal"
			if m["italic"] == true {
				style = "italic"
			}
			tok := ""
			if m["token"] != nil {
				tok = fmt.Sprintf(" (`--%v`)", m["token"])
			}
			fmt.Fprintf(b, "- \"%v\": Inter %vpx, weight %v, %s, line height %vpx, %v%s\n", m["text"], m["size"], m["weight"], style, m["line_height"], m["color"], tok)
		}
		b.WriteString("\n")
	}
	if assets, ok := ins["assets"].([]any); ok && len(assets) > 0 {
		b.WriteString("## Images\n\n")
		for _, a := range assets {
			m, _ := a.(map[string]any)
			fmt.Fprintf(b, "- %v: %v, %v×%v, alt \"%v\"\n", m["id"], m["mime"], m["w"], m["h"], orDash(m["alt"]))
		}
		b.WriteString("\n")
	}
	if code, ok := ins["code"].(string); ok {
		fence := "```"
		for strings.Contains(code, fence) {
			fence += "`"
		}
		b.WriteString("## React (Tailwind)\n\n" + fence + "tsx\n" + code + fence + "\n")
	}
}

func orDash(v any) any {
	if v == nil {
		return "—"
	}
	return v
}
