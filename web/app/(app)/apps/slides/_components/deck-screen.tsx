"use client";

import { useRouter, useSearchParams } from "next/navigation";
import { useCallback, useEffect, useMemo, useRef, useState, type KeyboardEvent } from "react";
import { ApiError, api, unwrap } from "@/lib/api/client";
import { useCanvasActions, useChatCanvas } from "@/lib/api/canvas-hooks";
import { useMarkDocReviewed } from "@/lib/api/docs-hooks";
import { useChat, useSendChatMessage } from "@/lib/api/hooks";
import { uploadDeckAsset, useDeckRef } from "@/lib/api/slides-hooks";
import { cx } from "@/lib/cx";
import { reviewBase } from "@/lib/doc-review";
import { measure } from "@/lib/slides/fonts";
import { layoutSlide } from "@/lib/slides/layout";
import { assetHref, useSlideFonts } from "@/lib/slides/render";
import { mergeDecks } from "@/lib/slides/review";
import { THEMES, encodeDeck, newId, newSlide, parseDeck, relayout, textOf, type Deck, type Element, type Frame, type Paragraph, type Slide } from "@/lib/slides/schema";
import { Button } from "@/components/ui/button";
import { Banner, Skeleton } from "@/components/ui/primitives";
import { TopBar } from "@/components/shell/topbar";
import { Conversation } from "@/app/(app)/chat/_components/conversation";
import { DeckReview } from "./deck-review";
import { ExportMenu } from "./export-menu";
import { SlideCanvas } from "./slide-canvas";
import type { TextEditorHandle } from "./text-editor";
import { Thumbnails } from "./thumbnails";
import { Toolbar, type TextFormat, type ToolbarActions } from "./toolbar";

type SaveState = "saved" | "unsaved" | "saving" | "conflict" | "error";
const SAVE_LABEL: Record<SaveState, string> = { saved: "Saved", unsaved: "Unsaved changes", saving: "Saving…", conflict: "Not saved", error: "Not saved" };
const AUTOSAVE_MS = 800;
const MAX_IMAGE_EDGE = 4096;
const errorText = (e: unknown) => (e instanceof Error ? e.message : "Something went wrong. Please try again.");

/** A PNG or JPEG of at most 4096 px a side, converting or downscaling in the browser first. */
async function prepareImage(file: File): Promise<Blob> {
  const bmp = await createImageBitmap(file).catch(() => {
    throw new Error("this file isn't an image the browser can read");
  });
  const edge = Math.max(bmp.width, bmp.height);
  if ((file.type === "image/png" || file.type === "image/jpeg") && edge <= MAX_IMAGE_EDGE && file.size <= 10 << 20) return file;
  const k = Math.min(1, MAX_IMAGE_EDGE / edge);
  const canvas = document.createElement("canvas");
  canvas.width = Math.round(bmp.width * k);
  canvas.height = Math.round(bmp.height * k);
  canvas.getContext("2d")!.drawImage(bmp, 0, 0, canvas.width, canvas.height);
  const type = file.type === "image/jpeg" ? "image/jpeg" : "image/png";
  return new Promise((res, rej) => canvas.toBlob((b) => (b ? res(b) : rej(new Error("couldn't convert the image"))), type, 0.9));
}

const mapRuns = (paragraphs: Paragraph[], fn: (r: Paragraph["runs"][number]) => Paragraph["runs"][number]) => paragraphs.map((p) => ({ ...p, runs: p.runs.map(fn) }));

/** Applies a text format to every run (or paragraph) of an element's text. */
function formatElement(e: Element, f: TextFormat): Element {
  const box = textOf(e);
  if (!box) return e;
  const first = box.paragraphs[0]?.runs[0];
  let paragraphs = box.paragraphs;
  if (f.kind === "mark") paragraphs = mapRuns(paragraphs, (r) => ({ ...r, [f.mark]: !first?.[f.mark] || undefined }));
  if (f.kind === "size") paragraphs = mapRuns(paragraphs, (r) => ({ ...r, size: f.size }));
  if (f.kind === "font") paragraphs = mapRuns(paragraphs, (r) => ({ ...r, font: f.font }));
  if (f.kind === "color") paragraphs = mapRuns(paragraphs, (r) => ({ ...r, color: f.color }));
  if (f.kind === "align") paragraphs = paragraphs.map((p) => ({ ...p, align: f.align === "left" ? undefined : f.align }));
  if (f.kind === "bullet") paragraphs = paragraphs.map((p) => (f.bullet ? { ...p, bullet: f.bullet, level: p.level ?? 0 } : { ...p, bullet: undefined, level: undefined }));
  return withText(e, paragraphs);
}

function withText(e: Element, paragraphs: Paragraph[]): Element {
  if (e.type === "text") return { ...e, paragraphs };
  if (e.type === "shape") return { ...e, text: { ...(e.text ?? {}), paragraphs } };
  return e;
}

/**
 * A deck: the editor, its persona's chat beside it, and review of the
 * persona's changes (mockups 2/6, 3/6, 5/6, 6/6). Edits save themselves as
 * new versions; concurrent changes to different slides merge, the same slide
 * asks the person.
 */
export function DeckScreen({ id }: { id: string }) {
  const ref = useDeckRef(id);
  const router = useRouter();
  const search = useSearchParams();
  const chatId = ref.data?.chat_id ?? "";
  const chat = useChat(chatId || undefined).data;
  const live = chat?.state === "running";
  const archived = !!chat?.archived;
  const persona = (chat?.speaker ?? chat?.persona)?.name ?? "Assistant";
  const head = useChatCanvas(chatId, chatId ? id : undefined, undefined, live);
  const c = head.data;
  const fonts = useSlideFonts();
  const actions = useCanvasActions(chatId, id);
  const reviewed = useMarkDocReviewed(id);
  const send = useSendChatMessage(chatId);
  const assetUrl = useCallback((a: string) => assetHref(id, a), [id]);
  const [chatOpen, setChatOpen] = useState(true);

  // ---- The working copy, what was last saved, and history.
  const [deck, setDeckState] = useState<Deck | null>(null);
  const deckRef = useRef<Deck | null>(null);
  const saved = useRef<{ seq: number; deck: Deck } | null>(null);
  const dirty = useRef(false);
  const [isDirty, setIsDirty] = useState(false);
  const history = useRef<{ past: Deck[]; future: Deck[]; key?: string; at: number }>({ past: [], future: [], at: 0 });
  const [hist, setHist] = useState({ undo: 0, redo: 0 });
  const [savedSeq, setSavedSeq] = useState(0);
  const [saveState, setSaveState] = useState<{ s: SaveState; message?: string }>({ s: "saved" });
  const [conflict, setConflict] = useState<{ theirs: Deck; seq: number } | null>(null);
  const timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  // Set while a conflict waits on the person: nothing saves until they choose.
  const conflicted = useRef(false);
  const saving = useRef<Promise<void> | null>(null);

  const setDeck = (d: Deck) => {
    deckRef.current = d;
    setDeckState(d);
  };
  const setDirty = (v: boolean) => {
    dirty.current = v;
    setIsDirty(v);
  };
  const markSaved = (seq: number, d: Deck) => {
    saved.current = { seq, deck: d };
    setSavedSeq(seq);
  };
  const syncHist = () => setHist({ undo: history.current.past.length, redo: history.current.future.length });

  // Follow the server while there's nothing unsaved here (the persona's live edits included).
  useEffect(() => {
    if (!c || c.kind !== "deck") return;
    if (saved.current && c.seq <= saved.current.seq) return;
    if (dirty.current) return; // the next save meets the newer version and merges
    try {
      const d = parseDeck(c.body);
      saved.current = { seq: c.seq, deck: d };
      deckRef.current = d;
      // Syncing the editable draft from the server query, an external store.
      // eslint-disable-next-line react-hooks/set-state-in-effect
      setSavedSeq(c.seq);
      setDeckState(d);
    } catch {
      setSaveState({ s: "error", message: "This deck's content couldn't be read." });
    }
  }, [c]);

  const runSave = useCallback(async (): Promise<void> => {
    // A merge after a conflict saves again on top of the newer version.
    for (let attempt = 0; attempt < 3; attempt++) {
      const mine = deckRef.current;
      const at = saved.current;
      if (!mine || !at || !dirty.current || conflicted.current) return;
      if (encodeDeck(mine) === encodeDeck(at.deck)) {
        setDirty(false);
        setSaveState({ s: "saved" });
        return;
      }
      setSaveState({ s: "saving" });
      try {
        const v = await actions.save.mutateAsync({ base_seq: at.seq, body: encodeDeck(mine), note: "Edited" });
        markSaved(v.seq, mine);
        if (deckRef.current === mine) {
          setDirty(false);
          setSaveState({ s: "saved" });
        }
        return;
      } catch (e) {
        if (e instanceof ApiError && e.status === 409) {
          // Someone (the persona) saved in between: merge per slide, or ask.
          const latest = await unwrap(api.GET("/chats/{id}/canvases/{canvasId}", { params: { path: { id: chatId, canvasId: id } } }));
          const theirs = parseDeck(latest.body);
          const merged = mergeDecks(at.deck, deckRef.current ?? mine, theirs);
          markSaved(latest.seq, theirs);
          if (merged) {
            setDeck(merged);
            setSaveState({ s: "unsaved" });
            continue;
          }
          conflicted.current = true;
          clearTimeout(timer.current);
          setConflict({ theirs, seq: latest.seq });
          setSaveState({ s: "conflict", message: "This deck changed while you were editing." });
        } else if (e instanceof ApiError && e.status === 422 && /changes nothing/.test(e.message)) {
          setDirty(false);
          setSaveState({ s: "saved" });
        } else setSaveState({ s: "error", message: errorText(e) });
        return;
      }
    }
  }, [actions.save, chatId, id]);

  /** Saves now (after any save in flight) and returns the deck as saved. */
  const flush = useCallback(async () => {
    clearTimeout(timer.current);
    while (saving.current) await saving.current;
    if (dirty.current && !conflicted.current) {
      saving.current = runSave().finally(() => (saving.current = null));
      await saving.current;
    }
    if (dirty.current) throw new Error(conflicted.current ? "resolve the conflict first" : "the deck isn't saved yet");
    return { deck: saved.current!.deck, seq: saved.current!.seq };
  }, [runSave]);

  const schedule = useCallback(() => {
    clearTimeout(timer.current);
    const tick = () => {
      if (saving.current) {
        timer.current = setTimeout(tick, AUTOSAVE_MS);
        return;
      }
      saving.current = runSave().finally(() => (saving.current = null));
    };
    timer.current = setTimeout(tick, AUTOSAVE_MS);
  }, [runSave]);

  // Save on the way out: hidden tab, closed page or navigation.
  useEffect(() => {
    const hide = () => {
      if (document.visibilityState === "hidden" && dirty.current) void flush().catch(() => undefined);
    };
    const leave = (e: BeforeUnloadEvent) => {
      if (dirty.current) e.preventDefault();
    };
    document.addEventListener("visibilitychange", hide);
    window.addEventListener("beforeunload", leave);
    return () => {
      document.removeEventListener("visibilitychange", hide);
      window.removeEventListener("beforeunload", leave);
      if (dirty.current) void flush().catch(() => undefined);
    };
  }, [flush]);

  // ---- Persona review: their versions after the person's last edit or review.
  const base = c ? reviewBase(c.versions, ref.data?.reviewed_seq ?? 0) : 0;
  const reviewing = !!c && !live && !isDirty && base > 0 && c.seq > base;
  const before = useChatCanvas(chatId, reviewing ? id : undefined, base).data;
  const baseDeck = useMemo(() => (before && before.viewing_seq === base ? parseDeck(before.body) : undefined), [before, base]);
  const headDeck = useMemo(() => (reviewing && c ? parseDeck(c.body) : undefined), [reviewing, c]);
  const changedSlides = useMemo(() => {
    if (!baseDeck || !headDeck) return undefined;
    const old = new Map(baseDeck.slides.map((s) => [s.id, JSON.stringify(s)]));
    return new Set(headDeck.slides.filter((s) => old.get(s.id) !== JSON.stringify(s)).map((s) => s.id));
  }, [baseDeck, headDeck]);

  // ---- Selection.
  const [slideId, setSlideId] = useState<string>();
  // Back from presenting (or a link) at ?slide=N opens that slide.
  const startAt = Number(search.get("slide") ?? 0);
  const [selected, setSelected] = useState<string>();
  const [editing, setEditing] = useState<string>();
  const editorRef = useRef<TextEditorHandle | null>(null);
  const slide = deck?.slides.find((s) => s.id === slideId) ?? deck?.slides[Math.max(0, Math.min(deck.slides.length - 1, startAt - 1))];
  const selectedEl = slide?.elements.find((e) => e.id === selected);
  const readOnly = live || archived || reviewing || !!conflict;

  const laid = useMemo(() => (deck && fonts ? deck.slides.map((s) => ({ id: s.id, overflow: layoutSlide(deck, s, measure).overflow })) : []), [deck, fonts]);
  const overflowingSlides = useMemo(() => new Set(laid.filter((l) => l.overflow.size).map((l) => l.id)), [laid]);
  const overflowHere = laid.find((l) => l.id === slide?.id)?.overflow ?? new Set<string>();

  // ---- Changing the deck.
  const update = (fn: (d: Deck) => Deck, key?: string) => {
    const cur = deckRef.current;
    if (!cur || readOnly) return;
    const next = fn(cur);
    if (next === cur) return;
    const h = history.current;
    const now = Date.now();
    if (!(key && h.key === key && now - h.at < 1500)) {
      h.past.push(cur);
      if (h.past.length > 100) h.past.shift();
    }
    h.future = [];
    h.key = key;
    h.at = now;
    syncHist();
    setDeck(next);
    setDirty(true);
    setSaveState({ s: "unsaved" });
    schedule();
  };
  const travel = (back: boolean) => {
    const h = history.current;
    const cur = deckRef.current;
    const to = back ? h.past.pop() : h.future.pop();
    if (!to || !cur || readOnly) return;
    (back ? h.future : h.past).push(cur);
    h.key = undefined;
    setEditing(undefined);
    setDeck(to);
    setDirty(true);
    setSaveState({ s: "unsaved" });
    schedule();
    syncHist();
  };
  const mapSlide = (sid: string, fn: (s: Slide) => Slide) => (d: Deck) => ({ ...d, slides: d.slides.map((s) => (s.id === sid ? fn(s) : s)) });
  const mapElement = (eid: string, fn: (e: Element) => Element) => (d: Deck) => ({ ...d, slides: d.slides.map((s) => ({ ...s, elements: s.elements.map((e) => (e.id === eid ? fn(e) : e)) })) });
  const addElement = (e: Element, edit = false) => {
    if (!slide) return;
    update(mapSlide(slide.id, (s) => ({ ...s, elements: [...s.elements, e] })));
    setSelected(e.id);
    if (edit) setEditing(e.id);
  };
  const removeSelected = () => {
    if (!slide || !selected) return;
    update(mapSlide(slide.id, (s) => ({ ...s, elements: s.elements.filter((e) => e.id !== selected) })));
    setSelected(undefined);
  };

  const fileInput = useRef<HTMLInputElement>(null);
  const [notice, setNotice] = useState<string>();
  const [uploading, setUploading] = useState(false);
  const onImage = async (file: File) => {
    setNotice(undefined);
    setUploading(true);
    try {
      const blob = await prepareImage(file);
      if (blob.size > 10 << 20) throw new Error(`${file.name} is ${(blob.size / (1 << 20)).toFixed(0)} MB: the limit is 10 MB`);
      const a = await uploadDeckAsset(id, blob);
      const k = Math.min(1200 / a.w, 800 / a.h, 1);
      const w = Math.round(a.w * k), h = Math.round(a.h * k);
      const el: Element = { id: newId("e"), type: "image", asset: a.id, frame: { x: Math.round((1920 - w) / 2), y: Math.round((1080 - h) / 2), w, h }, alt: file.name.replace(/\.[a-z0-9]+$/i, "") };
      update((d) => ({ ...d, assets: { ...d.assets, [a.id]: { mime: a.mime, w: a.w, h: a.h } } }));
      addElement(el);
    } catch (e) {
      setNotice(`Couldn't add ${file.name}: ${errorText(e)}`);
    } finally {
      setUploading(false);
    }
  };

  const tools: ToolbarActions = {
    undo: () => travel(true),
    redo: () => travel(false),
    canUndo: hist.undo > 0,
    canRedo: hist.redo > 0,
    insertText: () => addElement({ id: newId("e"), type: "text", frame: { x: 560, y: 440, w: 800, h: 200 }, paragraphs: [{ runs: [{ text: "", size: 44, font: "body", color: "text" }] }] }, true),
    insertShape: (shape) =>
      addElement({ id: newId("e"), type: "shape", shape, frame: { x: 760, y: 390, w: 400, h: 300 }, fill: "accent", radius: shape === "roundRect" ? 32 : undefined, text: { valign: "middle", paragraphs: [{ align: "center", runs: [{ text: "", size: 40, color: "background", font: "body" }] }] } }),
    insertImage: () => fileInput.current?.click(),
    insertLine: () => addElement({ id: newId("e"), type: "line", frame: { x: 560, y: 540, w: 800, h: 0 }, stroke: "text", strokeWidth: 6 }),
    setLayout: (l) => slide && update(mapSlide(slide.id, (s) => relayout(s, l))),
    setTheme: (k) => update((d) => ({ ...d, theme: structuredClone(THEMES[k]) })),
    setBackground: (col) => slide && update(mapSlide(slide.id, (s) => ({ ...s, background: col ? { color: col } : undefined }))),
    format: (f) => {
      if (!selectedEl) return;
      if (editing && editorRef.current) {
        if (f.kind === "mark") return editorRef.current.format(f.mark);
        if (f.kind === "color") return editorRef.current.color(f.color);
        const paragraphs = editorRef.current.read();
        setEditing(undefined);
        update(mapElement(selectedEl.id, (e) => formatElement(withText(e, paragraphs), f)));
        return;
      }
      update(mapElement(selectedEl.id, (e) => formatElement(e, f)));
    },
    setFill: (col) => selected && update(mapElement(selected, (e) => (e.type === "shape" ? { ...e, fill: col } : e))),
    setStroke: (col) => selected && update(mapElement(selected, (e) => (e.type === "shape" ? { ...e, stroke: col, strokeWidth: col ? (e.strokeWidth || 4) : undefined } : e.type === "line" && col ? { ...e, stroke: col } : e))),
    remove: removeSelected,
  };

  const present = async (from?: number) => {
    try {
      await flush();
    } catch {
      // Present what's saved; the banner already says why the rest isn't.
    }
    const i = from ?? Math.max(0, deckRef.current?.slides.findIndex((s) => s.id === slide?.id) ?? 0);
    router.push(`/present/slides/${id}?slide=${i + 1}`);
  };

  // ---- Keyboard: undo/redo anywhere; the rest when the canvas has the selection.
  useEffect(() => {
    const onKey = (e: globalThis.KeyboardEvent) => {
      const t = e.target as HTMLElement;
      if (t.isContentEditable || t instanceof HTMLInputElement || t instanceof HTMLTextAreaElement) return;
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "z") {
        e.preventDefault();
        travel(!e.shiftKey);
      } else if ((e.metaKey || e.ctrlKey) && e.key === "Enter") {
        e.preventDefault();
        void present(e.shiftKey ? 0 : undefined);
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  });
  const onCanvasKey = (e: KeyboardEvent<HTMLDivElement>) => {
    if (!selectedEl || editing || readOnly) {
      if (e.key === "Escape") setSelected(undefined);
      return;
    }
    const step = e.shiftKey ? 40 : 8;
    const nudge = { ArrowLeft: [-step, 0], ArrowRight: [step, 0], ArrowUp: [0, -step], ArrowDown: [0, step] }[e.key];
    if (nudge) {
      e.preventDefault();
      update(mapElement(selectedEl.id, (x) => ({ ...x, frame: { ...x.frame, x: x.frame.x + nudge[0], y: x.frame.y + nudge[1] } })), `nudge:${selectedEl.id}`);
    } else if (e.key === "Delete" || e.key === "Backspace") {
      e.preventDefault();
      removeSelected();
    } else if (e.key === "Enter" && textOf(selectedEl)) {
      e.preventDefault();
      setEditing(selectedEl.id);
    } else if (e.key === "Escape") setSelected(undefined);
    else if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "d") {
      e.preventDefault();
      const copy = { ...structuredClone(selectedEl), id: newId("e"), frame: { ...selectedEl.frame, x: selectedEl.frame.x + 40, y: selectedEl.frame.y + 40 }, placeholder: undefined };
      addElement(copy);
    }
  };

  // ---- Slides.
  const addSlide = () => {
    if (!slide) return;
    const s = newSlide(slide.layout === "title" ? "title-body" : slide.layout);
    update((d) => {
      const i = d.slides.findIndex((x) => x.id === slide.id);
      return { ...d, slides: [...d.slides.slice(0, i + 1), s, ...d.slides.slice(i + 1)] };
    });
    setSlideId(s.id);
    setSelected(undefined);
  };
  const duplicateSlide = (sid: string) => {
    const src = deckRef.current?.slides.find((s) => s.id === sid);
    if (!src) return;
    const copy: Slide = { ...structuredClone(src), id: newId("s"), elements: src.elements.map((e) => ({ ...structuredClone(e), id: newId("e") })) };
    update((d) => {
      const i = d.slides.findIndex((x) => x.id === sid);
      return { ...d, slides: [...d.slides.slice(0, i + 1), copy, ...d.slides.slice(i + 1)] };
    });
    setSlideId(copy.id);
  };
  const removeSlide = (sid: string) => {
    if ((deckRef.current?.slides.length ?? 0) < 2) return;
    const i = deckRef.current!.slides.findIndex((s) => s.id === sid);
    update((d) => ({ ...d, slides: d.slides.filter((s) => s.id !== sid) }));
    const next = deckRef.current!.slides[Math.min(i, deckRef.current!.slides.length - 1)];
    setSlideId(next?.id);
  };
  const moveSlide = (sid: string, to: number) =>
    update((d) => {
      const s = d.slides.find((x) => x.id === sid)!;
      const rest = d.slides.filter((x) => x.id !== sid);
      return { ...d, slides: [...rest.slice(0, to), s, ...rest.slice(to)] };
    });

  // A deck started from a description asks the persona to draft it, once.
  const brief = search.get("brief");
  const briefSent = useRef(false);
  useEffect(() => {
    if (!brief || !chatId || briefSent.current) return;
    briefSent.current = true;
    router.replace(`/apps/slides/${id}`);
    void send.mutateAsync({ body: `Draft this deck: ${brief}` }).catch((e) => setNotice(`Couldn't ask ${persona} to draft the deck: ${errorText(e)}`));
  }, [brief, chatId, id, persona, router, send]);

  const title = ref.data?.title ?? c?.title ?? "Deck";
  const loadError = ref.error ?? head.error;
  const themeKey = deck ? Object.entries(THEMES).find(([, t]) => t.name === deck.theme.name)?.[0] : undefined;

  return (
    <div className="flex h-dvh flex-col">
      <TopBar
        crumbs={[{ label: "Apps" }, { label: "Slides", href: "/apps/slides" }, { label: title }]}
        actions={
          ref.data && (
            <>
              <span className="flex items-center gap-1.5 text-[12px] text-fg-tertiary">
                <span aria-hidden className={cx("grid size-[18px] place-items-center rounded-full bg-accent-soft text-[10px] font-bold text-accent", live && "animate-pulse")}>{persona.slice(0, 1)}</span>
                {live ? `${persona} is editing…` : reviewing ? `${persona} suggested changes` : `${persona} can edit`}
              </span>
              {c && (
                <span data-testid="deck-save-state" title={saveState.message} className={cx("text-[12px]", saveState.s === "error" || saveState.s === "conflict" ? "text-pill-blocked-fg" : "text-fg-tertiary")}>
                  {SAVE_LABEL[saveState.s]} · v{savedSeq || c.seq}
                  {saveState.s === "error" && (
                    <button type="button" className="ml-1.5 text-accent hover:underline" onClick={() => void flush().catch(() => undefined)}>
                      Retry
                    </button>
                  )}
                </span>
              )}
              <Button variant="primary" onClick={() => void present()} disabled={!deck} hint="⌘↵">
                ▶ Present
              </Button>
              {deck && <ExportMenu title={title} current={flush} assetUrl={assetUrl} fontsReady={fonts} onPresent={() => void present()} />}
              <Button variant="secondary" aria-pressed={chatOpen} onClick={() => setChatOpen((o) => !o)}>
                ✦ {chatOpen ? "Hide chat" : `Chat with ${persona}`}
              </Button>
            </>
          )
        }
      />
      {loadError ? (
        <div className="mx-auto w-full max-w-[720px] p-6">
          <Banner tone="blocked" title="Couldn’t open this deck" actions={<Button onClick={() => router.push("/apps/slides")}>Back to Slides</Button>}>
            {loadError instanceof ApiError && loadError.status === 404 ? "It may have been deleted, or it belongs to someone else." : errorText(loadError)}
          </Banner>
        </div>
      ) : !deck || !slide || !c ? (
        <div aria-busy="true" className="flex min-h-0 flex-1 gap-4 p-4">
          <div className="flex w-[140px] flex-col gap-3">{[0, 1, 2, 3].map((i) => <Skeleton key={i} className="aspect-video w-full" />)}</div>
          <div className="flex flex-1 flex-col gap-3">
            <Skeleton className="h-8 w-1/2" />
            <Skeleton className="aspect-video w-full max-w-[900px]" />
          </div>
        </div>
      ) : (
        <div className="relative flex min-h-0 flex-1">
          {chatOpen && (
            <aside aria-label={`Chat with ${persona}`} className="flex min-h-0 w-[360px] flex-none flex-col border-r border-border-subtle bg-sidebar max-lg:absolute max-lg:inset-y-0 max-lg:left-0 max-lg:z-30 max-lg:w-[min(380px,100%)] max-lg:shadow-[var(--shadow-float)]">
              <Conversation key={chatId} id={chatId} onNew={() => router.push("/apps/slides")} />
            </aside>
          )}
          <div className="flex min-h-0 min-w-0 flex-1 max-md:flex-col-reverse">
            <Thumbnails
              deck={reviewing && headDeck ? headDeck : deck}
              current={slide.id}
              onSelect={(sid) => {
                setSlideId(sid);
                setSelected(undefined);
                setEditing(undefined);
                if (reviewing) document.querySelector(`[data-change-slide="${sid}"]`)?.scrollIntoView({ block: "nearest" });
              }}
              onMove={moveSlide}
              onDuplicate={duplicateSlide}
              onRemove={removeSlide}
              onAdd={addSlide}
              assetUrl={assetUrl}
              fontsReady={fonts}
              overflowing={overflowingSlides}
              changed={changedSlides}
              persona={persona}
              readOnly={readOnly}
            />
            <main className="flex min-h-0 min-w-0 flex-1 flex-col bg-surface-sunken">
              <Toolbar theme={deck.theme} themeKey={themeKey} layout={slide.layout} selected={selectedEl} editing={!!editing} disabled={readOnly} a={tools} />
              <input ref={fileInput} type="file" accept="image/*" className="hidden" aria-label="Add an image to the slide" data-testid="slide-image-input" onChange={(e) => {
                const f = e.target.files?.[0];
                e.target.value = "";
                if (f) void onImage(f);
              }} />
              {reviewing ? (
                baseDeck && headDeck ? (
                  <DeckReview
                    key={`${base}-${c.seq}`}
                    base={baseDeck}
                    head={headDeck}
                    persona={persona}
                    assetUrl={assetUrl}
                    fontsReady={fonts}
                    busy={archived || reviewed.isPending || actions.save.isPending}
                    error={reviewed.error || actions.save.error ? errorText(reviewed.error ?? actions.save.error) : undefined}
                    onAcceptAll={() => reviewed.mutate(c.seq)}
                    onResolve={(d, note) =>
                      actions.save.mutate(
                        { base_seq: c.seq, body: encodeDeck(d), note },
                        { onSuccess: (v) => reviewed.mutate(v.seq) },
                      )
                    }
                  />
                ) : (
                  <div aria-busy="true" className="p-6"><Skeleton className="h-[320px]" /></div>
                )
              ) : (
                <div className="min-h-0 flex-1 overflow-y-auto px-8 py-6 max-sm:px-3" onKeyDown={onCanvasKey}>
                  <div className="mx-auto flex max-w-[1100px] flex-col gap-3">
                    {conflict && (
                      <Banner
                        tone="blocked"
                        title="This deck changed while you were editing"
                        actions={
                          <>
                            <Button
                              variant="secondary"
                              onClick={() => {
                                conflicted.current = false;
                                markSaved(conflict.seq, conflict.theirs);
                                setDeck(conflict.theirs);
                                setDirty(false);
                                setConflict(null);
                                setSaveState({ s: "saved" });
                              }}
                            >
                              Use the latest version
                            </Button>
                            <Button
                              variant="primary"
                              onClick={() => {
                                conflicted.current = false;
                                setConflict(null);
                                void flush().catch(() => undefined);
                              }}
                            >
                              Keep mine
                            </Button>
                          </>
                        }
                      >
                        {persona} changed some of the same slides you did. Keep mine saves your version over theirs (theirs stays in the version history).
                      </Banner>
                    )}
                    {live && <Banner tone="human" title={`${persona} is editing…`}>The canvas is read-only until {persona} finishes; you’ll then review the changes.</Banner>}
                    {archived && <Banner tone="human" title="This deck’s chat is archived">Restore the chat to edit the deck.</Banner>}
                    {saveState.s === "error" && <Banner tone="blocked" title="Couldn’t save your changes" actions={<Button onClick={() => void flush().catch(() => undefined)}>Retry now</Button>}>{saveState.message} Your edits are kept in this tab; don’t close it yet.</Banner>}
                    {(notice || uploading) && <p role={notice ? "alert" : "status"} className={cx("text-[12px]", notice ? "text-pill-blocked-fg" : "text-fg-secondary")}>{notice ?? "Uploading image…"}</p>}
                    <SlideCanvas
                      deck={deck}
                      slide={slide}
                      assetUrl={assetUrl}
                      fontsReady={fonts}
                      selected={selected}
                      onSelect={setSelected}
                      editing={editing}
                      onEdit={setEditing}
                      onFrame={(eid, frame: Frame, rotation) => update(mapElement(eid, (e) => ({ ...e, frame, rotation })))}
                      onText={(eid, paragraphs) => update(mapElement(eid, (e) => withText(e, paragraphs)))}
                      readOnly={readOnly}
                      editorRef={editorRef}
                      overflow={overflowHere}
                    />
                    <label className="flex flex-col gap-1.5 rounded-xl border border-border bg-surface p-3">
                      <span className="flex items-center justify-between text-[12px] font-medium text-fg-secondary">
                        Speaker notes
                        <span className="font-mono text-[11.5px] font-normal text-fg-tertiary">
                          Slide {deck.slides.findIndex((s) => s.id === slide.id) + 1} of {deck.slides.length}
                        </span>
                      </span>
                      <textarea
                        className="min-h-[72px] resize-y bg-transparent text-[13.5px] text-fg-2 outline-none placeholder:text-fg-tertiary"
                        placeholder="What to say on this slide"
                        value={slide.notes ?? ""}
                        readOnly={readOnly}
                        maxLength={10000}
                        onChange={(e) => {
                          const notes = e.target.value;
                          update(mapSlide(slide.id, (s) => ({ ...s, notes: notes || undefined })), `notes:${slide.id}`);
                        }}
                      />
                    </label>
                    <label className="flex items-center gap-2 text-[12.5px] text-fg-secondary">
                      <input type="checkbox" checked={!!slide.hidden} disabled={readOnly} onChange={(e) => update(mapSlide(slide.id, (s) => ({ ...s, hidden: e.target.checked || undefined })))} />
                      Skip this slide when presenting and exporting
                    </label>
                  </div>
                </div>
              )}
            </main>
          </div>
        </div>
      )}
    </div>
  );
}
