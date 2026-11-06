// Search snippets wrap matched terms in « » (see SearchHit/WikiPattern).

export type SnippetPart = { text: string; match: boolean };

/** Splits a snippet into plain and matched parts; the markers are dropped. */
export function snippetParts(snippet: string): SnippetPart[] {
  const parts: SnippetPart[] = [];
  for (const piece of snippet.split(/(«[^»]*»)/)) {
    if (!piece) continue;
    if (piece.startsWith("«") && piece.endsWith("»")) parts.push({ text: piece.slice(1, -1), match: true });
    else parts.push({ text: piece.replace(/[«»]/g, ""), match: false });
  }
  return parts;
}
