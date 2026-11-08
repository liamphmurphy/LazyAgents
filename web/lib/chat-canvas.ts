// Canvas cards in chat: a ```canvas fence holds the id of a canvas in this chat
// (components/domain/canvas-fence). Kept free of React so `npm run test:unit` covers it.

export const isCanvasFence = (info: string | undefined) => (info ?? "").trim().split(/\s+/)[0]?.toLowerCase() === "canvas";

const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

const FENCE = /^ {0,3}(?:`{3,}|~{3,})[ \t]*canvas\b[^\n]*\n\s*([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})\s*$/gim;

/** The canvases a message has a card for, in order. */
export function canvasFenceIds(md: string): string[] {
  return [...md.matchAll(FENCE)].map((m) => m[1].toLowerCase());
}

/** The canvas id in a fence, or null when it holds anything else. */
export function parseCanvasFence(code: string): string | null {
  const id = code.trim();
  return UUID.test(id) ? id.toLowerCase() : null;
}
