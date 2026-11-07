// Pure rules for chat attachments (LAZYAGENTS-72), checked on the client before an
// upload; the server validates again. Kept free of React so `npm run test:unit` covers it.

export const MAX_ATTACHMENT_BYTES = 25 * 1024 * 1024;
export const MAX_ATTACHMENTS = 5;

const EXTENSIONS = ["pdf", "png", "jpg", "jpeg", "gif", "webp", "txt", "md", "csv", "json"];
/** The file input's `accept`. */
export const ATTACH_ACCEPT = EXTENSIONS.map((e) => `.${e}`).join(",");

const ext = (name: string) => (name.includes(".") ? (name.split(".").pop() ?? "").toLowerCase() : "");

/** Why a file can't be attached, or undefined when it can. Empty files are the server's call too, but never worth uploading. */
export function attachmentProblem(file: { name: string; size: number }): string | undefined {
  if (!EXTENSIONS.includes(ext(file.name))) return "type not supported";
  if (file.size === 0) return "file is empty";
  if (file.size > MAX_ATTACHMENT_BYTES) return "over 25 MB";
  return undefined;
}

/** The chip's badge: PDF, IMG, CSV, TXT, MD or JSON. */
export function attachmentBadge(filename: string): string {
  const e = ext(filename);
  if (["png", "jpg", "jpeg", "gif", "webp"].includes(e)) return "IMG";
  return EXTENSIONS.includes(e) ? e.toUpperCase() : "FILE";
}
