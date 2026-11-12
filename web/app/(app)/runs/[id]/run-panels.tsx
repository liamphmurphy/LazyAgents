"use client";

import Link from "next/link";
import { useQueryClient } from "@tanstack/react-query";
import { useEffect, useRef, useState, type ReactNode } from "react";
import type { Schemas } from "@/lib/api/client";
import { useComment, useControlRunById, useDecideRunApproval } from "@/lib/api/hooks";
import { cx } from "@/lib/cx";
import { compact, duration, runCost } from "@/lib/format";
import { cacheShare, subAgentTotals, usageByModel, workHref } from "@/lib/run-view";
import { latestQuestion, withMention } from "@/lib/run-question";
import { AgentAvatar } from "@/components/domain/avatars";
import { MonoChip } from "@/components/domain/labels";
import { RUN_ENDED, StatusPill } from "@/components/domain/run-status";
import { Button, ButtonLink } from "@/components/ui/button";
import { Banner, Kbd, TextArea } from "@/components/ui/primitives";
import { isTyping } from "@/components/shell/shortcuts";

/** The ActionBox carries this id; the Graph view focuses it from a pending gate. */
export const ACTION_BOX_ID = "run-action-box";

type Run = Schemas["Run"];
type RunDetail = Schemas["RunDetail"];
type P = Record<string, unknown>;

/** Cancel always asks first: it ends the run and discards its workspace session. */
export function confirmCancel(agent: string) {
  return window.confirm(`Cancel ${agent}'s run? It stops now and can't be resumed.`);
}

/** Sends a reply to a run waiting on `human_reply`: a ticket comment mentioning the agent. */
export function useReply(run: Run, lastSeq: number) {
  const qc = useQueryClient();
  const comment = useComment(run.ticket_key);
  const [body, setBody] = useState("");
  const [sentAt, setSentAt] = useState<number | null>(null);
  const sent = sentAt === lastSeq;
  const submit = () => {
    if (!body.trim() || comment.isPending) return;
    comment.mutate(
      { body_md: withMention(body, run.agent.handle) },
      {
        onSuccess: () => {
          setBody("");
          setSentAt(lastSeq);
        },
        onSettled: () => qc.invalidateQueries({ queryKey: ["runs", run.id] }),
      },
    );
  };
  return { comment, body, setBody, sent, again: () => setSentAt(null), submit };
}

/** Whether the run waits for a reply that the run page itself sends (ticket runs; plans and chats reply in place). */
export function waitsForReply(run: Run) {
  return run.state === "waiting" && run.wait_reason === "human_reply" && run.purpose !== "planning" && run.purpose !== "chat";
}

/** Chat-style prompt for a run waiting on `human_reply`; replies go out as a ticket comment mentioning the agent. */
function ReplyBox({ run, question, lastSeq }: { run: Run; question: string | null; lastSeq: number }) {
  const { comment, body, setBody, sent, again, submit } = useReply(run, lastSeq);
  return (
    <>
      <p className="text-xs font-medium text-pill-human-fg">{run.agent.name} is waiting for your reply</p>
      {question && <p className="max-h-40 overflow-y-auto whitespace-pre-wrap text-[13.5px] leading-relaxed text-fg-2">{question}</p>}
      {sent ? (
        <p role="status" className="flex items-center gap-3 text-[13px] text-fg-secondary">
          Sent. Resuming {run.agent.name}…
          <Button variant="outline" onClick={again}>Reply again</Button>
        </p>
      ) : (
        <>
          <label className="sr-only" htmlFor="run-reply">Reply to {run.agent.name}</label>
          <TextArea
            id="run-reply"
            value={body}
            rows={3}
            placeholder={`Reply to ${run.agent.name}`}
            onChange={(e) => setBody(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
                e.preventDefault();
                submit();
              }
            }}
          />
          {comment.isError && (
            <Banner tone="blocked" title="Couldn't send the reply">
              {comment.error.message}
            </Banner>
          )}
          <div className="flex items-center justify-end gap-3">
            <span className="text-xs text-fg-tertiary pointer-coarse:hidden">
              <kbd className="font-mono">⌘↵</kbd> to send
            </span>
            <Button variant="approve" disabled={!body.trim() || comment.isPending} onClick={submit}>
              {comment.isPending ? "Sending…" : "Send"}
            </Button>
          </div>
        </>
      )}
    </>
  );
}

/**
 * The pending approval: Approve, Approve all … for this run (when the run asks
 * for more than one), or Deny… with an optional reason. With `keys`, A, ⇧A
 * and D decide from the keyboard while focus isn't in a field.
 */
export function ApprovalBox({ run, approvals, keys, onDecided }: { run: Run; approvals: Schemas["Approval"][]; keys?: boolean; onDecided?: () => void }) {
  const decide = useDecideRunApproval(run.id);
  const [note, setNote] = useState("");
  const [denying, setDenying] = useState(false);
  const [more, setMore] = useState(false);
  const [busy, setBusy] = useState(false);
  const noteRef = useRef<HTMLTextAreaElement>(null);
  const approval = approvals[0];
  const preview = approval?.action_preview ?? "";
  const long = preview.length > 400 || preview.split("\n").length > 6;
  const shown = more || !long ? preview : preview.split("\n").slice(0, 6).join("\n").slice(0, 400);
  const pending = decide.isPending || busy;
  const go = (decision: "approve" | "deny") => {
    if (pending || !approval) return;
    decide.mutate({ id: approval.id, decision, note: decision === "deny" ? note.trim() || undefined : undefined }, {
      onSuccess: () => {
        setNote("");
        setDenying(false);
        onDecided?.();
      },
    });
  };
  /** No server call grants a scope for the rest of a run, so this approves every request the run has open now. */
  const approveAll = async () => {
    if (pending || approvals.length < 2) return;
    if (!window.confirm(`Approve all ${approvals.length} requests from ${run.agent.name} for this run?`)) return;
    setBusy(true);
    try {
      for (const a of approvals) await decide.mutateAsync({ id: a.id, decision: "approve" });
      onDecided?.();
    } catch {
      /* decide.error shows it. */
    } finally {
      setBusy(false);
    }
  };
  const startDeny = () => {
    setDenying(true);
    requestAnimationFrame(() => noteRef.current?.focus());
  };
  // Re-subscribed each render, so the keys always act on the current approval and note.
  useEffect(() => {
    if (!keys) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.metaKey || e.ctrlKey || e.altKey || isTyping(e.target) || document.querySelector('[role="dialog"]')) return;
      const k = e.key.toLowerCase();
      if (k === "a" && e.shiftKey) void approveAll();
      else if (k === "a") go("approve");
      else if (k === "d" && !e.shiftKey) startDeny();
      else return;
      e.preventDefault();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  });
  if (!approval) return null;
  return (
    <>
      <p className="flex flex-wrap items-center gap-2 text-xs font-medium text-pill-human-fg">
        {run.agent.name} needs a decision <MonoChip>{approval.scope}</MonoChip>
        {approvals.length > 1 && <span className="font-normal text-fg-tertiary">1 of {approvals.length}</span>}
      </p>
      <p className="text-[13.5px] font-medium text-fg">{approval.action_summary}</p>
      {approval.risk_summary && <p className="text-[13px] text-fg-secondary">{approval.risk_summary}</p>}
      {preview && (
        <div>
          <pre className="overflow-x-auto whitespace-pre-wrap rounded-md border border-border-subtle bg-surface-sunken px-3 py-2 font-mono text-[11.5px] leading-relaxed text-fg-2">{shown}{long && !more ? "…" : ""}</pre>
          {long && (
            <button type="button" aria-expanded={more} onClick={() => setMore(!more)} className="mt-1 text-xs text-accent hover:underline">
              {more ? "less" : "more"}
            </button>
          )}
        </div>
      )}
      {denying && (
        <form
          className="flex flex-col gap-2"
          onSubmit={(e) => {
            e.preventDefault();
            go("deny");
          }}
        >
          <TextArea
            ref={noteRef}
            rows={2}
            aria-label={`Reason for denying, sent to ${run.agent.name} (optional)`}
            placeholder="Why not? (optional, sent to the agent)"
            value={note}
            onChange={(e) => setNote(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter" && !e.shiftKey) {
                e.preventDefault();
                go("deny");
              } else if (e.key === "Escape") {
                setDenying(false);
              }
            }}
          />
          <div className="flex items-center justify-end gap-2">
            <Button variant="ghost" onClick={() => setDenying(false)}>Cancel</Button>
            <Button type="submit" variant="outline" disabled={pending}>Deny</Button>
          </div>
        </form>
      )}
      {decide.isError && (
        <Banner tone="blocked" title="Couldn't record the decision">
          {decide.error.message}
        </Banner>
      )}
      {!denying && (
        <div className="flex flex-wrap items-center justify-end gap-2">
          {keys && (
            <span className="mr-auto flex items-center gap-2 text-xs text-fg-tertiary pointer-coarse:hidden">
              <span><Kbd>A</Kbd> approve</span>
              {approvals.length > 1 && <span><Kbd>⇧A</Kbd> all</span>}
              <span><Kbd>D</Kbd> deny</span>
            </span>
          )}
          <Button variant="outline" disabled={pending} onClick={startDeny}>Deny…</Button>
          {approvals.length > 1 && (
            <Button variant="approve-outline" disabled={pending} onClick={approveAll}>Approve all {approvals.length} for this run</Button>
          )}
          <Button variant="approve" disabled={pending} onClick={() => go("approve")}>
            {pending ? "Saving…" : "Approve"}
          </Button>
        </div>
      )}
    </>
  );
}

/**
 * What the run waits for and how to unblock it, inline under the header. `compact` (the
 * compact run detail) shows it inside the log and leaves replies to the pinned ReplyComposer.
 */
export function ActionBox({ d, compact, inThread, keys, onDecided }: { d: RunDetail; compact?: boolean; inThread?: boolean; keys?: boolean; onDecided?: () => void }) {
  const run = d.run;
  const control = useControlRunById(run.id);
  const cancel = (
    <Button
      variant="outline"
      icon="x"
      className="text-pill-blocked-fg"
      disabled={control.isPending}
      onClick={() => { if (confirmCancel(run.agent.name)) control.mutate("cancel"); }}
    >
      Cancel run
    </Button>
  );
  const pending = d.approvals.filter((a) => a.state === "pending");
  const liveChildren = d.child_runs.filter((c) => !RUN_ENDED.includes(c.state));
  let body: ReactNode = null;
  if (run.state === "queued") {
    body = (
      <div className="flex items-center justify-between gap-3">
        <p className="text-[13px] text-fg-2">Waiting for a runner slot. It starts when one is free.</p>
        {cancel}
      </div>
    );
  } else if (run.state === "paused") {
    body = (
      <div className="flex items-center justify-between gap-3">
        <p className="text-[13px] text-fg-2"><span className="font-medium text-pill-human-fg">Paused.</span> Nothing runs until it is resumed.</p>
        <span className="flex gap-2">
          <Button variant="approve" icon="play" disabled={control.isPending} onClick={() => control.mutate("resume")}>Resume</Button>
          {cancel}
        </span>
      </div>
    );
  } else if (run.state === "waiting_for_quota") {
    const at = run.hold_until ? new Date(run.hold_until).toLocaleString() : "soon";
    body = (
      <div className="flex items-center justify-between gap-3">
        <p className="text-[13px] text-fg-2">
          <span className="font-medium text-fg">Out of provider usage.</span>{" "}
          {run.hold_reset_known ? `It resumes after the limit resets at ${at}.` : `The reset time is unknown, so it tries again at ${at}.`}
        </p>
        <span className="flex gap-2">
          <Button variant="approve" icon="play" disabled={control.isPending} onClick={() => control.mutate("resume")}>Try now</Button>
          {cancel}
        </span>
      </div>
    );
  } else if (run.state === "waiting") {
    switch (run.wait_reason) {
      case "approval":
        body = pending.length ? <ApprovalBox run={run} approvals={pending} keys={keys} onDecided={onDecided} /> : <p className="text-[13px] text-fg-2">Waiting for an approval decision.</p>;
        break;
      case "human_reply":
        if (run.purpose === "planning") {
          body = <p className="text-[13px] text-fg-2">{run.agent.name} is waiting for your reply. <Link className="text-accent hover:underline" href={workHref(run) ?? "/plans"}>Open the plan</Link></p>;
        } else if (run.purpose === "chat") {
          body = <p className="text-[13px] text-fg-2">{run.agent.name} is waiting for your reply. <Link className="text-accent hover:underline" href={workHref(run) ?? "/chat"}>Open chat</Link></p>;
        } else if (!compact && !inThread) {
          body = <ReplyBox run={run} question={latestQuestion(d.events)} lastSeq={d.events.at(-1)?.seq ?? 0} />;
        }
        break;
      case "mockup_review":
        body = (
          <div className="flex items-center justify-between gap-3">
            <p className="text-[13px] text-fg-2">Waiting for a mockup review. The next turn starts when a person reviews it.</p>
            <span className="flex gap-2">
              {run.ticket_key && <ButtonLink variant="primary" href={`/tickets/${encodeURIComponent(run.ticket_key)}`}>Review mockup</ButtonLink>}
              {cancel}
            </span>
          </div>
        );
        break;
      case "delegation":
        body = (
          <div className="flex flex-col gap-2">
            <p className="text-[13px] font-medium text-fg-2">
              Waiting for {liveChildren.length} sub agent{liveChildren.length === 1 ? "" : "s"}
            </p>
            <ul className="flex flex-col gap-1">
              {liveChildren.map((c) => (
                <li key={c.id} className="flex items-center gap-2 text-[13px]">
                  <AgentAvatar name={c.agent.name} colorIndex={c.agent.color_index} label={false} />
                  <Link href={`/runs/${c.id}`} className="font-medium text-accent hover:underline">{c.agent.name} ↗</Link>
                  <StatusPill run={c} />
                </li>
              ))}
            </ul>
            <div>{cancel}</div>
          </div>
        );
        break;
      default:
        body = (
          <div className="flex items-center justify-between gap-3">
            <p className="text-[13px] text-fg-2">Waiting for {run.agent.name}.</p>
            {cancel}
          </div>
        );
    }
  }
  if (!body) return null;
  return (
    <div className={compact ? "px-3 py-3" : inThread ? "py-2" : "border-b border-border-subtle px-6 py-3"}>
      <div id={ACTION_BOX_ID} tabIndex={-1} aria-label="Needs attention" role="group" className="flex flex-col gap-2 rounded-xl border border-state-human/40 bg-surface p-3.5">
        {body}
        {control.isError && (
          <Banner tone="blocked" title="Couldn't change the run">{control.error.message}</Banner>
        )}
      </div>
    </div>
  );
}

// ------------------------------------------------------------------ rail

function RailCard({ title, aside, children }: { title: string; aside?: ReactNode; children: ReactNode }) {
  return (
    <section aria-label={title} className="rounded-xl border border-border bg-surface p-3.5">
      <h2 className="mb-2 flex items-center justify-between text-xs font-medium text-fg-tertiary">
        {title}
        {aside && <span className="font-mono">{aside}</span>}
      </h2>
      {children}
    </section>
  );
}

const STEP_MARK: Record<string, string> = { done: "✓", current: "●", needs_human: "!", failed: "✕", pending: "○" };

function PlanCard({ run }: { run: Run }) {
  if (!run.plan.length) return null;
  const done = run.plan.filter((s) => s.state === "done").length;
  return (
    <RailCard title="Plan" aside={`${done} / ${run.plan.length}`}>
      <ol className="flex flex-col gap-1.5">
        {run.plan.map((s) => (
          <li key={s.position} className="flex gap-2 text-[13px]">
            <span aria-hidden className={cx("w-3 flex-none text-center", s.state === "done" && "text-state-done", s.state === "failed" && "text-pill-blocked-fg", s.state === "current" && "text-accent")}>{STEP_MARK[s.state]}</span>
            <span className={cx("min-w-0 flex-1", s.state === "done" ? "text-fg-secondary" : "text-fg-2")}>{s.text}</span>
            <span className="flex-none font-mono text-[11px] text-fg-tertiary">{s.state.replace("_", " ")}</span>
          </li>
        ))}
      </ol>
    </RailCard>
  );
}

function TreeNode({ run, label, depth, kids }: { run: Run; label?: string; depth: number; kids?: Run[] }) {
  return (
    <li>
      <div className={cx("flex items-start gap-2 py-1.5", depth > 0 && "ml-4 border-l border-border-subtle pl-3")}>
        <AgentAvatar name={run.agent.name} colorIndex={run.agent.color_index} label={false} />
        <div className="flex min-w-0 flex-col text-[13px] leading-tight">
          <span className="flex flex-wrap items-center gap-2">
            {label ? <span className="font-medium">{run.agent.name} <span className="font-normal text-fg-tertiary">({label})</span></span> : (
              <Link href={`/runs/${run.id}`} className="font-medium text-accent hover:underline">{run.agent.name} ↗</Link>
            )}
            <StatusPill run={run} />
          </span>
          <span className="font-mono text-[11px] text-fg-tertiary">
            {run.model} · {duration(run.started_at, run.ended_at)} · {runCost(run.cost_usd, run.cost_source)}
          </span>
        </div>
      </div>
      {kids && kids.length > 0 && (
        <ul>
          {kids.map((k) => <TreeNode key={k.id} run={k} depth={depth + 1} />)}
        </ul>
      )}
    </li>
  );
}

function DelegatedTree({ d, grandchildren }: { d: RunDetail; grandchildren: (Run[] | undefined)[] }) {
  if (!d.child_runs.length) return null;
  return (
    <RailCard title="Delegated runs" aside={`${d.child_runs.length}`}>
      <ul>
        <TreeNode run={d.run} label="this run" depth={0} />
        {d.child_runs.map((c, i) => (
          <TreeNode key={c.id} run={c} depth={1} kids={grandchildren[i]} />
        ))}
      </ul>
    </RailCard>
  );
}

function Row({ k, v, sub }: { k: ReactNode; v: ReactNode; sub?: ReactNode }) {
  return (
    <li className="flex items-baseline justify-between gap-3 text-[12.5px]">
      <span className="min-w-0 truncate text-fg-2">{k}</span>
      <span className="flex-none font-mono text-[11.5px] text-fg-secondary">{v}{sub && <span className="ml-2 text-fg-tertiary">{sub}</span>}</span>
    </li>
  );
}

function UsageCard({ d, grandchildren }: { d: RunDetail; grandchildren: (Run[] | undefined)[] }) {
  const run = d.run;
  const sub = subAgentTotals(d.child_runs);
  const seen = new Set<string>();
  const all = [run, ...d.child_runs, ...grandchildren.flatMap((g) => g ?? [])].filter((r) => !seen.has(r.id) && !!seen.add(r.id));
  const models = usageByModel(all);
  const share = cacheShare(run.input_tokens, run.cached_input_tokens);
  return (
    <RailCard title="Usage">
      <div className="flex flex-col gap-3">
        <div>
          <h3 className="mb-1 text-[11px] font-medium uppercase tracking-wide text-fg-tertiary">By turn</h3>
          <ul className="flex flex-col gap-1">
            {d.turns.map((t) => {
              const u = (t.usage ?? {}) as P;
              return (
                <Row
                  key={t.seq}
                  k={`Turn ${t.seq}`}
                  v={`${compact(Number(u.input_tokens ?? 0))} in · ${compact(Number(u.output_tokens ?? 0))} out`}
                  sub={runCost(Number(u.cost_usd ?? 0), typeof u.cost_source === "string" ? u.cost_source : undefined)}
                />
              );
            })}
            {d.child_runs.length > 0 && (
              <Row
                k={`Sub agents (${d.child_runs.length})`}
                v={`${compact(sub.input)} in · ${compact(sub.output)} out`}
                sub={sub.unpriced ? `≥ $${sub.cost.toFixed(2)}` : `$${sub.cost.toFixed(2)}`}
              />
            )}
            {d.turns.length === 0 && d.child_runs.length === 0 && <li className="text-xs text-fg-tertiary">No turns yet.</li>}
          </ul>
        </div>
        <div>
          <h3 className="mb-1 text-[11px] font-medium uppercase tracking-wide text-fg-tertiary">By model</h3>
          <ul className="flex flex-col gap-1">
            {models.map((m) => (
              <Row key={m.model} k={m.model} v={`${m.runs} run${m.runs === 1 ? "" : "s"}`} sub={m.unpriced ? `≥ $${m.cost.toFixed(2)}` : `$${m.cost.toFixed(2)}`} />
            ))}
          </ul>
        </div>
        <div>
          <h3 className="mb-1 text-[11px] font-medium uppercase tracking-wide text-fg-tertiary">Cache share</h3>
          {share === null ? (
            <p className="text-xs text-fg-tertiary">Cache breakdown unavailable.</p>
          ) : (
            <div className="flex items-center gap-2">
              <div role="img" aria-label={`${Math.round(share * 100)}% of input served from cache`} className="h-1.5 flex-1 overflow-hidden rounded-full bg-surface-sunken">
                <div className="h-full bg-accent" style={{ width: `${share * 100}%` }} />
              </div>
              <span className="font-mono text-[11.5px] text-fg-secondary">{Math.round(share * 100)}%</span>
            </div>
          )}
        </div>
      </div>
    </RailCard>
  );
}

export function RunRail({ d, grandchildren }: { d: RunDetail; grandchildren: (Run[] | undefined)[] }) {
  return (
    <aside aria-label="Run summary" className="flex flex-col gap-3 border-t border-border-subtle px-4 py-4 xl:min-h-0 xl:w-[340px] xl:flex-none xl:overflow-y-auto xl:border-l xl:border-t-0">
      <PlanCard run={d.run} />
      <DelegatedTree d={d} grandchildren={grandchildren} />
      <UsageCard d={d} grandchildren={grandchildren} />
    </aside>
  );
}


/** The compact run detail's reply composer, pinned under the log while the run waits for a reply. */
export function ReplyComposer({ run, lastSeq }: { run: Run; lastSeq: number }) {
  const { comment, body, setBody, sent, again, submit } = useReply(run, lastSeq);
  return (
    <form
      aria-label={`Reply to ${run.agent.name}`}
      onSubmit={(e) => {
        e.preventDefault();
        submit();
      }}
      className="flex flex-none flex-col gap-2 border-t border-border-subtle bg-bg px-3 pt-2.5 pb-[max(10px,env(safe-area-inset-bottom))]"
    >
      <p className="text-xs font-medium text-pill-human-fg">{run.agent.name} is waiting for your reply</p>
      {comment.isError && (
        <Banner tone="blocked" title="Couldn't send the reply">
          {comment.error.message}
        </Banner>
      )}
      {sent ? (
        <p role="status" className="flex items-center justify-between gap-3 text-[13px] text-fg-secondary">
          Sent. Resuming {run.agent.name}…
          <Button variant="outline" className="h-[44px]" onClick={again}>Reply again</Button>
        </p>
      ) : (
        <div className="flex items-end gap-2">
          <label className="sr-only" htmlFor="run-reply-compact">Reply to {run.agent.name}</label>
          <TextArea
            id="run-reply-compact"
            value={body}
            rows={1}
            placeholder={`Reply to ${run.agent.name}`}
            onChange={(e) => setBody(e.target.value)}
            className="max-h-32 min-h-[44px] flex-1 text-[16px]"
          />
          <Button type="submit" variant="approve" className="h-[44px]" disabled={!body.trim() || comment.isPending}>
            {comment.isPending ? "Sending…" : "Send"}
          </Button>
        </div>
      )}
    </form>
  );
}
