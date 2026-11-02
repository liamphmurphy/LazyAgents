export type DiffLine = { op: "same" | "add" | "del"; text: string };

/**
 * Line diff (LCS) for reviewing suggested page edits. Pages are small, so
 * the O(n·m) table is fine; past ~4M cells it falls back to "all replaced".
 */
export function diffLines(a: string, b: string): DiffLine[] {
  const x = a === "" ? [] : a.replace(/\n$/, "").split("\n");
  const y = b === "" ? [] : b.replace(/\n$/, "").split("\n");
  const n = x.length;
  const m = y.length;
  if (n * m > 4_000_000) {
    return [...x.map((text) => ({ op: "del" as const, text })), ...y.map((text) => ({ op: "add" as const, text }))];
  }
  const w = m + 1;
  const t = new Uint32Array((n + 1) * w);
  for (let i = n - 1; i >= 0; i--) {
    for (let j = m - 1; j >= 0; j--) {
      t[i * w + j] = x[i] === y[j] ? t[(i + 1) * w + j + 1] + 1 : Math.max(t[(i + 1) * w + j], t[i * w + j + 1]);
    }
  }
  const out: DiffLine[] = [];
  let i = 0;
  let j = 0;
  while (i < n && j < m) {
    if (x[i] === y[j]) {
      out.push({ op: "same", text: x[i] });
      i++;
      j++;
    } else if (t[(i + 1) * w + j] >= t[i * w + j + 1]) {
      out.push({ op: "del", text: x[i++] });
    } else {
      out.push({ op: "add", text: y[j++] });
    }
  }
  while (i < n) out.push({ op: "del", text: x[i++] });
  while (j < m) out.push({ op: "add", text: y[j++] });
  return out;
}
