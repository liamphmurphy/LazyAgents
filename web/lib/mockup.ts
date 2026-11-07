/** Pure helpers for UI mockups (SVG source). No DOM access at import time. */

type ParseDoc = {
  documentElement: { nodeName: string };
  getElementsByTagName(name: string): ArrayLike<{ textContent: string | null }>;
};
type Parser = { parseFromString(src: string, type: "image/svg+xml"): ParseDoc };

/**
 * Client-side well-formedness check. Returns an error message, or null when the
 * source parses and its root is <svg>. The server still enforces safety rules.
 */
export function svgError(source: string, parser: Parser | null = typeof DOMParser === "undefined" ? null : new DOMParser()): string | null {
  if (!source.trim()) return "The SVG is empty.";
  if (!parser) return null;
  const doc = parser.parseFromString(source, "image/svg+xml");
  const err = doc.getElementsByTagName("parsererror")[0];
  if (err) {
    // Browsers wrap the message in markup; keep the first meaningful line.
    const line = (err.textContent ?? "").split("\n").map((l) => l.trim()).find((l) => l && !/^this page contains/i.test(l));
    return line ? `Not well-formed SVG: ${line}` : "Not well-formed SVG.";
  }
  if (doc.documentElement.nodeName !== "svg") return "The root element must be <svg>.";
  return null;
}

export function escapeXml(s: string): string {
  return s.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;").replace(/"/g, "&quot;");
}

/** A blank 1280×800 wireframe: header bar, sidebar, content placeholders, labelled with the title. */
export function blankWireframe(title: string): string {
  const t = escapeXml(title.trim() || "Untitled mockup");
  const bar = (x: number, y: number, w: number) => `    <rect x="${x}" y="${y}" width="${w}" height="12" rx="6" fill="#D9DCE3"/>`;
  return `<svg xmlns="http://www.w3.org/2000/svg" width="1280" height="800" viewBox="0 0 1280 800" font-family="Inter, Helvetica, Arial, sans-serif">
  <rect width="1280" height="800" fill="#F6F7F9"/>
  <!-- Header -->
  <rect width="1280" height="64" fill="#FFFFFF"/>
  <line x1="0" y1="64" x2="1280" y2="64" stroke="#D9DCE3"/>
  <text x="24" y="40" font-size="20" font-weight="600" fill="#1F2430">${t}</text>
  <rect x="1124" y="18" width="132" height="28" rx="6" fill="#4547C9"/>
  <!-- Sidebar -->
  <rect y="65" width="240" height="735" fill="#FFFFFF"/>
  <line x1="240" y1="65" x2="240" y2="800" stroke="#D9DCE3"/>
  <g>
${bar(24, 96, 140)}
${bar(24, 132, 110)}
${bar(24, 168, 150)}
${bar(24, 204, 100)}
  </g>
  <!-- Content -->
  <rect x="272" y="96" width="420" height="28" rx="6" fill="#C5C9D3"/>
  <rect x="272" y="156" width="480" height="180" rx="12" fill="#FFFFFF" stroke="#D9DCE3"/>
  <rect x="776" y="156" width="480" height="180" rx="12" fill="#FFFFFF" stroke="#D9DCE3"/>
  <rect x="272" y="360" width="984" height="400" rx="12" fill="#FFFFFF" stroke="#D9DCE3"/>
  <g>
${bar(296, 392, 360)}
${bar(296, 432, 520)}
${bar(296, 472, 440)}
  </g>
</svg>
`;
}

export function isSvgFile(name: string, mime: string): boolean {
  return mime === "image/svg+xml" || /\.svg$/i.test(name);
}

/** Tab-key behaviour for a textarea: returns the new value and caret. */
export function insertIndent(value: string, start: number, end: number, indent = "  "): { value: string; caret: number } {
  return { value: value.slice(0, start) + indent + value.slice(end), caret: start + indent.length };
}

export type Zoom = "fit" | "100" | "200";

/**
 * SVG source as a base64 data: URL for an <img>. Not a blob: URL: those carry
 * the app's origin, so "Open image in new tab" would render the SVG top-level
 * on it; data: URLs get an opaque origin and browsers refuse to navigate to them.
 */
export function svgDataUrl(source: string): string {
  const bytes = new TextEncoder().encode(source);
  let bin = "";
  for (let i = 0; i < bytes.length; i += 0x8000) bin += String.fromCharCode(...bytes.subarray(i, i + 0x8000));
  return "data:image/svg+xml;base64," + btoa(bin);
}
