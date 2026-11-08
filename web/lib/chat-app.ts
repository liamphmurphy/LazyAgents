// Interactive apps in chat: an ```app fence holds a self-contained HTML/JS
// document that the chat runs in a sandboxed iframe (components/domain/app-fence).

/** Fence info words that render as a live app instead of code. */
const APP_FENCES = new Set(["app", "html-app"]);

export const isAppFence = (info: string | undefined) => APP_FENCES.has((info ?? "").trim().split(/\s+/)[0]?.toLowerCase() ?? "");

/** CDNs an app may load scripts and styles from. Nothing else on the network is reachable. */
export const APP_CDNS = ["https://cdn.jsdelivr.net", "https://unpkg.com", "https://cdnjs.cloudflare.com"];

/**
 * The app's Content-Security-Policy. No fetch, XHR, WebSocket, form posts or
 * remote images, so an app can't send what the person types anywhere; inline
 * code and the CDNs above are allowed so apps can use libraries like Chart.js.
 */
export const APP_CSP = [
  "default-src 'none'",
  `script-src 'unsafe-inline' 'unsafe-eval' ${APP_CDNS.join(" ")}`,
  `style-src 'unsafe-inline' ${APP_CDNS.join(" ")}`,
  `font-src data: ${APP_CDNS.join(" ")}`,
  "img-src data: blob:",
  "media-src data: blob:",
  "connect-src 'none'",
  "form-action 'none'",
  "base-uri 'none'",
].join("; ");

/** Message type the app's frame posts its height with. */
export const APP_RESIZE = "vesper-app:resize";

export const APP_MIN_HEIGHT = 120;
export const APP_MAX_HEIGHT = 720;

/** Clamps a reported content height to the frame's bounds. */
export const appHeight = (h: unknown) =>
  typeof h === "number" && Number.isFinite(h) ? Math.min(APP_MAX_HEIGHT, Math.max(APP_MIN_HEIGHT, Math.ceil(h))) : APP_MIN_HEIGHT;

const escapeAttr = (s: string) => s.replace(/&/g, "&amp;").replace(/"/g, "&quot;");

const BASE_STYLE = `<style>:root{color-scheme:light}html,body{margin:0;background:#fff;color:#111}body{padding:12px;font:14px/1.45 system-ui,-apple-system,sans-serif}</style>`;

// Reports the document's height to the chat so the frame fits its content.
const RESIZE_SCRIPT = `<script>(()=>{let last=0;const post=()=>{const h=Math.ceil(document.documentElement.getBoundingClientRect().height);if(h!==last){last=h;parent.postMessage({type:${JSON.stringify(APP_RESIZE)},height:h},"*")}};addEventListener("load",post);new ResizeObserver(post).observe(document.documentElement);post()})()</script>`;

/**
 * The iframe srcdoc for an app's source. The CSP meta comes first, before any
 * of the app's own markup, so it applies to every script the app runs (a later
 * policy the app adds can only narrow it). A leading doctype is dropped and
 * re-added ahead of it, so the document stays in standards mode.
 */
export function appDocument(source: string): string {
  const body = source.replace(/^\s*<!doctype[^>]*>/i, "");
  return `<!doctype html><meta charset="utf-8"><meta http-equiv="Content-Security-Policy" content="${escapeAttr(APP_CSP)}"><meta name="viewport" content="width=device-width,initial-scale=1">${BASE_STYLE}${RESIZE_SCRIPT}${body}`;
}
