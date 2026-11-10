"use client";

import { useRouter } from "next/navigation";
import { useEffect, useMemo, useRef, useState } from "react";
import { useCanvasActions, useCanvasComments, useChatCanvas } from "@/lib/api/canvas-hooks";
import { useDoc, useMarkDocReviewed } from "@/lib/api/docs-hooks";
import { useChat, useSendChatMessage } from "@/lib/api/hooks";
import type { Schemas } from "@/lib/api/client";
import { editorKeeps, locateQuote } from "@/lib/doc-markdown";
import { reviewBase, reviewSegments } from "@/lib/doc-review";
import { cx } from "@/lib/cx";
import { Button } from "@/components/ui/button";
import { Banner, Skeleton } from "@/components/ui/primitives";
import { TopBar } from "@/components/shell/topbar";
import { CanvasView, Rail, type Target } from "@/components/domain/canvas-view";
import { Conversation } from "@/app/(app)/chat/_components/conversation";
import { DocEditor, type DocEditorHandle, type Picked, type SaveState } from "./doc-editor";
import { DocReview } from "./doc-review";
import { ExportMenu } from "./export-menu";

type Comment = Schemas["ChatCanvasComment"];

const errorText = (e: unknown) => (e instanceof Error ? e.message : "Something went wrong. Please try again.");

const SAVE_LABEL: Record<SaveState, string> = { saved: "Saved", unsaved: "Unsaved changes", saving: "Saving…", conflict: "Not saved", error: "Not saved" };

/**
 * A document full page: you type on it, its persona works on it with you.
 * The document's own chat sits beside it; the persona's edits come back as
 * suggestions to accept or reject, and comments sit in the margin.
 */
export function DocScreen({ id }: { id: string }) {
  const doc = useDoc(id);
  const router = useRouter();
  const chatId = doc.data?.chat_id ?? "";
  const chat = useChat(chatId || undefined).data;
  const live = chat?.state === "running";
  const readOnly = !!chat?.archived;
  const persona = (chat?.speaker ?? chat?.persona)?.name ?? "Assistant";
  const head = useChatCanvas(chatId, chatId ? id : undefined, undefined, live);
  const c = head.data;
  const [chatOpen, setChatOpen] = useState(true);

  // The persona's versions since the person's last edit or review wait on
  // them. Nothing is decided mid-turn: the changes are still coming in.
  const base = c ? reviewBase(c.versions, doc.data?.reviewed_seq ?? 0) : 0;
  const reviewing = !!c && !live && base > 0 && c.seq > base;
  const before = useChatCanvas(chatId, reviewing ? id : undefined, base).data;
  const baseBody = before?.viewing_seq === base ? before.body : undefined;
  const actions = useCanvasActions(chatId, id);
  const reviewed = useMarkDocReviewed(id);
  // Versions that changed nothing in the end need no decision.
  const empty = reviewing && baseBody !== undefined && c && !reviewSegments(baseBody, c.body).some((s) => s.kind === "change");
  useEffect(() => {
    if (empty && c && !reviewed.isPending) reviewed.mutate(c.seq);
  }, [empty, c, reviewed]);

  const editable = useMemo(() => (c ? editorKeeps(c.body) : true), [c]);

  // The version the editor's text was last saved as; comments anchor to it.
  const saved = useRef<{ seq: number; body: string }>({ seq: 0, body: "" });
  useEffect(() => {
    if (c && c.seq > saved.current.seq) saved.current = { seq: c.seq, body: c.body };
  }, [c]);
  const editor = useRef<DocEditorHandle>(null);
  const [saveState, setSaveState] = useState<{ s: SaveState; message?: string }>({ s: "saved" });

  const send = useSendChatMessage(chatId);
  const [notice, setNotice] = useState<string>();
  const [target, setTarget] = useState<{ t: Extract<Target, { kind: "text" }>; seq: number } | null>(null);
  const [activeId, setActiveId] = useState<string>();
  const comments = useCanvasComments(chatId, chatId ? id : undefined, live).data?.comments;
  const { open, resolved, replies } = useMemo(() => {
    const replies = new Map<string, Comment[]>();
    const top: Comment[] = [];
    for (const x of comments ?? []) {
      if (x.parent_id) replies.set(x.parent_id, [...(replies.get(x.parent_id) ?? []), x]);
      else top.push(x);
    }
    return { open: top.filter((x) => x.status !== "resolved"), resolved: top.filter((x) => x.status === "resolved"), replies };
  }, [comments]);
  const highlights = useMemo(() => open.filter((x) => !x.target_lost && x.anchor.quote).map((x) => ({ quote: x.anchor.quote!, active: x.id === activeId })), [open, activeId]);

  const ask = async (body: string) => {
    setChatOpen(true);
    await send.mutateAsync({ body });
  };
  const flushed = async () => {
    await editor.current?.flush();
    return saved.current;
  };
  // A selection either opens a comment in the margin, or goes to the persona as one.
  const pick = async (p: Picked, message?: string) => {
    setNotice(undefined);
    try {
      const at = await flushed();
      const r = locateQuote(at.body, p.quote, p.at * at.body.length);
      if (!message) {
        if (r) setTarget({ t: { kind: "text", start: r.start, end: r.end, quote: p.quote.slice(0, 140) }, seq: at.seq });
        else setNotice("Couldn't place a comment on that selection. Try a shorter one.");
        return;
      }
      if (r) {
        const made = await actions.add.mutateAsync({ seq: at.seq, body: message, start: r.start, end: r.end });
        await actions.send.mutateAsync({ comment_ids: [made.id], message });
        setChatOpen(true);
      } else await ask(`${message}\n\nThe passage, in the document:\n> ${p.quote.replace(/\n/g, "\n> ")}`);
    } catch (e) {
      setNotice(errorText(e));
    }
  };
  const mention = async (instruction: string, after: string) => {
    setNotice(undefined);
    try {
      await flushed();
      const where = after ? `right after the passage “${after.length > 160 ? `${after.slice(0, 160)}…` : after}”` : "at the start";
      await ask(`${instruction}\n\n(Asked from inside the document, ${where}. Write it into the document there.)`);
    } catch (e) {
      setNotice(errorText(e));
    }
  };

  const title = c?.title ?? doc.data?.title ?? "Document";
  const showRail = !reviewing && editable && (open.length > 0 || resolved.length > 0 || !!target);

  return (
    <div className="flex h-dvh flex-col">
      <TopBar
        crumbs={[{ label: "Apps" }, { label: "Docs", href: "/apps/docs" }, { label: title }]}
        actions={
          doc.data && (
            <>
              <span data-testid="doc-presence" className="flex items-center gap-1.5 text-[12px] text-fg-tertiary">
                <span aria-hidden className={cx("grid size-[18px] place-items-center rounded-full bg-accent-soft text-[10px] font-bold text-accent", live && "animate-pulse")}>{persona.slice(0, 1)}</span>
                {live ? `${persona} is editing…` : reviewing ? `${persona} suggested changes` : `${persona} can edit`}
              </span>
              {c && (
                <span data-testid="doc-save-state" title={saveState.message} className={cx("text-[12px]", saveState.s === "error" || saveState.s === "conflict" ? "text-pill-blocked-fg" : "text-fg-tertiary")}>
                  {SAVE_LABEL[saveState.s]} · v{c.seq}
                </span>
              )}
              <ExportMenu id={id} title={title} current={async () => (await flushed()).body} />
              <Button variant={chatOpen ? "secondary" : "primary"} aria-pressed={chatOpen} onClick={() => setChatOpen((o) => !o)}>
                ✦ {chatOpen ? "Hide chat" : `Chat with ${persona}`}
              </Button>
            </>
          )
        }
      />
      {doc.isError || head.isError ? (
        <div className="mx-auto w-full max-w-[720px] p-6">
          <Banner tone="blocked" title="Couldn’t open this document">{errorText(doc.error ?? head.error)}</Banner>
        </div>
      ) : !c ? (
        <div aria-busy="true" className="mx-auto flex w-full max-w-[820px] flex-col gap-3 p-6">
          <Skeleton className="h-7 w-1/2" />
          <Skeleton className="h-[320px]" />
        </div>
      ) : (
        <div className="relative flex min-h-0 flex-1">
          {chatOpen && (
            <aside aria-label={`Chat with ${persona}`} data-testid="doc-chat" className="flex min-h-0 w-[380px] flex-none flex-col border-r border-border-subtle bg-sidebar max-lg:absolute max-lg:inset-y-0 max-lg:left-0 max-lg:z-30 max-lg:w-[min(380px,100%)] max-lg:shadow-[var(--shadow-float)]">
              {/* A document keeps its one chat: /new starts another document instead. */}
              <Conversation key={chatId} id={chatId} onNew={() => router.push("/apps/docs")} />
            </aside>
          )}
          <main data-testid="doc-page" className="flex min-h-0 min-w-0 flex-1">
            <div className="min-h-0 min-w-0 flex-1 overflow-y-auto">
              {notice && (
                <p role="alert" className="mx-auto mt-3 max-w-[720px] px-8 text-xs text-pill-blocked-fg">
                  {notice}
                </p>
              )}
              {saveState.s === "conflict" && (
                <div className="mx-auto mt-3 max-w-[720px] px-8">
                  <Banner tone="blocked" title="This document changed while you were typing">
                    <span className="flex flex-wrap items-center gap-2">
                      Your last edits aren&apos;t saved yet.
                      <Button variant="secondary" onClick={() => editor.current?.takeTheirs(c.seq, c.body)}>Use the latest version</Button>
                      <Button variant="primary" onClick={() => void editor.current?.keepMine(c.seq)}>Keep mine</Button>
                    </span>
                  </Banner>
                </div>
              )}
              {reviewing ? (
                baseBody === undefined || empty ? (
                  <div aria-busy="true" className="mx-auto max-w-[720px] p-8"><Skeleton className="h-[320px]" /></div>
                ) : (
                  <DocReview
                    key={`${base}-${c.seq}`}
                    base={baseBody}
                    head={c.body}
                    persona={persona}
                    busy={readOnly || reviewed.isPending || actions.save.isPending}
                    error={reviewed.error || actions.save.error ? errorText(reviewed.error ?? actions.save.error) : undefined}
                    onAcceptAll={() => reviewed.mutate(c.seq)}
                    onResolve={(body, note) => actions.save.mutate({ base_seq: c.seq, body, note })}
                  />
                )
              ) : editable ? (
                <DocEditor
                  ref={editor}
                  body={c.body}
                  seq={c.seq}
                  locked={live || readOnly}
                  persona={persona}
                  highlights={highlights}
                  onSave={async (body, baseSeq) => {
                    const v = await actions.save.mutateAsync({ base_seq: baseSeq, body, note: "Edited" });
                    saved.current = { seq: v.seq, body };
                    return v.seq;
                  }}
                  onState={(s, message) => setSaveState({ s, message })}
                  onAsk={(instruction, after) => void mention(instruction, after)}
                  onPick={(p, message) => void pick(p, message)}
                />
              ) : (
                <div className="flex min-h-full flex-col bg-surface-sunken">
                  <p className="mx-auto mt-3 max-w-[720px] px-8 text-xs text-fg-tertiary">
                    This document has HTML or footnotes the editor can&apos;t keep, so it opens as Markdown. Edit it under Edit.
                  </p>
                  <CanvasView chatId={chatId} canvasId={id} live={live} readOnly={readOnly} agent={persona} page />
                </div>
              )}
            </div>
            {showRail && (
              <Rail
                inline={false}
                threads={open}
                replies={replies}
                resolved={resolved}
                target={target?.t ?? null}
                activeId={activeId}
                setActive={setActiveId}
                agent={persona}
                readOnly={readOnly || live}
                actions={actions}
                seq={target?.seq ?? c.seq}
                onComposerDone={() => setTarget(null)}
                numberOf={(x) => [...open, ...resolved].findIndex((y) => y.id === x.id) + 1}
              />
            )}
          </main>
        </div>
      )}
    </div>
  );
}
