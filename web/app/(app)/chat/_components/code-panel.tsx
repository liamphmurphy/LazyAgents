"use client";

import { useMemo, useState } from "react";
import { useRouter } from "next/navigation";
import { switchPanel } from "@/lib/chat-panels";
import { useCodeReview, type CodeReview } from "@/lib/api/code-review-hooks";
import { useChat, useChatMessages, useQueueFollowup, useSendChatMessage } from "@/lib/api/hooks";
import type { Schemas } from "@/lib/api/client";
import { changeTotals, STATUS_BADGE } from "@/lib/code-diff";
import { baseName, commentKey, fileTree, repoName, reviewMessage, type LineComment } from "@/lib/code-review";
import { cx } from "@/lib/cx";
import { Button } from "@/components/ui/button";
import { Icon } from "@/components/ui/icon";
import { CodeChanges, Counts, Diff } from "./code-changes";

type Change = Schemas["ChatCodeChange"];
type Mode = "branch" | "turn";

/**
 * The Code dock: each repository's branch against its default branch, as the runner last read it
 * (live while a turn edits it, and still there once the workspace is cleaned up), or just what the
 * last reply changed. Comments on lines collect into one review, sent to the persona as a message.
 */
export function CodePanel({ chatId }: { chatId: string }) {
  const reviews = useCodeReview(chatId).data?.repositories ?? [];
  const chat = useChat(chatId).data;
  const messages = useChatMessages(chatId).data?.messages;
  const [repo, setRepo] = useState<string>();
  const [mode, setMode] = useState<Mode>("branch");
  const [path, setPath] = useState<string>();
  const [comments, setComments] = useState<Record<string, LineComment>>({});
  const [editing, setEditing] = useState<string>();
  const send = useSendChatMessage(chatId);
  const queue = useQueueFollowup(chatId);

  const review: CodeReview | undefined = reviews.find((r) => r.repository === repo) ?? reviews.find((r) => r.repository === chat?.repo_url) ?? reviews[0];
  const lastTurn = useMemo(() => {
    const m = messages?.findLast((x) => x.role === "assistant" && x.code_changes?.length);
    return (m?.code_changes ?? []).filter((c) => !c.repository || c.repository === review?.repository);
  }, [messages, review?.repository]);
  const branchFiles = review?.files;
  const files: Change[] = useMemo(() => (mode === "branch" ? (branchFiles ?? []) : lastTurn), [mode, branchFiles, lastTurn]);
  // A file that left the diff (reverted, or the mode switched) falls back to the first one.
  const file = files.find((f) => f.path === path) ?? files[0];
  const tree = useMemo(() => fileTree(files), [files]);
  const totals = changeTotals(files);
  const pending = Object.values(comments).filter((c) => c.body.trim());

  if (!review) {
    return (
      <div className="grid flex-1 place-items-center p-6 text-center text-13 text-fg-tertiary" data-testid="code-panel-empty">
        No code yet. Once the persona works in a repository, its branch shows here, updating as it edits.
      </div>
    );
  }

  const sendReview = () => {
    const body = reviewMessage(pending, review.branch);
    if (!body) return;
    const done = { onSuccess: () => setComments({}) };
    if (chat?.state === "running") queue.mutate(body, done);
    else send.mutate({ body }, done);
  };
  const sending = send.isPending || queue.isPending;

  return (
    <div className="flex min-h-0 flex-1 flex-col" data-testid="code-panel">
      <header className="flex flex-none flex-wrap items-center gap-2 border-b border-border-subtle px-3 py-2 text-13">
        {reviews.length > 1 ? (
          <select aria-label="Repository" value={review.repository} onChange={(e) => setRepo(e.target.value)} className="rounded-md border border-border bg-surface px-1.5 py-0.5 text-12">
            {reviews.map((r) => (
              <option key={r.repository} value={r.repository}>
                {repoName(r.repository)}
              </option>
            ))}
          </select>
        ) : (
          <span className="text-fg-tertiary">{repoName(review.repository)}</span>
        )}
        <span className="flex min-w-0 items-center gap-1 font-mono text-12 text-fg">
          <Icon name="branch" size={13} className="flex-none text-fg-tertiary" />
          <span className="truncate">{review.branch ?? "detached"}</span>
        </span>
        <span className="text-12 text-fg-tertiary">
          {review.commits.length} {review.commits.length === 1 ? "commit" : "commits"}
          {review.dirty && " · uncommitted changes"}
        </span>
        {review.live && (
          <span className="flex items-center gap-1 text-12 text-state-progress" data-testid="code-live">
            <span className="size-1.5 animate-pulse rounded-full bg-current" aria-hidden />
            live
          </span>
        )}
        <span className="ml-auto flex rounded-md bg-surface-sunken p-0.5 text-12" role="tablist" aria-label="Show">
          {(["branch", "turn"] as const).map((m) => (
            <button key={m} type="button" role="tab" aria-selected={mode === m} onClick={() => setMode(m)} className={cx("rounded px-2 py-0.5", mode === m ? "bg-surface text-fg shadow-sm" : "text-fg-tertiary hover:text-fg")}>
              {m === "branch" ? "Branch" : "Last reply"}
            </button>
          ))}
        </span>
      </header>
      <div className="flex min-h-0 flex-1">
        <nav aria-label="Changed files" className="w-56 flex-none overflow-y-auto border-r border-border-subtle py-1 text-12">
          <p className="flex items-center gap-1.5 px-3 py-1 text-fg-tertiary">
            {files.length} {files.length === 1 ? "file" : "files"}
            <Counts {...totals} />
          </p>
          {tree.map((d) => (
            <div key={d.dir}>
              {d.dir && <p className="truncate px-3 pt-1.5 font-mono text-11 text-fg-tertiary">{d.dir}/</p>}
              {d.files.map((f) => {
                const n = pending.filter((c) => c.path === f.path).length;
                return (
                  <button
                    key={f.path}
                    type="button"
                    aria-current={f.path === file?.path}
                    onClick={() => setPath(f.path)}
                    className={cx("flex w-full items-center gap-1.5 py-0.5 pr-2 text-left hover:bg-row-hover", d.dir ? "pl-5" : "pl-3", f.path === file?.path && "bg-selected")}
                  >
                    <span title={STATUS_BADGE[f.status].label} className="flex-none font-mono text-10 text-fg-tertiary">
                      {STATUS_BADGE[f.status].letter}
                    </span>
                    <span className="min-w-0 flex-1 truncate font-mono">{baseName(f.path)}</span>
                    {n > 0 && (
                      <span className="flex-none rounded-full bg-accent/15 px-1 text-10 text-accent" aria-label={`${n} comments`}>
                        {n}
                      </span>
                    )}
                  </button>
                );
              })}
            </div>
          ))}
        </nav>
        <section className="flex min-w-0 flex-1 flex-col" aria-label={file ? `Diff of ${file.path}` : "Diff"}>
          {file ? (
            <>
              <p className="flex flex-none items-center gap-2 border-b border-border-subtle px-3 py-1.5 font-mono text-12 text-fg">
                <span className="min-w-0 truncate">{file.status === "renamed" && file.old_path ? `${file.old_path} → ${file.path}` : file.path}</span>
                <span className="ml-auto">
                  <Counts additions={file.additions} deletions={file.deletions} />
                </span>
              </p>
              <div className="min-h-0 flex-1 overflow-auto">
                {file.binary ? (
                  <p className="px-3 py-2 text-12 text-fg-tertiary">Binary file changed</p>
                ) : !file.patch ? (
                  <p className="px-3 py-2 text-12 text-fg-tertiary">Diff not shown (too large).</p>
                ) : (
                  <Diff
                    key={`${mode}:${file.path}`}
                    tall
                    patch={file.patch}
                    path={file.path}
                    active
                    onLine={(row) => {
                      const line = row.newNo ?? row.oldNo ?? 0;
                      const c: LineComment = { path: file.path, line, side: row.newNo === undefined ? "old" : "new", code: row.text, body: "" };
                      const k = commentKey(c);
                      setComments((all) => (all[k] ? all : { ...all, [k]: c }));
                      setEditing(k);
                    }}
                    below={(row) => {
                      const k = commentKey({ path: file.path, line: row.newNo ?? row.oldNo ?? 0, side: row.newNo === undefined ? "old" : "new" });
                      const c = comments[k];
                      if (!c) return null;
                      return (
                        <LineCommentBox
                          comment={c}
                          editing={editing === k}
                          onEdit={() => setEditing(k)}
                          onSave={(body) => {
                            setComments((all) => ({ ...all, [k]: { ...c, body } }));
                            setEditing(undefined);
                          }}
                          onDelete={() => {
                            setComments((all) => Object.fromEntries(Object.entries(all).filter(([key]) => key !== k)));
                            setEditing(undefined);
                          }}
                        />
                      );
                    }}
                  />
                )}
                {file.truncated && <p className="px-3 py-2 text-12 text-fg-tertiary">Diff truncated.</p>}
              </div>
            </>
          ) : (
            <p className="grid flex-1 place-items-center text-13 text-fg-tertiary">{mode === "turn" ? "The last reply changed no files here." : "No changes on this branch yet."}</p>
          )}
        </section>
      </div>
      {pending.length > 0 && (
        <footer className="flex flex-none items-center gap-2 border-t border-border-subtle px-3 py-2 text-13" data-testid="code-review-bar">
          <span className="text-fg-secondary">
            {pending.length} {pending.length === 1 ? "comment" : "comments"} for the persona
          </span>
          <span className="flex-1" />
          <Button variant="outline" disabled={sending} onClick={() => setComments({})}>
            Discard
          </Button>
          <Button variant="primary" disabled={sending} onClick={sendReview} title={chat?.state === "running" ? "Queued after the current reply" : undefined}>
            Send review
          </Button>
        </footer>
      )}
    </div>
  );
}

function LineCommentBox({ comment, editing, onEdit, onSave, onDelete }: { comment: LineComment; editing: boolean; onEdit: () => void; onSave: (body: string) => void; onDelete: () => void }) {
  const [draft, setDraft] = useState(comment.body);
  if (!editing) {
    return (
      <div className="mx-3 my-1 whitespace-pre-wrap rounded-md border border-border bg-surface px-3 py-2 font-sans text-13 text-fg" data-testid="line-comment">
        {comment.body}
        <span className="ml-2 inline-flex gap-2 text-12">
          <button type="button" className="text-accent hover:underline" onClick={onEdit}>
            Edit
          </button>
          <button type="button" className="text-fg-tertiary hover:underline" onClick={onDelete}>
            Delete
          </button>
        </span>
      </div>
    );
  }
  return (
    <div className="mx-3 my-1 flex flex-col gap-2 rounded-md border border-accent/40 bg-surface p-2 font-sans" data-testid="line-comment-editor">
      <textarea
        autoFocus
        aria-label={`Comment on ${comment.path}:${comment.line}`}
        value={draft}
        onChange={(e) => setDraft(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) onSave(draft);
          if (e.key === "Escape") {
            if (comment.body) onSave(comment.body);
            else onDelete();
          }
        }}
        rows={3}
        placeholder="What should change here?"
        className="w-full resize-y rounded border border-border bg-surface-sunken px-2 py-1 text-13 text-fg"
      />
      <span className="flex justify-end gap-2">
        <Button variant="outline" onClick={() => (comment.body ? onSave(comment.body) : onDelete())}>
          Cancel
        </Button>
        <Button variant="primary" disabled={!draft.trim()} onClick={() => onSave(draft)}>
          Add to review
        </Button>
      </span>
    </div>
  );
}

/** A reply's changed files, with a way into the Code dock for the whole branch. */
export function ReplyCodeChanges({ changes }: { changes?: Change[] }) {
  const router = useRouter();
  if (!changes?.length) return null;
  const review = () => router.replace(`${window.location.pathname}?${switchPanel(new URLSearchParams(window.location.search), "code").toString()}`, { scroll: false });
  return <CodeChanges changes={changes} onReview={review} />;
}
