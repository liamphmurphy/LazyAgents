// A ~2k-node design for the pan/zoom test: 40 phone frames in a grid, each an
// auto-layout list of 12 rows (row frame + avatar + two text nodes), wired in
// order by connectors. Every node goes through the real layout and SVG paths.
export function largeDesign(base, frames = 40, rows = 12) {
  const children = [];
  const cols = 8;
  for (let f = 0; f < frames; f++) {
    const fx = (f % cols) * 473, fy = Math.floor(f / cols) * 952;
    const kids = [
      { id: `n_f${f}_title`, type: "text", name: "Title", x: 24, y: 64, w: 345, h: 31, sizing: { w: "fill", h: "hug" },
        paragraphs: [{ runs: [{ text: `Screen ${f + 1}`, size: 26, weight: 600, color: "$text" }] }] },
    ];
    for (let r = 0; r < rows; r++) {
      const id = `n_f${f}_r${r}`;
      kids.push({
        id, type: "frame", name: `Row ${r + 1}`, x: 24, y: 0, w: 345, h: 52, sizing: { w: "fill", h: "hug" },
        fill: "$surface", radius: 10,
        auto_layout: { direction: "horizontal", gap: 12, padding: [8, 12, 8, 12], align: "center", justify: "start" },
        children: [
          { id: `${id}_av`, type: "shape", shape: "ellipse", name: "Avatar", x: 0, y: 0, w: 36, h: 36, fill: r % 2 ? "$agent-quill" : "$agent-atlas" },
          { id: `${id}_name`, type: "text", name: "Name", x: 0, y: 0, w: 150, h: 18, sizing: { w: "fill", h: "hug" },
            paragraphs: [{ runs: [{ text: `Item ${r + 1} on screen ${f + 1}`, size: 15, weight: 500, color: "$text" }] }] },
          { id: `${id}_meta`, type: "text", name: "Meta", x: 0, y: 0, w: 60, h: 16, sizing: { w: "hug", h: "hug" },
            paragraphs: [{ runs: [{ text: `${(r * 7 + f) % 59}m ago`, size: 13, color: "$text-tertiary" }] }] },
        ],
      });
    }
    children.push({
      id: `n_f${f}`, type: "frame", name: `Screen ${f + 1}`, preset: "iphone-15", x: fx, y: fy, w: 393, h: 852, fill: "$bg-panel", clip: true,
      auto_layout: { direction: "vertical", gap: 8, padding: [64, 24, 32, 24], align: "start", justify: "start" },
      children: kids,
    });
    if (f > 0 && f % cols) children.push({ id: `n_w${f}`, type: "connector", from: { node: `n_f${f - 1}`, anchor: "right" }, to: { node: `n_f${f}`, anchor: "left" }, route: "orthogonal", stroke: "$accent", stroke_width: 2, end_arrow: "arrow" });
  }
  return { ...base, pages: [{ id: "p_large", name: "Large", background: "$bg", children }], links: [] };
}
