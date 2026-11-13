import { Extension } from "@tiptap/core";
import { Plugin, PluginKey } from "@tiptap/pm/state";
import { Decoration, DecorationSet } from "@tiptap/pm/view";
import type { Node as PMNode } from "@tiptap/pm/model";

// Live preview of Obsidian syntax in the Docs editor: [[links]] and ![[embeds]]
// read as links, #tags as pills and ^block ids fade, while the Markdown stays
// exactly as typed. ⌘/Ctrl-click a link to open it.

const PATTERN = /(!?\[\[[^\]\n]+\]\])|(?<![\w/#])(#[\p{L}\p{N}_/-]*\p{L}[\p{L}\p{N}_/-]*)|(\s\^[A-Za-z0-9-]+\s*$)/gu;

function decorate(doc: PMNode, isResolved: (target: string) => boolean): DecorationSet {
  const out: Decoration[] = [];
  doc.descendants((node, pos, parent) => {
    if (!node.isText || !node.text) return;
    if (parent?.type.name === "codeBlock" || node.marks.some((m) => m.type.name === "code")) return;
    for (const m of node.text.matchAll(PATTERN)) {
      const from = pos + (m.index ?? 0);
      const to = from + m[0].length;
      if (m[1]) {
        const target = m[1].replace(/^!?\[\[/, "").replace(/\]\]$/, "").split("|")[0].split("#")[0].trim();
        const cls = m[1].startsWith("!") ? "kn-embed" : isResolved(target) ? "kn-link" : "kn-link kn-unresolved";
        out.push(Decoration.inline(from, to, { class: cls, "data-target": m[1] }));
      } else if (m[2]) out.push(Decoration.inline(from, to, { class: "kn-tag" }));
      else if (m[3]) out.push(Decoration.inline(from, to, { class: "kn-block" }));
    }
  });
  return DecorationSet.create(doc, out);
}

export type KnowledgeSyntaxOptions = {
  /** Whether a link target names a page that exists; unresolved links fade. */
  isResolved: (target: string) => boolean;
  /** ⌘/Ctrl-click on a [[link]] or ![[embed]], with the link as written. */
  onOpen: (raw: string) => void;
};

export const KnowledgeSyntax = Extension.create<KnowledgeSyntaxOptions>({
  name: "knowledgeSyntax",
  addOptions() {
    return { isResolved: () => true, onOpen: () => {} };
  },
  addProseMirrorPlugins() {
    const opts = this.options;
    const key = new PluginKey("knowledgeSyntax");
    return [
      new Plugin({
        key,
        state: {
          init: (_, { doc }) => decorate(doc, opts.isResolved),
          apply: (tr, old) => (tr.docChanged || tr.getMeta(key) ? decorate(tr.doc, opts.isResolved) : old),
        },
        props: {
          decorations(state) {
            return key.getState(state);
          },
          handleClick(_view, _pos, event) {
            const el = (event.target as HTMLElement | null)?.closest?.("[data-target]");
            if (!el || !(event.metaKey || event.ctrlKey)) return false;
            opts.onOpen(el.getAttribute("data-target") ?? "");
            return true;
          },
        },
      }),
    ];
  },
});

/** The page a [[link]] as written points at: its target before # and |. */
export function linkTarget(raw: string): { target: string; anchor?: string } {
  const inner = raw.replace(/^!?\[\[/, "").replace(/\]\]$/, "").split("|")[0];
  const [target, anchor] = inner.split("#");
  return { target: target.trim(), anchor: anchor?.trim() || undefined };
}
