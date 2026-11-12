"use client";

import { useRouter, useSearchParams } from "next/navigation";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { ApiError } from "@/lib/api/client";
import { fetchDesign, fetchDesignVersion, uploadDesignAsset, useDesign, useDesignProposals, useDesignSettings, useDesignVersion, useResolveProposal, useSaveDesign } from "@/lib/api/designs-hooks";
import { provideChatRefs, useChat, useChatRunEvents, useSendChatMessage } from "@/lib/api/hooks";
import { toolCalls } from "@/lib/chat-ui";
import { cx } from "@/lib/cx";
import * as W from "@/lib/designs/doc";
import { dropTarget } from "@/lib/designs/hit";
import { duplicateNodes, find, groupNodes, inAutoLayout, insertNodes, labelOf, mapNodes, newId, removeNodes, reorder, topLevelIds, ungroupNodes, type ZOrder } from "@/lib/designs/edit";
import { makeLayout, settle, type LaidNode, type LaidPage } from "@/lib/designs/layout";
import { mergeDesigns } from "@/lib/designs/merge";
import { presenceOf } from "@/lib/designs/presence";
import { adoptVersion, type Change } from "@/lib/designs/review";
import { designAssetHref, useDesignFonts } from "@/lib/designs/render";
import { DesignError, normalize, type Design, type DesignNode, type ImageNode } from "@/lib/designs/schema";
import { Button } from "@/components/ui/button";
import { Banner, Skeleton } from "@/components/ui/primitives";
import { TopBar } from "@/components/shell/topbar";
import { Conversation } from "@/app/(app)/chat/_components/conversation";
import { DesignCanvas, fitCamera, zoomAt, type Camera, type Tool } from "./design-canvas";
import { ReviewBar, ReviewOverlay, ReviewPanel, useReview, type ReviewMode } from "./design-review";
import { PresenceLayer } from "./presence-layer";
import { Inspector } from "./inspector";
import { PrototypePanel } from "./prototype-panel";
import { LayersPanel } from "./layers-panel";

type SaveState = "saved" | "unsaved" | "saving" | "conflict" | "error";
const SAVE_LABEL: Record<SaveState, string> = { saved: "Saved", unsaved: "Unsaved changes", saving: "Saving…", conflict: "Not saved", error: "Not saved" };
/** Until the canvas has measured itself and the page is fitted, nothing shows. */
const HIDDEN_CAMERA: Camera = { x: -1e7, y: -1e7, z: 1 };
/** Edits within this quiet time save as one version. */
export const AUTOSAVE_MS = 800;
const MAX_IMAGE_EDGE = 4096;
const errorText = (e: unknown) => (e instanceof Error ? e.message : "Something went wrong. Please try again.");

const TOOLS: { tool: Tool | "image"; label: string; key: string; glyph: string }[] = [
  { tool: "select", label: "Move", key: "V", glyph: "➤" },
  { tool: "hand", label: "Hand", key: "H", glyph: "✋" },
  { tool: "frame", label: "Frame", key: "F", glyph: "#" },
  { tool: "rect", label: "Rectangle", key: "R", glyph: "▭" },
  { tool: "ellipse", label: "Ellipse", key: "O", glyph: "◯" },
  { tool: "line", label: "Line", key: "L", glyph: "╱" },
  { tool: "pen", label: "Pen", key: "P", glyph: "✒" },
  { tool: "text", label: "Text", key: "T", glyph: "T" },
  { tool: "image", label: "Image", key: "⇧⌘K", glyph: "▣" },
];
const TOOL_KEYS: Record<string, Tool> = { v: "select", h: "hand", f: "frame", a: "frame", r: "rect", o: "ellipse", l: "line", p: "pen", t: "text" };

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

/** A top-level node's ops, kept while the node and what it depends on (tokens, assets, components) are the same objects. */
const laidCache = new WeakMap<DesignNode, { deps: unknown[]; laid: LaidNode }>();

/**
 * The page's display list, reusing a top-level node's ops while that node
 * (and what instances and tokens depend on) is the same object, so an edit
 * re-renders only the nodes it touched.
 */
function useLaidPage(design: Design | undefined, pageId: string | undefined): { laid?: LaidPage; lay?: ReturnType<typeof makeLayout> } {
  return useMemo(() => {
    const page = design?.pages.find((p) => p.id === pageId) ?? design?.pages[0];
    if (!design || !page) return {};
    const lay = makeLayout(design);
    const laid = lay.layoutPage(page);
    const components: DesignNode[] = [];
    for (const p of design.pages) {
      const walk = (list: DesignNode[]) => {
        for (const n of list) {
          if (n.type === "component") components.push(n);
          if ("children" in n && Array.isArray(n.children)) walk(n.children);
        }
      };
      walk(p.children);
    }
    const deps = [design.mode, design.token_sets, design.tokens_ref, design.assets, ...components];
    const byId = new Map(page.children.map((n) => [n.id, n]));
    laid.nodes = laid.nodes.map((ln) => {
      const node = byId.get(ln.id)!;
      const prev = laidCache.get(node);
      if (prev && prev.deps.length === deps.length && prev.deps.every((d, i) => d === deps[i])) return prev.laid;
      laidCache.set(node, { deps, laid: ln });
      return ln;
    });
    return { laid, lay };
  }, [design, pageId]);
}

/**
 * A design: pages and layers, the canvas, the inspector and the design's
 * chat (docs/design/designs-editor.svg). Edits save themselves as versions
 * after a quiet moment; a version from elsewhere merges per node, and
 * overlapping changes ask the person.
 */
export function DesignEditor({ id }: { id: string }) {
  const router = useRouter();
  const search = useSearchParams();
  const ref = useDesign(id);
  const chatId = ref.data?.chat_id ?? "";
  const chat = useChat(chatId || undefined).data;
  const persona = (chat?.speaker ?? chat?.persona)?.name ?? "Assistant";
  const head = useDesignVersion(id, ref.data?.seq);
  const save = useSaveDesign(id);
  // mutateAsync is stable across renders; the mutation object isn't (a Slides review fix).
  const saveVersion = save.mutateAsync;
  const send = useSendChatMessage(chatId);
  useDesignFonts();
  const assetUrl = useCallback((a: string) => designAssetHref(id, a), [id]);

  // ---- The working copy (lib/designs/doc.ts) in a ref; `tick` re-renders on change.
  const docRef = useRef<W.Doc | null>(null);
  const [doc, setDocState] = useState<W.Doc | null>(null);
  const setDoc = (d: W.Doc) => {
    docRef.current = d;
    setDocState(d);
  };
  const [saveState, setSaveState] = useState<{ s: SaveState; message?: string }>({ s: "saved" });
  const [conflict, setConflict] = useState<{ theirs: Design; seq: number; layers: string[] } | null>(null);
  const conflicted = useRef(false);
  const timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const saving = useRef<Promise<void> | null>(null);
  const retries = useRef(0);
  const retryRef = useRef<() => void>(() => undefined);
  const scheduleRef = useRef<() => void>(() => undefined);
  const personaRef = useRef(persona);
  useEffect(() => {
    personaRef.current = persona;
  }, [persona]);
  const [loadError, setLoadError] = useState<string>();

  const runSave = useCallback(async (): Promise<void> => {
    for (let attempt = 0; attempt < 3; attempt++) {
      const cur = docRef.current;
      if (!cur || conflicted.current) return;
      if (!W.isDirty(cur)) {
        setSaveState({ s: "saved" });
        return;
      }
      const mine = cur.draft;
      const base = cur.saved;
      let body: string;
      try {
        // The layout's boxes are a cache in the body (ADR §3.2): written fresh on save.
        body = normalize(settle(mine)).json;
      } catch (e) {
        setSaveState({ s: "error", message: `This change can’t be saved: ${errorText(e)}` });
        return;
      }
      setSaveState({ s: "saving" });
      try {
        const v = await saveVersion({ base_seq: base.seq, body, note: "Edited" });
        retries.current = 0;
        const now = docRef.current!;
        setDoc(W.saved(now, v.seq, mine));
        if (now.draft === mine) setSaveState({ s: "saved" });
        else setSaveState({ s: "unsaved" });
        return;
      } catch (e) {
        if (e instanceof ApiError && e.status === 409) {
          // Saved elsewhere in between (a persona with direct edit, another tab): merge per node, or ask.
          let latest: { seq: number }, theirs: Design;
          try {
            latest = await fetchDesign(id);
            theirs = normalize((await fetchDesignVersion(id, latest.seq)).body).design;
          } catch (err) {
            setSaveState({ s: "error", message: `Couldn’t load the newer version: ${errorText(err)}` });
            retryRef.current();
            return;
          }
          const now = docRef.current!;
          // All three settled, so cached boxes don't look like edits on either side.
          const r = mergeDesigns(base.design, W.settled(now.draft), W.settled(theirs));
          if (r.design) {
            setDoc(W.merged(now, latest.seq, theirs, r.design));
            setSaveState({ s: "unsaved" });
            continue;
          }
          conflicted.current = true;
          clearTimeout(timer.current);
          const names = r.conflicts.map((c) => {
            const f = find(now.draft, c) ?? find(theirs, c);
            return f ? labelOf(f.node) : (now.draft.pages.find((p) => p.id === c) ?? theirs.pages.find((p) => p.id === c))?.name ?? "the design’s settings";
          });
          setConflict({ theirs, seq: latest.seq, layers: [...new Set(names)] });
          setSaveState({ s: "conflict", message: "This design changed while you were editing." });
        } else if (e instanceof ApiError && e.status === 422 && /changes nothing/.test(e.message)) {
          setDoc(W.saved(docRef.current!, base.seq, mine));
          setSaveState({ s: "saved" });
        } else {
          setSaveState({ s: "error", message: errorText(e) });
          retryRef.current();
        }
        return;
      }
    }
    setSaveState({ s: "error", message: "The design kept changing while saving." });
    retryRef.current();
  }, [saveVersion, id]);

  /** Saves now (after any save in flight). */
  const flush = useCallback(async () => {
    clearTimeout(timer.current);
    while (saving.current) await saving.current;
    if (docRef.current && W.isDirty(docRef.current) && !conflicted.current) {
      saving.current = runSave().finally(() => (saving.current = null));
      await saving.current;
    }
  }, [runSave]);

  /** Debounced autosave: every edit restarts the quiet time, so a burst of edits is one version. */
  const schedule = useCallback(
    (delay = AUTOSAVE_MS) => {
      clearTimeout(timer.current);
      const tick = () => {
        if (saving.current) {
          timer.current = setTimeout(tick, AUTOSAVE_MS);
          return;
        }
        saving.current = runSave().finally(() => (saving.current = null));
      };
      timer.current = setTimeout(tick, delay);
    },
    [runSave],
  );
  useEffect(() => {
    scheduleRef.current = () => schedule();
    retryRef.current = () => {
      retries.current += 1;
      schedule(Math.min(30_000, 2_000 * 2 ** (retries.current - 1)));
    };
  }, [schedule]);

  // Open the design, then follow the server: a version from elsewhere (a persona's direct edit, an
  // accepted suggestion) replaces a clean draft or merges into unsaved edits, and is one undo step.
  const [arrival, setArrival] = useState<{ seq: number; text: string }>();
  useEffect(() => {
    const v = head.data;
    if (!v) return;
    let d: Design;
    try {
      d = normalize(v.body).design;
    } catch (e) {
      // Showing why the body can't be opened (external data).
      // eslint-disable-next-line react-hooks/set-state-in-effect
      setLoadError(e instanceof DesignError ? e.message : errorText(e));
      return;
    }
    const cur = docRef.current;
    if (!cur) {
      setDoc(W.open(v.seq, d));
      return;
    }
    // A save in flight (or a conflict being asked about) meets the newer version itself, as a 409.
    if (v.seq <= cur.saved.seq || saving.current || conflicted.current) return;
    const r = adoptVersion(cur, v.seq, d);
    if (r.doc === cur) return;
    setDoc(r.doc);
    if (r.doc.past !== cur.past) {
      setArrival({ seq: v.seq, text: v.author_type === "agent" ? `${personaRef.current} edited the design` : "Accepted changes were applied" });
    }
    if (W.isDirty(r.doc)) {
      setSaveState({ s: "unsaved" });
      scheduleRef.current();
    }
  }, [head.data]);

  // Save on the way out: hidden tab, closed page or navigation.
  const flushRef = useRef(flush);
  useEffect(() => {
    flushRef.current = flush;
  }, [flush]);
  useEffect(() => {
    const dirty = () => !!docRef.current && W.isDirty(docRef.current);
    const hide = () => {
      if (document.visibilityState === "hidden" && dirty()) void flushRef.current().catch(() => undefined);
    };
    const leave = (e: BeforeUnloadEvent) => {
      if (dirty()) e.preventDefault();
    };
    document.addEventListener("visibilitychange", hide);
    window.addEventListener("beforeunload", leave);
    return () => {
      document.removeEventListener("visibilitychange", hide);
      window.removeEventListener("beforeunload", leave);
      if (dirty()) void flushRef.current().catch(() => undefined);
    };
  }, []);

  // ---- Review (a persona's suggestions as a diff): After and Diff show a preview, so nothing is editable there.
  const proposals = useDesignProposals(id, !!ref.data).data;
  const [reviewOpen, setReviewing] = useState(false);
  const reviewing = reviewOpen && !!proposals?.length;
  const [mode, setMode] = useState<ReviewMode>("diff");
  const [resolving, setResolving] = useState(false);
  const [resolveError, setResolveError] = useState<string>();
  const resolve = useResolveProposal(id);
  const settings = useDesignSettings(id);
  const viewOnly = reviewing && mode !== "before" && !!proposals?.length;
  const readOnly = !!conflict || !!chat?.archived || viewOnly;
  const readOnlyRef = useRef(readOnly);
  useEffect(() => {
    readOnlyRef.current = readOnly;
  }, [readOnly]);

  /** One edit of the working copy; key coalesces a drag or typing into one undo step. */
  const apply = useCallback(
    (fn: (d: Design) => Design, key?: string) => {
      const cur = docRef.current;
      if (!cur || readOnlyRef.current) return;
      const next = fn(cur.draft);
      if (next === cur.draft) return;
      setDoc(W.edit(cur, next, key));
      setSaveState({ s: "unsaved" });
      schedule();
    },
    [schedule],
  );
  const travel = (back: boolean) => {
    const cur = docRef.current;
    if (!cur || readOnly) return;
    const next = back ? W.undo(cur) : W.redo(cur);
    if (next === cur) return;
    setDoc(next);
    setEditing(undefined);
    setSaveState({ s: "unsaved" });
    schedule();
  };

  // ---- Page, selection, tools and camera.
  const design = doc?.draft;
  const review = useReview(design, proposals, reviewing);
  // What the canvas draws: the working copy, or in After / Diff the suggestion applied to it.
  const shown = viewOnly && review ? review.after : design;
  const [pageId, setPageId] = useState<string>();
  const page = shown?.pages.find((p) => p.id === pageId) ?? shown?.pages[0];
  const [selectedRaw, setSelected] = useState<string[]>([]);
  const [editing, setEditing] = useState<string>();
  const [tool, setTool] = useState<Tool>("select");
  const { laid, lay } = useLaidPage(shown, page?.id);
  // Ids that left the page (undo, another page) drop out of the selection.
  const selected = useMemo(() => (laid ? selectedRaw.filter((s) => laid.boxes.has(s)) : []), [selectedRaw, laid]);
  const [camera, setCameraState] = useState<Camera | null>(null);
  const [viewport, setViewport] = useState({ w: 1200, h: 800, known: false });
  const camKey = page ? `designs.camera.${id}.${page.id}` : "";
  const setCamera = useCallback((c: Camera | ((c: Camera) => Camera)) => setCameraState((prev) => (typeof c === "function" ? (prev ? c(prev) : prev) : c)), []);
  // Each page keeps its own camera in this browser (ADR §3.2); a page opens fitted the first time.
  useEffect(() => {
    if (!camKey || !laid || !viewport.known) return;
    let c: Camera | null = null;
    try {
      const v = JSON.parse(localStorage.getItem(camKey) ?? "null");
      if (v && [v.x, v.y, v.z].every(Number.isFinite)) c = v;
    } catch {
      // Unreadable storage: fit the page.
    }
    // Choosing the camera once the page is known (browser storage).
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setCameraState(c ?? fitCamera(laid.box, viewport.w, viewport.h));
    // eslint-disable-next-line react-hooks/exhaustive-deps -- only when the page changes
  }, [camKey, !!laid, viewport.known]);
  useEffect(() => {
    if (!camera || !camKey) return;
    const t = setTimeout(() => {
      try {
        localStorage.setItem(camKey, JSON.stringify(camera));
      } catch {
        // Storage full or blocked: the camera resets next time.
      }
    }, 300);
    return () => clearTimeout(t);
  }, [camera, camKey]);
  const zoomBy = (k: number) => setCamera((c) => zoomAt(c, c.z * k, viewport.w / 2, viewport.h / 2));
  const zoomToFit = () => laid && setCameraState(fitCamera(selected.length ? unionOf(selected) : laid.box, viewport.w, viewport.h));
  const unionOf = (ids: string[]) => {
    const bs = ids.map((s) => laid!.boxes.get(s)).filter((b) => !!b);
    const x = Math.min(...bs.map((b) => b.x)), y = Math.min(...bs.map((b) => b.y));
    return { x, y, w: Math.max(...bs.map((b) => b.x + b.w)) - x, h: Math.max(...bs.map((b) => b.y + b.h)) - y };
  };

  // ---- Commands on the selection: each is one undo step.
  const removeSelected = () => {
    if (!selected.length) return;
    apply((d) => removeNodes(d, new Set(selected)));
    setSelected([]);
  };
  const duplicateSelected = () => {
    if (!selected.length || !doc) return;
    let made: string[] = [];
    apply((d) => {
      const r = duplicateNodes(d, selected);
      made = r.ids;
      return r.design;
    });
    if (made.length) setSelected(made);
  };
  const group = () => {
    let gid = "";
    apply((d) => {
      const r = groupNodes(d, selected);
      if (!r) return d;
      gid = r.id;
      return r.design;
    });
    if (gid) setSelected([gid]);
  };
  const ungroup = () => {
    let freed: string[] = [];
    apply((d) => {
      const r = ungroupNodes(d, selected);
      freed = r.ids;
      return r.design;
    });
    if (freed.length) setSelected(freed);
  };
  const order = (op: ZOrder) => selected.length && apply((d) => reorder(d, new Set(selected), op));
  const nudge = (dx: number, dy: number) =>
    selected.length &&
    apply((d) => mapNodes(d, new Set(selected.filter((s) => {
      const f = find(d, s);
      return f && f.node.type !== "connector" && !inAutoLayout(f.parent) && !f.node.locked;
    })), (n) => (n.type === "connector" ? n : ({ ...n, x: n.x + dx, y: n.y + dy } as DesignNode))), `nudge:${selected.join(",")}`);
  const toggleAutoLayout = () => {
    if (selected.length !== 1) return;
    apply((d) => mapNodes(d, new Set(selected), (n) => {
      if (n.type !== "frame" && n.type !== "component") return n;
      if (n.auto_layout) return n;
      return { ...n, auto_layout: { direction: "vertical", gap: 16, padding: [16, 16, 16, 16] } };
    }));
  };

  // ---- Images: uploaded, added to the assets, placed in the frame under the viewport centre.
  const fileInput = useRef<HTMLInputElement>(null);
  const [notice, setNotice] = useState<string>();
  const [uploading, setUploading] = useState(false);
  const onImage = async (file: File) => {
    const target = page?.id;
    if (!target || !camera) return;
    setNotice(undefined);
    setUploading(true);
    try {
      const blob = await prepareImage(file);
      if (blob.size > 10 << 20) throw new Error(`${file.name} is ${(blob.size / (1 << 20)).toFixed(0)} MB: the limit is 10 MB`);
      const a = await uploadDesignAsset(id, blob);
      if (readOnlyRef.current) throw new Error("the design became read-only while uploading; add it again once you can edit");
      const k = Math.min(600 / a.w, 600 / a.h, 1);
      const w = Math.round(a.w * k), h = Math.round(a.h * k);
      const cx0 = (viewport.w / 2 - camera.x) / camera.z, cy0 = (viewport.h / 2 - camera.y) / camera.z;
      const node: ImageNode = { id: newId("n"), type: "image", name: file.name.replace(/\.[a-z0-9]+$/i, "").slice(0, 200) || "Image", asset: a.id, x: Math.round(cx0 - w / 2), y: Math.round(cy0 - h / 2), w, h, alt: file.name.replace(/\.[a-z0-9]+$/i, "").slice(0, 500) };
      const board = laid ? dropTarget(page, laid.boxes, cx0, cy0) : undefined;
      const bb = board && laid?.boxes.get(board.id);
      if (board && bb) Object.assign(node, { x: node.x - Math.round(bb.x), y: node.y - Math.round(bb.y) });
      apply((d) => insertNodes({ ...d, assets: { ...d.assets, [a.id]: { mime: a.mime as "image/png" | "image/jpeg", w: a.w, h: a.h } } }, board?.id ?? target, [node]));
      setSelected([node.id]);
    } catch (e) {
      setNotice(`Couldn’t add ${file.name}: ${errorText(e)}`);
    } finally {
      setUploading(false);
    }
  };

  // ---- Keyboard.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const t = e.target as HTMLElement;
      if (t.isContentEditable || t instanceof HTMLInputElement || t instanceof HTMLTextAreaElement || t instanceof HTMLSelectElement) return;
      // The chat rail takes its own keys.
      if (t.closest?.("[data-design-chat]")) return;
      const mod = e.metaKey || e.ctrlKey;
      const k = e.key.toLowerCase();
      if (mod && k === "z") {
        e.preventDefault();
        travel(!e.shiftKey);
      } else if (mod && k === "y") {
        e.preventDefault();
        travel(false);
      } else if (mod && k === "s") {
        e.preventDefault();
        void flush();
      } else if (mod && k === "a") {
        e.preventDefault();
        if (page) setSelected(topLevelIds(page));
      } else if (mod && k === "d") {
        e.preventDefault();
        if (!readOnly) duplicateSelected();
      } else if (mod && k === "g") {
        e.preventDefault();
        if (readOnly) return;
        if (e.shiftKey) ungroup();
        else group();
      } else if (mod && (e.code === "BracketRight" || e.code === "BracketLeft")) {
        e.preventDefault();
        if (!readOnly) order(e.code === "BracketRight" ? (e.altKey ? "front" : "forward") : e.altKey ? "back" : "backward");
      } else if (mod && e.shiftKey && k === "k") {
        e.preventDefault();
        if (!readOnly) fileInput.current?.click();
      } else if (mod && (k === "=" || k === "+")) {
        e.preventDefault();
        zoomBy(1.25);
      } else if (mod && k === "-") {
        e.preventDefault();
        zoomBy(0.8);
      } else if ((mod && k === "0") || (e.shiftKey && e.code === "Digit0")) {
        e.preventDefault();
        setCamera((c) => zoomAt(c, 1, viewport.w / 2, viewport.h / 2));
      } else if (e.shiftKey && e.code === "Digit1") {
        e.preventDefault();
        zoomToFit();
      } else if (e.shiftKey && k === "a" && !mod) {
        e.preventDefault();
        if (!readOnly) toggleAutoLayout();
      } else if (e.key === "Escape") {
        if (tool !== "select") setTool("select");
        else if (selected.length) {
          // Up to the parent first, then nothing.
          const f = selected.length === 1 && design ? find(design, selected[0]) : undefined;
          setSelected(f?.parent ? [f.parent.id] : []);
        }
      } else if ((e.key === "Delete" || e.key === "Backspace") && !readOnly) {
        e.preventDefault();
        removeSelected();
      } else if (e.key === "Enter" && selected.length === 1 && design) {
        e.preventDefault();
        const n = find(design, selected[0])?.node;
        if (n?.type === "text" && !readOnly) setEditing(n.id);
        else if (n && "children" in n && n.children.length) setSelected(n.children.filter((c) => c.type !== "connector").map((c) => c.id));
      } else if (e.key.startsWith("Arrow") && !mod && !readOnly && selected.length) {
        e.preventDefault();
        const s = e.shiftKey ? 10 : 1;
        nudge(e.key === "ArrowLeft" ? -s : e.key === "ArrowRight" ? s : 0, e.key === "ArrowUp" ? -s : e.key === "ArrowDown" ? s : 0);
      } else if (!mod && !e.altKey && !e.shiftKey && TOOL_KEYS[k] && !readOnly) {
        setTool(TOOL_KEYS[k]);
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  });

  // Every message sent from this design's chat carries what the person has selected and is looking at.
  const ctx = useRef({ page: "", selected: [] as string[], camera: null as Camera | null, w: 1200, h: 800 });
  useEffect(() => {
    ctx.current = { page: page?.id ?? "", selected, camera, w: viewport.w, h: viewport.h };
  });
  useEffect(() => {
    if (!chatId) return;
    return provideChatRefs(chatId, () => {
      const c = ctx.current;
      if (!c.page) return undefined;
      const z = c.camera?.z ?? 1;
      const viewportBox = c.camera ? { x: Math.round(-c.camera.x / z), y: Math.round(-c.camera.y / z), w: Math.round(c.w / z), h: Math.round(c.h / z), zoom: z } : { x: 0, y: 0, w: Math.round(c.w), h: Math.round(c.h), zoom: 1 };
      return [{ kind: "canvas", id, selection: { page: c.page, nodes: c.selected.slice(0, 100), viewport: viewportBox } }];
    });
  }, [chatId, id]);

  // ---- Presence: the persona's running turn names the canvas and nodes it is working on.
  const running = chat?.state === "running";
  const events = useChatRunEvents(chat?.current_run_id, running && !!ref.data).data;
  const presence = useMemo(() => (running ? presenceOf(toolCalls(events), id, true) : undefined), [events, running, id]);
  const presenceBoxes = useMemo(() => (presence && laid ? presence.nodes.flatMap((n) => laid.boxes.get(n) ?? []) : []), [presence, laid]);
  const [follow, setFollow] = useState(false);
  const paused = !!ref.data?.settings?.persona_paused;
  const presenceKey = presenceBoxes.map((b) => `${b.x},${b.y}`).join(";");
  useEffect(() => {
    if (!follow || !presenceBoxes.length || !viewport.known) return;
    const x0 = Math.min(...presenceBoxes.map((b) => b.x)), y0 = Math.min(...presenceBoxes.map((b) => b.y));
    const x1 = Math.max(...presenceBoxes.map((b) => b.x + b.w)), y1 = Math.max(...presenceBoxes.map((b) => b.y + b.h));
    // Following means the camera tracks an external thing (the persona's turn).
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setCamera((c) => ({ ...c, x: viewport.w / 2 - ((x0 + x1) / 2) * c.z, y: viewport.h / 2 - ((y0 + y1) / 2) * c.z }));
    // eslint-disable-next-line react-hooks/exhaustive-deps -- follows when the nodes it names move
  }, [follow, presenceKey, viewport.known]);

  // A change picked in the review: its page, and the camera moves there when it is off screen.
  const focusChange = (c: Change) => {
    review?.setFocus(c.i);
    if (c.page) setPageId(c.page);
    const b = c.box;
    if (!b || !camera) return;
    const sx = b.x * camera.z + camera.x, sy = b.y * camera.z + camera.y;
    if (c.page === page?.id && sx >= 0 && sy >= 0 && sx + b.w * camera.z <= viewport.w && sy + b.h * camera.z <= viewport.h) return;
    setCamera((cam) => ({ ...cam, x: viewport.w / 2 - (b.x + b.w / 2) * cam.z, y: viewport.h / 2 - (b.y + b.h / 2) * cam.z }));
  };
  /** Done: saves what is unsaved first, so the accepted ops land on it; the version that comes back merges into anything typed meanwhile. */
  const finishReview = async () => {
    if (!review?.body) return;
    setResolving(true);
    setResolveError(undefined);
    try {
      await flush();
      if (conflicted.current) throw new Error("Choose between your version and the latest first.");
      await resolve.mutateAsync({ proposalId: review.proposal.id, decisions: review.body.decisions });
      setSelected([]);
      setMode("diff");
    } catch (e) {
      setResolveError(errorText(e));
    } finally {
      setResolving(false);
    }
  };
  // A design started from a description or a ticket key asks its persona to draft it, once.
  const brief = search.get("brief");
  const briefSent = useRef(false);
  useEffect(() => {
    if (!brief || !chatId || !doc || briefSent.current) return;
    briefSent.current = true;
    router.replace(`/apps/designs/${id}`);
    void send.mutateAsync({ body: `Design this: ${brief}` }).catch((e) => setNotice(`Couldn’t ask ${persona} to draft it: ${errorText(e)}`));
  }, [brief, chatId, doc, id, persona, router, send]);

  const [chatOpen, setChatOpen] = useState(true);
  const [tab, setTab] = useState<"Design" | "Prototype">("Design");
  /** Present mode plays the saved design, so unsaved edits are saved first. */
  const present = async (frame?: string) => {
    try {
      await flush();
    } catch {
      // The save banner explains; Present shows the last saved version.
    }
    const q = new URLSearchParams();
    if (frame) q.set("start", frame);
    else if (selected.length === 1) q.set("start", selected[0]);
    if (page) q.set("page", page.id);
    router.push(`/present/designs/${id}?${q}`);
  };
  const title = ref.data?.title ?? "Design";
  const fetchError = ref.error ?? head.error;
  const selectionLabel = design && selected.length === 1 ? find(design, selected[0])?.node : undefined;

  return (
    <div className="flex h-dvh flex-col">
      <TopBar
        crumbs={[{ label: "Apps" }, { label: "Designs", href: "/apps/designs" }, { label: title }]}
        actions={
          ref.data && (
            <>
              {doc && (
                <span data-testid="design-save-state" title={saveState.message} className={cx("text-[12px]", saveState.s === "error" || saveState.s === "conflict" ? "text-pill-blocked-fg" : "text-fg-tertiary")}>
                  {SAVE_LABEL[saveState.s]} · v{doc.saved.seq}
                  {saveState.s === "error" && (
                    <button type="button" className="ml-1.5 text-accent hover:underline" onClick={() => void flush().catch(() => undefined)}>
                      Retry
                    </button>
                  )}
                </span>
              )}
              <Button variant="secondary" disabled={!design} onClick={() => void present()} title="Play the prototype from the selected frame">
                ▶ Present
              </Button>
              <label className="flex cursor-pointer items-center gap-2 text-[12.5px] text-fg-secondary" title={`Off: ${persona}’s edits wait as suggestions you review. On: they apply at once, and Undo reverts them.`}>
                <input
                  type="checkbox"
                  role="switch"
                  aria-label="Let personas edit directly"
                  checked={!!ref.data.settings?.direct_edit}
                  disabled={settings.isPending}
                  onChange={(e) => settings.mutate({ direct_edit: e.target.checked })}
                  className="size-[14px] accent-[var(--color-accent)]"
                />
                Let personas edit directly
              </label>
              {!!proposals?.length && (
                <Button variant="primary" aria-pressed={reviewing} onClick={() => setReviewing((r) => !r)}>
                  Review {proposals.length} {proposals.length === 1 ? "suggestion" : "suggestions"}
                </Button>
              )}
              <Button variant="secondary" aria-pressed={chatOpen} onClick={() => setChatOpen((o) => !o)}>
                ✦ {chatOpen ? "Hide chat" : `Chat with ${persona}`}
              </Button>
            </>
          )
        }
      />
      {fetchError || loadError ? (
        <div className="mx-auto w-full max-w-[720px] p-6">
          <Banner tone="blocked" title="Couldn’t open this design" actions={<Button onClick={() => router.push("/apps/designs")}>Back to Designs</Button>}>
            {loadError ?? (fetchError instanceof ApiError && fetchError.status === 404 ? "It may have been deleted, or it belongs to someone else." : errorText(fetchError))}
          </Banner>
        </div>
      ) : !design || !shown || !page || !laid || !lay ? (
        <div aria-busy="true" className="flex min-h-0 flex-1 gap-3 p-3">
          <Skeleton className="w-[240px]" />
          <Skeleton className="flex-1" />
          <Skeleton className="w-[260px]" />
        </div>
      ) : (
        <div className="relative flex min-h-0 flex-1">
          {chatOpen && (
            <aside data-design-chat aria-label={`Chat with ${persona}`} className="flex min-h-0 w-[320px] flex-none flex-col border-r border-border-subtle bg-sidebar max-xl:absolute max-xl:inset-y-0 max-xl:left-0 max-xl:z-30 max-xl:shadow-[var(--shadow-float)]">
              {chatId && <Conversation key={chatId} id={chatId} onNew={() => router.push("/apps/designs")} />}
            </aside>
          )}
          <nav aria-label="Pages and layers" className="flex min-h-0 w-[232px] flex-none flex-col border-r border-border-subtle bg-surface max-md:hidden">
            <LayersPanel
              design={shown}
              page={page}
              onPage={(pid) => {
                setPageId(pid);
                setSelected([]);
                setEditing(undefined);
              }}
              selected={selected}
              onSelect={setSelected}
              onEdit={apply}
              readOnly={readOnly}
            />
          </nav>
          <main className="relative flex min-h-0 min-w-0 flex-1 flex-col">
            <div className="absolute inset-x-3 top-3 z-10 flex flex-col gap-2">
              {conflict && (
                <Banner
                  tone="blocked"
                  title="This design changed while you were editing"
                  actions={
                    <>
                      <Button
                        variant="secondary"
                        onClick={() => {
                          conflicted.current = false;
                          setDoc(W.takeTheirs(conflict.seq, conflict.theirs));
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
                          setDoc(W.keepMine(docRef.current!, conflict.seq, conflict.theirs));
                          setConflict(null);
                          void flush().catch(() => undefined);
                        }}
                      >
                        Keep mine
                      </Button>
                    </>
                  }
                >
                  You and {persona} both changed {conflict.layers.slice(0, 3).join(", ")}
                  {conflict.layers.length > 3 ? ` and ${conflict.layers.length - 3} more` : ""}. Nothing is saved until you choose: Keep mine saves your version over theirs (theirs stays in the version history), or use the latest version and drop your unsaved edits.
                </Banner>
              )}
              {presence && (
                <div role="status" data-testid="persona-chip" className="pointer-events-auto flex items-center gap-2 self-start rounded-full border border-[#3FC1B0] bg-surface-raised py-1 pl-1 pr-2 text-[12.5px] text-fg shadow-[var(--shadow-float)]">
                  <span aria-hidden className="grid size-5 place-items-center rounded-full bg-[#3FC1B024] text-[10px] font-bold text-[#3FC1B0]">{persona.slice(0, 1).toUpperCase()}</span>
                  {persona.slice(0, 1).toUpperCase()} is editing
                  <Button variant="ghost" className="h-[22px] px-1.5 text-[12px]" aria-pressed={follow} onClick={() => setFollow((f) => !f)}>Follow</Button>
                  <Button variant="ghost" className="h-[22px] px-1.5 text-[12px]" disabled={settings.isPending} onClick={() => settings.mutate({ persona_paused: true })}>Pause</Button>
                </div>
              )}
              {paused && (
                <div role="status" data-testid="persona-paused" className="pointer-events-auto flex items-center gap-2 self-start rounded-full border border-border-float bg-surface-raised px-3 py-1 text-[12.5px] text-fg-secondary shadow-[var(--shadow-float)]">
                  {persona}’s edits are paused
                  <Button variant="ghost" className="h-[22px] px-1.5 text-[12px]" disabled={settings.isPending} onClick={() => settings.mutate({ persona_paused: false })}>Resume</Button>
                </div>
              )}
              {arrival && !reviewing && (
                <div role="status" data-testid="design-arrival" className="pointer-events-auto flex items-center gap-2 self-start rounded-md bg-surface-raised px-3 py-1.5 text-[12.5px] text-fg shadow-[var(--shadow-float)]">
                  {arrival.text}
                  <Button variant="ghost" className="h-[22px] px-1.5 text-[12px] text-accent" onClick={() => (travel(true), setArrival(undefined))}>Undo</Button>
                  <Button variant="ghost" className="h-[22px] px-1.5 text-[12px]" aria-label="Dismiss" onClick={() => setArrival(undefined)}>✕</Button>
                </div>
              )}
              {!reviewing && !!proposals?.length && (
                <Banner tone="human" title={`${persona} suggested changes`} actions={<Button variant="approve" onClick={() => setReviewing(true)}>Review {proposals.length} {proposals.length === 1 ? "suggestion" : "suggestions"}</Button>}>
                  {proposals[0].summary || `${proposals[0].ops.length} changes`} · nothing changes until you accept.
                </Banner>
              )}
              {reviewing && review && <ReviewBar review={review} persona={persona} mode={mode} onMode={setMode} busy={resolving} onDone={() => void finishReview()} onClose={() => setReviewing(false)} />}
              {chat?.archived && <Banner tone="human" title="This design’s chat is archived">Restore the chat to edit the design.</Banner>}
              {saveState.s === "error" && <Banner tone="blocked" title="Couldn’t save your changes" actions={<Button onClick={() => void flush().catch(() => undefined)}>Retry now</Button>}>{saveState.message} Your edits are kept in this tab; don’t close it yet.</Banner>}
              {(notice || uploading) && <p role={notice ? "alert" : "status"} className={cx("self-start rounded-md bg-surface px-2 py-1 text-[12px] shadow", notice ? "text-pill-blocked-fg" : "text-fg-secondary")}>{notice ?? "Uploading image…"}</p>}
            </div>
            {(
              <DesignCanvas
                design={shown}
                page={page}
                laid={laid}
                selected={selected}
                onSelect={setSelected}
                tool={tool}
                onTool={setTool}
                readOnly={readOnly}
                assetUrl={assetUrl}
                camera={camera ?? HIDDEN_CAMERA}
                onCamera={setCamera}
                onEdit={apply}
                editing={editing}
                onEditing={setEditing}
                onViewport={(w, h) => setViewport({ w, h, known: true })}
                wires={tab === "Prototype"}
              />
            )}
            {reviewing && review && <ReviewOverlay review={review} mode={mode} page={page.id} camera={camera ?? HIDDEN_CAMERA} />}
            {presence && <PresenceLayer name={persona} boxes={presenceBoxes} camera={camera ?? HIDDEN_CAMERA} busy={presence.busy} />}
            <div role="toolbar" aria-label="Tools" className="absolute bottom-4 left-1/2 z-10 flex -translate-x-1/2 items-center gap-0.5 rounded-xl border border-border-float bg-surface-raised p-1 shadow-[var(--shadow-float)]">
              {TOOLS.map((t) => (
                <button
                  key={t.tool}
                  type="button"
                  aria-label={`${t.label} (${t.key})`}
                  title={`${t.label} (${t.key})`}
                  aria-pressed={t.tool !== "image" ? tool === t.tool : undefined}
                  disabled={readOnly && t.tool !== "select" && t.tool !== "hand"}
                  className={cx("grid size-[32px] place-items-center rounded-lg text-[15px] disabled:opacity-40", tool === t.tool ? "bg-accent text-fg-on-accent" : "text-fg-2 hover:bg-hover")}
                  onClick={() => (t.tool === "image" ? fileInput.current?.click() : setTool(t.tool))}
                >
                  {t.glyph}
                </button>
              ))}
              <span aria-hidden className="mx-1 h-5 w-px bg-border" />
              <button type="button" className="flex h-[32px] items-center gap-1.5 rounded-lg px-3 text-[12.5px] font-medium text-accent hover:bg-hover" onClick={() => {
                setChatOpen(true);
                requestAnimationFrame(() => document.querySelector<HTMLTextAreaElement>("[data-design-chat] textarea")?.focus());
              }}>
                ✦ Ask
              </button>
            </div>
            <div className="absolute bottom-5 right-4 z-10 flex items-center gap-1 rounded-lg border border-border-float bg-surface-raised px-1 py-0.5 text-[12px] text-fg-secondary shadow-[var(--shadow-float)]">
              <button type="button" aria-label="Zoom out" className="grid size-6 place-items-center rounded hover:bg-hover" onClick={() => zoomBy(0.8)}>−</button>
              <button type="button" aria-label="Zoom to fit (⇧1)" data-testid="design-zoom" className="min-w-[44px] rounded px-1 hover:bg-hover" onClick={zoomToFit}>
                {Math.round((camera?.z ?? 1) * 100)}%
              </button>
              <button type="button" aria-label="Zoom in" className="grid size-6 place-items-center rounded hover:bg-hover" onClick={() => zoomBy(1.25)}>+</button>
            </div>
            <input ref={fileInput} type="file" accept="image/png,image/jpeg,image/*" className="hidden" aria-label="Add an image to the design" data-testid="design-image-input" onChange={(e) => {
              const f = e.target.files?.[0];
              e.target.value = "";
              if (f) void onImage(f);
            }} />
          </main>
          {reviewing && review ? (
            <aside aria-label="Review" className="flex min-h-0 w-[320px] flex-none flex-col border-l border-border-subtle bg-surface max-lg:hidden">
              <ReviewPanel review={review} design={shown} chatId={chatId} canvasId={id} seq={doc?.saved.seq ?? 0} persona={persona} error={resolveError} busy={resolving} onFocus={focusChange} />
            </aside>
          ) : (
          <aside aria-label="Inspector" className="flex min-h-0 w-[264px] flex-none flex-col border-l border-border-subtle bg-surface max-lg:hidden">
            <div role="tablist" aria-label="Inspector" className="flex gap-4 border-b border-border-subtle px-4 pt-3">
              {(["Design", "Prototype", "Inspect"] as const).map((t) => (
                <button key={t} type="button" role="tab" aria-selected={t === tab} disabled={t === "Inspect"} title={t === "Inspect" ? "Coming in a later release" : undefined} onClick={() => t !== "Inspect" && setTab(t)} className={cx("-mb-px border-b-2 pb-2 text-[12.5px]", t === tab ? "border-accent font-medium text-fg" : "border-transparent text-fg-tertiary", t !== "Inspect" && t !== tab && "hover:text-fg")}>
                  {t}
                </button>
              ))}
            </div>
            <div className="min-h-0 flex-1 overflow-y-auto" role="tabpanel" aria-label={selectionLabel ? `${tab}: ${labelOf(selectionLabel)}` : tab}>
              {tab === "Design" ? (
                <Inspector design={design} selected={selected} lay={lay} onEdit={apply} readOnly={readOnly} onGroup={group} onRemove={removeSelected} />
              ) : (
                <PrototypePanel design={design} selected={selected} readOnly={readOnly} onEdit={apply} onSelect={setSelected} onPresent={(f) => void present(f)} />
              )}
            </div>
          </aside>
          )}
        </div>
      )}
    </div>
  );
}
