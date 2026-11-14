import { Extension } from "@tiptap/core";
import { Plugin, TextSelection, type EditorState } from "@tiptap/pm/state";
import type { EditorView } from "@tiptap/pm/view";

type PageLink = { id: string; title: string; path: string };
type Match = { from: number; to: number; query: string };

/** Only an unfinished page name at the caret, outside code and link aliases. */
function matchLink(state: EditorState): Match | undefined {
  const { empty, $from, from } = state.selection;
  if (!empty || !$from.parent.isTextblock || $from.parent.type.spec.code) return;
  if ((state.storedMarks ?? $from.marks()).some((m) => m.type.name === "code")) return;
  const before = $from.parent.textBetween(0, $from.parentOffset, "\n", "\ufffc");
  const at = before.lastIndexOf("[[");
  if (at < 0 || before[at - 1] === "[" || /(?:^|[^\\])(?:\\\\)*\\$/.test(before.slice(0, at))) return;
  const query = before.slice(at + 2);
  if (/[\[\]\n|#\ufffc]/.test(query) || query.length > 200) return;
  const start = from - query.length - 2;
  const code = state.schema.marks.code;
  if (code && state.doc.rangeHasMark(start, from, code)) return;
  const after = $from.parent.textBetween($from.parentOffset, $from.parent.content.size, "\n", "\ufffc");
  const closing = /^[^[\]\n]*\]\]/.exec(after)?.[0].length ?? 0;
  return { from: start, to: from + closing, query };
}

/** An editor-owned popup keeps focus and selection in the document. */
class Completion {
  private panel: HTMLDivElement;
  private match?: Match;
  private dismissed?: number;
  private items: PageLink[] = [];
  private active = 0;
  private loading = false;
  private request?: AbortController;
  private timer?: ReturnType<typeof setTimeout>;
  private destroyed = false;
  private readonly role: string | null;

  constructor(private view: EditorView, private search: (query: string, signal: AbortSignal) => Promise<PageLink[]>) {
    this.role = view.dom.getAttribute("role");
    this.panel = document.createElement("div");
    this.panel.id = `kn-link-picker-${++pickerId}`;
    this.panel.className = "fixed z-[60] overflow-y-auto rounded-xl border border-border-float bg-surface-raised p-1 text-13 text-fg shadow-[var(--shadow-float)]";
    this.panel.setAttribute("role", "listbox");
    this.panel.setAttribute("aria-label", "Link to note");
    this.panel.hidden = true;
    document.body.append(this.panel);
    document.addEventListener("scroll", this.position, true);
    window.addEventListener("resize", this.position);
  }

  update = () => {
    if (this.destroyed) return;
    const next = this.view.editable && this.view.hasFocus() ? matchLink(this.view.state) : undefined;
    if (!next) {
      this.dismissed = undefined;
      this.close();
      return;
    }
    if (this.dismissed === next.from) return;
    if (this.match?.from === next.from && this.match.query === next.query) {
      this.match = next;
      this.position();
      return;
    }
    this.close();
    this.match = next;
    this.active = 0;
    this.items = [];
    this.loading = true;
    this.render("Finding notes…");
    const request = new AbortController();
    this.request = request;
    this.timer = setTimeout(async () => {
      try {
        const items = await this.search(next.query, request.signal);
        if (this.destroyed || request.signal.aborted) return;
        this.items = items;
        this.loading = false;
        this.render(items.length ? undefined : "No matching notes");
      } catch {
        if (!this.destroyed && !request.signal.aborted) {
          this.loading = false;
          this.render("Couldn’t find notes. Keep typing to try again.");
        }
      }
    }, 100);
  };

  private position = () => {
    if (!this.match || this.panel.hidden || this.destroyed) return;
    const caret = this.view.coordsAtPos(this.view.state.selection.from);
    const width = Math.min(360, window.innerWidth - 16);
    Object.assign(this.panel.style, {
      width: `${width}px`,
      left: `${Math.max(8, Math.min(caret.left, window.innerWidth - width - 8))}px`,
      bottom: `${window.innerHeight - caret.top + 8}px`,
      maxHeight: `${Math.max(32, Math.min(288, caret.top - 16))}px`,
    });
  };

  private render(message?: string) {
    this.panel.replaceChildren();
    this.panel.hidden = false;
    this.view.dom.setAttribute("role", "combobox");
    this.view.dom.setAttribute("aria-expanded", "true");
    this.view.dom.setAttribute("aria-autocomplete", "list");
    this.view.dom.setAttribute("aria-controls", this.panel.id);
    this.view.dom.removeAttribute("aria-activedescendant");
    if (message) {
      const status = document.createElement("div");
      status.className = "px-3 py-2 text-fg-tertiary";
      status.setAttribute("role", "status");
      status.textContent = message;
      this.panel.append(status);
    }
    this.items.forEach((item, i) => {
      const option = document.createElement("div");
      option.id = `${this.panel.id}-${i}`;
      option.setAttribute("role", "option");
      option.className = "cursor-default rounded-md px-3 py-2";
      const title = document.createElement("div");
      title.className = "truncate font-medium";
      title.textContent = item.title;
      const path = document.createElement("div");
      path.className = "truncate text-11 text-fg-tertiary";
      path.textContent = item.path;
      option.append(title, path);
      option.addEventListener("mouseenter", () => { this.active = i; this.highlight(); });
      option.addEventListener("mousedown", (event) => { event.preventDefault(); this.pick(item); });
      this.panel.append(option);
    });
    this.highlight();
    this.position();
  }

  private highlight() {
    this.panel.querySelectorAll('[role="option"]').forEach((option, i) => {
      option.setAttribute("aria-selected", String(i === this.active));
      option.classList.toggle("bg-hover", i === this.active);
    });
    if (this.items.length) this.view.dom.setAttribute("aria-activedescendant", `${this.panel.id}-${this.active}`);
  }

  private pick(item: PageLink) {
    const match = matchLink(this.view.state);
    if (!this.match || !match || match.from !== this.match.from || match.query !== this.match.query) return;
    const text = `[[${item.path}]]`;
    this.close();
    const tr = this.view.state.tr.insertText(text, match.from, match.to);
    tr.setSelection(TextSelection.create(tr.doc, match.from + text.length));
    this.view.dispatch(tr);
    this.view.focus();
  }

  key = (event: KeyboardEvent) => {
    if (!this.match || event.isComposing) return false;
    if (event.key === "Escape") {
      this.dismissed = this.match.from;
      this.close();
      event.stopPropagation();
      return true;
    }
    if (event.key === "ArrowDown" || event.key === "ArrowUp") {
      if (this.items.length) {
        this.active = (this.active + (event.key === "ArrowDown" ? 1 : this.items.length - 1)) % this.items.length;
        this.highlight();
        this.panel.children[this.active]?.scrollIntoView({ block: "nearest" });
      }
      return true;
    }
    if (event.key === "Enter" || event.key === "Tab") {
      if (this.items.length) this.pick(this.items[this.active]);
      else if (!this.loading) { this.close(); return false; }
      return true;
    }
    return false;
  };

  close = () => {
    clearTimeout(this.timer);
    this.request?.abort();
    this.match = undefined;
    this.items = [];
    this.loading = false;
    this.panel.hidden = true;
    if (this.role) this.view.dom.setAttribute("role", this.role);
    else this.view.dom.removeAttribute("role");
    for (const name of ["aria-expanded", "aria-controls", "aria-autocomplete", "aria-activedescendant"]) this.view.dom.removeAttribute(name);
  };

  destroy = () => {
    this.destroyed = true;
    this.close();
    this.panel.remove();
    document.removeEventListener("scroll", this.position, true);
    window.removeEventListener("resize", this.position);
  };
}

let pickerId = 0;

export const KnowledgeCompletion = Extension.create<{ search: (query: string, signal: AbortSignal) => Promise<PageLink[]> }>({
  name: "knowledgeCompletion",
  addOptions() { return { search: async () => [] }; },
  addProseMirrorPlugins() {
    let popup: Completion;
    const search = this.options.search;
    return [new Plugin({
      view(view) {
        popup = new Completion(view, search);
        return { update: popup.update, destroy: popup.destroy };
      },
      props: {
        handleKeyDown: (_view, event) => popup.key(event),
        handleDOMEvents: {
          focus: () => { queueMicrotask(() => popup.update()); return false; },
          blur: () => { popup.close(); return false; },
        },
      },
    })];
  },
});
