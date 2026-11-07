"use client";

import Link from "next/link";
import { useQueryClient } from "@tanstack/react-query";
import { useState, type ReactNode } from "react";
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
import { Banner, TextArea, TextInput } from "@/components/ui/primitives";

type Run = Schemas["Run"];
type RunDetail = Schemas["RunDetail"];
type P = Record<string, unknown>;

/** Cancel always asks first: it ends the run and discards its workspace session. */
export function confirmCancel(agent: string) {
  return window.confirm(`Cancel ${agent}'s run? It stops now and can't be resumed.`);
}

/** Chat-style prompt for a run waiting on `human_reply`; replies go out as a ticket comment mentioning the agent. */
function ReplyBox({ run, question, lastSeq }: { run: Run; question: string | null; lastSeq: number }) {
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
  return (
    <>
      <p className="text-xs font-medium text-pill-human-fg">{run.agent.name} is waiting for your reply</p>
      {question && <p className="max-h-40 overflow-y-auto whitespace-pre-wrap text-[13.5px] leading-relaxed text-fg-2">{question}</p>}
      {sent ? (
        <p role="status" className="flex items-center gap-3 text-[13px] text-fg-secondary">
          Sent. Resuming {run.agent.name}…
          <Button variant="outline" onClick={() => setSentAt(null)}>Reply again</Button>
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
            <span className="text-xs text-fg-tertiary">
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

function ApprovalBox({ run, approval }: { run: Run; approval: Schemas["Approval"] }) {
  const decide = useDecideRunApproval(run.id);
  const [note, setNote] = useState("");
  const [more, setMore] = useState(false);
  const preview = approval.action_preview ?? "";
  const long = preview.length > 400 || preview.split("\n").length > 6;
  const shown = more || !long ? preview : preview.split("\n").slice(0, 6).join("\n").slice(0, 400);
  const go = (decision: "approve" | "deny") => {
    if (!decide.isPending) decide.mutate({ id: approval.id, decision, note: note.trim() || undefined });
  };
  return (
    <>
      <p className="flex flex-wrap items-center gap-2 text-xs font-medium text-pill-human-fg">
        {run.agent.name} needs a decision <MonoChip>{approval.scope}</MonoChip>
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
      <label className="sr-only" htmlFor="approval-note">Note (optional)</label>
      <TextInput
        id="approval-note"
        value={note}
        placeholder="Note (optional)"
        onChange={(e) => setNote(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
            e.preventDefault();
            go("approve");
          }
        }}
      />
      {decide.isError && (
        <Banner tone="blocked" title="Couldn't record the decision">
          {decide.error.message}
        </Banner>
      )}
      <div className="flex items-center justify-end gap-3">
        <span className="text-xs text-fg-tertiary">
          <kbd className="font-mono">⌘↵</kbd> to approve
        </span>
        <Button variant="outline" disabled={decide.isPending} onClick={() => go("deny")}>Deny</Button>
        <Button variant="approve" disabled={decide.isPending} onClick={() => go("approve")}>
          {decide.isPending ? "Saving…" : "Approve"}
        </Button>
      </div>
    </>
  );
}

/** What the run waits for and how to unblock it, inline under the header. */
export function ActionBox({ d }: { d: RunDetail }) {
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
  const pending = d.approvals.find((a) => a.state === "pending");
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
  } else if (run.state === "waiting") {
    switch (run.wait_reason) {
      case "approval":
        body = pending ? <ApprovalBox run={run} approval={pending} /> : <p className="text-[13px] text-fg-2">Waiting for an approval decision.</p>;
        break;
      case "human_reply":
        if (run.purpose === "planning") {
          body = <p className="text-[13px] text-fg-2">{run.agent.name} is waiting for your reply. <Link className="text-accent hover:underline" href={workHref(run) ?? "/plans"}>Open the plan</Link></p>;
        } else if (run.purpose === "chat") {
          body = <p className="text-[13px] text-fg-2">{run.agent.name} is waiting for your reply. <Link className="text-accent hover:underline" href={workHref(run) ?? "/chat"}>Open chat</Link></p>;
        } else {
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
    <div className="border-b border-border-subtle px-6 py-3">
      <div aria-label="Needs attention" role="group" className="flex flex-col gap-2 rounded-xl border border-state-human/40 bg-surface p-3.5">
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
    <aside aria-label="Run summary" className="flex flex-col gap-3 border-t border-border-subtle px-4 py-4 lg:min-h-0 lg:w-[340px] lg:flex-none lg:overflow-y-auto lg:border-l lg:border-t-0">
      <PlanCard run={d.run} />
      <DelegatedTree d={d} grandchildren={grandchildren} />
      <UsageCard d={d} grandchildren={grandchildren} />
    </aside>
  );
}

