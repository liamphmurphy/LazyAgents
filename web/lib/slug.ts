// Must match server/internal/artifacts/outline.go Slug + Outline anchors, so
// headings the UI renders get the same ids the outline links to.

export function slug(s: string): string {
  let out = "";
  let dash = false;
  for (const ch of s.toLowerCase()) {
    if (/[\p{L}\p{Nd}]/u.test(ch)) {
      if (dash && out) out += "-";
      out += ch;
      dash = false;
    } else {
      dash = true;
    }
  }
  return out || "section";
}

/** Dedupes anchors in document order: notes, notes-2, notes-3. */
export function anchorer() {
  const seen = new Map<string, number>();
  return (text: string) => {
    const base = slug(text);
    const n = seen.get(base) ?? 0;
    seen.set(base, n + 1);
    return n ? `${base}-${n + 1}` : base;
  };
}

const ATX = /^ {0,3}(#{1,6})(?:[ \t]+(.*?))?(?:[ \t]+#+)?[ \t]*$/;
const FENCE = /^ {0,3}(```|~~~)/;
// Same stripping as the server: emphasis, code ticks and strikethrough marks
// go, link text stays, underscores stay.
const INLINE = /[*`~]|\[([^\]]*)\]\([^)]*\)/g;

/**
 * Heading anchors by 1-based source line, mirroring the server's Outline()
 * so in-page links match the outline the API returns.
 */
export function headingAnchors(md: string): Map<number, string> {
  const out = new Map<number, string>();
  const next = anchorer();
  let fence = "";
  md.split("\n").forEach((raw, i) => {
    const line = raw.replace(/\r$/, "");
    const f = FENCE.exec(line);
    if (f) {
      if (!fence) fence = f[1];
      else if (fence === f[1]) fence = "";
      return;
    }
    if (fence) return;
    const m = ATX.exec(line);
    if (!m) return;
    const text = (m[2] ?? "").replace(INLINE, (_all, linkText) => linkText ?? "").trim();
    if (text) out.set(i + 1, next(text));
  });
  return out;
}
