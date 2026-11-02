export function formatBytes(n: number): string {
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${Math.round(n / 1024)} KB`;
  return `${(n / 1024 / 1024).toFixed(1)} MB`;
}

/** "PDF", "PNG", "SVG" from a content type. */
export function fileKind(contentType: string, filename?: string): string {
  const ext = filename?.split(".").pop();
  if (ext && ext !== filename) return ext.toUpperCase();
  return (contentType.split("/")[1] ?? "file").split(/[+;]/)[0].toUpperCase();
}
