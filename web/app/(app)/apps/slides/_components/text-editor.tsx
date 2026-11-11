"use client";

import { forwardRef, useEffect, useImperativeHandle, useLayoutEffect, useRef } from "react";
import { FAMILY } from "@/lib/slides/fonts";
import { LIMITS } from "@/lib/slides/edit";
import { BULLET_INDENT, INSET, lineHeight } from "@/lib/slides/layout";
import { resolveColor, slotOf, THEME_COLORS, type ColorRef, type FontRef, type Paragraph, type Run, type TextBox, type Theme } from "@/lib/slides/schema";

export type TextEditorHandle = {
  /** Applies a mark to the selection (bold, italic, underline, strike), a colour or a highlight. */
  format: (cmd: "bold" | "italic" | "underline" | "strike") => void;
  color: (c: ColorRef) => void;
  highlight: (c: ColorRef | undefined) => void;
  /** Moves the bulleted paragraphs in the selection in or out a level; false if there are none. */
  indent: (delta: 1 | -1) => boolean;
  /** The text as edited so far. */
  read: () => TextBox["paragraphs"];
};

const esc = (s: string) => s.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;").replace(/"/g, "&quot;");

/** The editable HTML for a text box: one div per paragraph, one span per run. */
function toHtml(theme: Theme, paragraphs: Paragraph[]): string {
  return paragraphs
    .map((p) => {
      const level = p.level ?? 0;
      const style = [`text-align:${p.align ?? "left"}`, `line-height:${lineHeight(p)}px`, `margin-top:${p.spaceBefore ?? 0}px`, p.bullet ? `padding-left:${BULLET_INDENT * (level + 1)}px` : "", `--bullet-x:${BULLET_INDENT * level}px`, `--bullet-size:${p.runs[0].size}px`].filter(Boolean).join(";");
      const runs = p.runs
        .map((r) => {
          const slot = slotOf(theme, r.font);
          const css = [`font-family:'${FAMILY[slot]}'`, `font-size:${r.size}px`, `font-weight:${r.bold ? 700 : 400}`, `font-style:${r.italic ? "italic" : "normal"}`, r.underline || r.strike ? `text-decoration:${[r.underline && "underline", r.strike && "line-through"].filter(Boolean).join(" ")}` : "", `color:${resolveColor(theme, r.color)}`, r.highlight ? `background-color:${resolveColor(theme, r.highlight)}` : ""].filter(Boolean).join(";");
          return `<span data-font="${r.font ?? "body"}" data-color="${esc(r.color ?? "text")}"${r.highlight ? ` data-highlight="${esc(r.highlight)}"` : ""} style="${css}">${r.text ? esc(r.text) : "<br>"}</span>`;
        })
        .join("");
      return `<div data-bullet="${p.bullet ?? ""}" data-level="${level}" data-space="${p.spaceBefore ?? 0}" data-ls="${p.lineSpacing ?? 1}" style="${style}">${runs}</div>`;
    })
    .join("");
}

const hex = (rgb: string) => {
  const m = rgb.match(/\d+(\.\d+)?/g);
  if (!m) return "#000000";
  return `#${m.slice(0, 3).map((n) => Math.round(Number(n)).toString(16).padStart(2, "0")).join("")}`.toUpperCase();
};

/** Reads the edited DOM back into paragraphs of runs; adjacent runs that match merge. */
function fromDom(root: HTMLElement, theme: Theme, fallback: Run): Paragraph[] {
  const blocks: HTMLElement[] = [];
  // Stray inline content at the top level (from some pastes) becomes its own paragraph.
  for (const n of [...root.childNodes]) {
    if (n instanceof HTMLElement && (n.tagName === "DIV" || n.tagName === "P")) blocks.push(n);
    else if (n.textContent) {
      const d = document.createElement("div");
      root.insertBefore(d, n);
      d.append(n);
      blocks.push(d);
    }
  }
  const out: Paragraph[] = [];
  for (const b of blocks) {
    const runs: Run[] = [];
    const walker = document.createTreeWalker(b, NodeFilter.SHOW_TEXT);
    let first: Run | undefined;
    for (let t = walker.nextNode(); t; t = walker.nextNode()) {
      const el = t.parentElement!;
      const cs = getComputedStyle(el);
      let underline = false;
      let strike = false;
      let font: FontRef | undefined;
      let token: string | undefined;
      let hlToken: string | undefined;
      let bg: string | undefined;
      for (let a: HTMLElement | null = el; a && a !== b; a = a.parentElement) {
        const as = getComputedStyle(a);
        if (as.textDecorationLine.includes("underline") || a.tagName === "U") underline = true;
        if (as.textDecorationLine.includes("line-through") || a.tagName === "S" || a.tagName === "STRIKE") strike = true;
        font ??= (a.dataset.font as FontRef | undefined) ?? undefined;
        token ??= a.dataset.color;
        hlToken ??= a.dataset.highlight;
        // The innermost painted background is the one that shows.
        if (bg === undefined && !/^(transparent|rgba\(.*,\s*0\))$/.test(as.backgroundColor)) bg = hex(as.backgroundColor);
      }
      const color = hex(cs.color);
      const tokenColor = token && (THEME_COLORS as string[]).includes(token) ? resolveColor(theme, token) : token;
      const hlColor = hlToken && (THEME_COLORS as string[]).includes(hlToken) ? resolveColor(theme, hlToken) : hlToken;
      const highlight = bg && (hlToken && hlColor?.toUpperCase() === bg ? hlToken : bg);
      const run: Run = {
        text: (t.textContent ?? "").replace(/[\n\r ]/g, (c) => (c === " " ? " " : "")),
        size: Math.round(parseFloat(cs.fontSize)) || fallback.size,
        ...(parseInt(cs.fontWeight, 10) >= 600 ? { bold: true } : {}),
        ...(cs.fontStyle === "italic" ? { italic: true } : {}),
        ...(underline ? { underline: true } : {}),
        ...(strike ? { strike: true } : {}),
        color: token && tokenColor?.toUpperCase() === color ? token : color,
        ...(highlight ? { highlight } : {}),
        font: font ?? fallback.font,
      };
      first ??= run;
      const prev = runs.at(-1);
      if (prev && prev.size === run.size && !!prev.bold === !!run.bold && !!prev.italic === !!run.italic && !!prev.underline === !!run.underline && !!prev.strike === !!run.strike && prev.highlight === run.highlight && prev.color === run.color && prev.font === run.font) prev.text += run.text;
      else runs.push(run);
    }
    if (!runs.length || runs.every((r) => !r.text)) runs.splice(0, runs.length, { ...(first ?? fallback), text: "" });
    const level = Number(b.dataset.level ?? 0);
    const bullet = b.dataset.bullet as Paragraph["bullet"] | "";
    const align = b.style.textAlign as Paragraph["align"];
    const space = Number(b.dataset.space ?? 0);
    const ls = Number(b.dataset.ls ?? 1);
    out.push({ runs, ...(align && align !== "left" ? { align } : {}), ...(bullet ? { bullet, level } : {}), ...(space ? { spaceBefore: space } : {}), ...(ls && ls !== 1 ? { lineSpacing: ls } : {}) });
  }
  return out.length ? out : [{ runs: [{ ...fallback, text: "" }] }];
}

/** The paragraph divs the selection touches. */
function selectedBlocks(root: HTMLElement): HTMLElement[] {
  const sel = getSelection();
  if (!sel?.rangeCount) return [];
  const blocks = [...root.children].filter((n): n is HTMLElement => n instanceof HTMLElement);
  const hit = blocks.filter((b) => sel.containsNode(b, true));
  if (hit.length) return hit;
  // A caret in an empty paragraph may sit on the root itself.
  const at = sel.anchorNode instanceof HTMLElement ? sel.anchorNode : sel.anchorNode?.parentElement;
  const b = at?.closest<HTMLElement>("[data-bullet]");
  return b && root.contains(b) ? [b] : [];
}

/** Changes the level of the bulleted paragraphs among blocks, as the layout would draw them. */
function shiftBlocks(blocks: HTMLElement[], delta: number): boolean {
  const bulleted = blocks.filter((b) => b.dataset.bullet);
  for (const b of bulleted) {
    const level = Math.max(LIMITS.level.min, Math.min(LIMITS.level.max, Number(b.dataset.level ?? 0) + delta));
    b.dataset.level = String(level);
    b.style.paddingLeft = `${BULLET_INDENT * (level + 1)}px`;
    b.style.setProperty("--bullet-x", `${BULLET_INDENT * level}px`);
  }
  return bulleted.length > 0;
}

/**
 * Edits a text box in place, drawn at slide units and scaled to the canvas,
 * with the same fonts, insets and line heights as the layout so lines break
 * where the slide will break them. Commits on blur or Escape.
 */
export const TextEditor = forwardRef<TextEditorHandle, { theme: Theme; box: TextBox; frame: { x: number; y: number; w: number; h: number }; rotation?: number; pivot?: { x: number; y: number }; scale: number; onCommit: (paragraphs: Paragraph[]) => void }>(
  /** pivot: the slide point the element rotates about, when the text box isn't the whole element (a preset shape's text rectangle). */
  function TextEditor({ theme, box, frame, rotation, pivot, scale, onCommit }, ref) {
    const el = useRef<HTMLDivElement>(null);
    const fallback = useRef<Run>(box.paragraphs[0]?.runs[0] ?? { text: "", size: 44 });
    const committed = useRef(false);
    const commit = () => {
      if (committed.current || !el.current) return;
      committed.current = true;
      onCommit(fromDom(el.current, theme, fallback.current));
    };
    useImperativeHandle(ref, () => ({
      format: (cmd) => document.execCommand(cmd === "strike" ? "strikeThrough" : cmd),
      color: (c) => {
        document.execCommand("styleWithCSS", false, "true");
        document.execCommand("foreColor", false, resolveColor(theme, c));
        // Tag what was just coloured so a theme colour stays a theme colour.
        const sel = getSelection();
        if (sel?.rangeCount) for (const s of el.current?.querySelectorAll<HTMLElement>("span[style*='color']") ?? []) if (sel.containsNode(s, true)) s.dataset.color = c;
      },
      highlight: (c) => {
        document.execCommand("styleWithCSS", false, "true");
        document.execCommand("hiliteColor", false, c ? resolveColor(theme, c) : "transparent");
        // Tag what was just highlighted so a theme colour stays a theme colour.
        const sel = getSelection();
        if (c && sel?.rangeCount) for (const s of el.current?.querySelectorAll<HTMLElement>("span[style*='background']") ?? []) if (sel.containsNode(s, true)) s.dataset.highlight = c;
      },
      indent: (delta) => !!el.current && shiftBlocks(selectedBlocks(el.current), delta),
      read: () => (el.current ? fromDom(el.current, theme, fallback.current) : box.paragraphs),
    }));
    useLayoutEffect(() => {
      const d = el.current!;
      d.innerHTML = toHtml(theme, box.paragraphs);
      document.execCommand("defaultParagraphSeparator", false, "div");
      d.focus();
      const r = document.createRange();
      r.selectNodeContents(d);
      getSelection()?.removeAllRanges();
      getSelection()?.addRange(r);
      // Mount only: the editor owns the DOM while it's open.
      // eslint-disable-next-line react-hooks/exhaustive-deps
    }, []);
    useEffect(() => () => commit(), []); // eslint-disable-line react-hooks/exhaustive-deps
    const justify = box.valign === "middle" ? "center" : box.valign === "bottom" ? "flex-end" : "flex-start";
    return (
      <div
        className="slides-text-editor absolute left-0 top-0 origin-top-left"
        style={{ transform: `translate(${frame.x * scale}px, ${frame.y * scale}px) scale(${scale})`, width: frame.w, minHeight: frame.h }}
      >
        <style>{`
          .slides-text-editor [data-bullet]{position:relative}
          .slides-text-editor [contenteditable]{counter-reset:n}
          .slides-text-editor [data-bullet="number"]{counter-increment:n}
          .slides-text-editor [data-bullet=""]{counter-reset:n}
          .slides-text-editor [data-bullet="disc"]::before,.slides-text-editor [data-bullet="number"]::before{position:absolute;left:var(--bullet-x);font-size:var(--bullet-size)}
          .slides-text-editor [data-bullet="disc"]::before{content:"•"}
          .slides-text-editor [data-bullet="number"]::before{content:counter(n) "."}
        `}</style>
        <div
          ref={el}
          contentEditable
          suppressContentEditableWarning
          role="textbox"
          aria-multiline
          aria-label="Slide text"
          spellCheck
          className="flex flex-col outline outline-2 outline-accent"
          style={{ minHeight: frame.h, padding: INSET, justifyContent: justify, transform: rotation ? `rotate(${rotation}deg)` : undefined, transformOrigin: pivot ? `${pivot.x - frame.x}px ${pivot.y - frame.y}px` : undefined, fontKerning: "none", whiteSpace: "pre-wrap", wordBreak: "break-word", caretColor: resolveColor(theme, "text") }}
          onBlur={commit}
          onPaste={(e) => {
            e.preventDefault();
            document.execCommand("insertText", false, e.clipboardData.getData("text/plain").replace(/\r?\n/g, " "));
          }}
          onKeyDown={(e) => {
            e.stopPropagation();
            if ((e.metaKey || e.ctrlKey) && e.shiftKey && e.key.toLowerCase() === "x") {
              e.preventDefault();
              document.execCommand("strikeThrough");
            }
            if (e.key === "Escape") {
              e.preventDefault();
              el.current?.blur();
            } else if (e.key === "Tab" && !e.altKey && !e.ctrlKey && !e.metaKey && el.current) {
              // In bulleted text Tab indents and Shift+Tab outdents; elsewhere it moves focus as usual.
              if (shiftBlocks(selectedBlocks(el.current), e.shiftKey ? -1 : 1)) e.preventDefault();
            }
          }}
        />
      </div>
    );
  },
);
