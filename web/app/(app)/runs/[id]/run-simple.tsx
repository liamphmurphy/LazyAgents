"use client";

// The run page's Simple mode (LAZYAGENTS-64, approved mockup in LAZYAGENTS-62):
// a five-field header, a "Now" line, a condensed per-turn feed and the outcome.
// Everything else lives in Details.

import Link from "next/link";
import { useEffect, useRef, useState, type ReactNode } from "react";
import type { Schemas } from "@/lib/api/client";
import { cx } from "@/lib/cx";
import { absoluteTime, duration, runCost } from "@/lib/format";
import { condenseFeed, delegateChild, finalMessage, nowLine, workHref, type FeedItem } from "@/lib/run-view";
import { Button, ButtonLink } from "@/components/ui/button";
import { EmptyState } from "@/components/ui/primitives";
import { AgentAvatar } from "@/components/domain/avatars";
import { Stat } from "@/components/domain/data";
import { StatusPill } from "@/components/domain/run-status";
import { SmallPill } from "@/components/domain/labels";

type RunDetail = Schemas["RunDetail"];
type Run = Schemas["Run"];

const clock = new Intl.DateTimeFormat(undefined, { hour: "2-digit", minute: "2-digit", hour12: false });

/** The run's cost, plus what its sub agents spent. Shared by both headers. */
export function CostValue({ d }: { d: RunDetail }) {
  const sub = d.child_runs.reduce((n, c) => n + c.cost_usd, 0);
  const subPriced = d.child_runs.every((c) => c.cost_source && c.cost_source !== "unknown");
  return (
    <>
      {runCost(d.run.cost_usd, d.run.cost_source)}
      {d.child_runs.length > 0 && <span className="ml-1.5 text-xs font-normal text-fg-tertiary">{subPriced ? "+" : "≥ +"}${sub.toFixed(2)} sub</span>}
    </>
  );
}

/** What the run works on: the ticket, the plan or the chat. */
export function WorkLink({ run }: { run: Run }) {
  const href = workHref(run);
  if (run.purpose === "ticket" && run.ticket_key) {
    return <Link href={`/tickets/${encodeURIComponent(run.ticket_key)}`} className="text-accent hover:underline">on <span className="font-mono">{run.ticket_key}</span> · {run.work_title} ↗</Link>;
  }
  if (!href) return null;
  return <Link href={href} className="text-accent hover:underline">{run.purpose === "planning" ? "Planning" : "Chat"} · {run.work_title || "untitled"} ↗</Link>;
}

/** Header: agent, status pill (with wait reason), work link, elapsed and cost. */
export function SimpleHeader({ d }: { d: RunDetail }) {
  const run = d.run;
  return (
    <div className="flex flex-wrap items-center gap-x-8 gap-y-3 border-b border-border-subtle px-6 py-3.5">
      <div className="flex min-w-0 flex-1 items-center gap-3">
        <AgentAvatar name={run.agent.name} colorIndex={run.agent.color_index} size="md" label={false} />
        <div className="flex min-w-0 flex-col leading-tight">
          <span className="flex flex-wrap items-center gap-2 font-semibold">
            {run.agent.name}
            <StatusPill run={run} />
          </span>
          <span className="truncate text-xs text-fg-tertiary">
            <WorkLink run={run} />
            {run.parent_run_id && <> · <Link href={`/runs/${run.parent_run_id}`} className="text-accent hover:underline">Delegated by ↗</Link></>}
          </span>
        </div>
      </div>
      <Stat label="Elapsed" value={duration(run.started_at, run.ended_at)} />
      <Stat label="Cost" value={<CostValue d={d} />} />
    </div>
  );
}

const GATE_TEXT: Record<string, string> = { approval: "Requested approval", reply: "Asked for a reply", mockup: "Submitted a mockup for review" };
const GATE_WAIT: Record<string, string> = { approval: "approval", reply: "human_reply", mockup: "mockup_review" };
const STEP_MARK: Record<string, string> = { done: "✓", failed: "✕", needs_human: "!" };
const STEP_TEXT: Record<string, string> = { done: "done", failed: "failed", needs_human: "needs a person" };

function Mark({ children, tone }: { children: ReactNode; tone?: "error" | "gate" | "done" }) {
  return (
    <span
      aria-hidden
      className={cx(
        "flex h-5 min-w-5 flex-none items-center justify-center rounded px-1 font-mono text-[10.5px] font-semibold",
        tone === "error" ? "bg-pill-blocked-bg text-pill-blocked-fg" : tone === "gate" ? "bg-pill-human-bg text-pill-human-fg" : tone === "done" ? "text-state-done" : "bg-surface-sunken text-fg-secondary",
      )}
    >
      {children}
    </span>
  );
}

function FeedRow({ item, d, waitingGate, onOpenTurn }: { item: FeedItem; d: RunDetail; waitingGate: boolean; onOpenTurn: (turn: number) => void }) {
  const run = d.run;
  const time = (
    <time dateTime={item.at} title={absoluteTime(item.at)} className="w-11 flex-none pt-0.5 font-mono text-[11.5px] text-fg-tertiary">
      {clock.format(new Date(item.at))}
    </time>
  );
  const row = "flex w-full items-start gap-3 border-b border-border-subtle px-5 py-2.5 text-left text-[13px] max-sm:px-3";
  switch (item.kind) {
    case "turn":
      return (
        <li>
          <button type="button" onClick={() => onOpenTurn(item.turn)} aria-label={`Turn ${item.turn}: ${item.text}. ${item.tools} tool calls. Open in Details`} className={cx(row, "hover:bg-hover")}>
            {time}
            <Mark>T{item.turn}</Mark>
            <span className="min-w-0 flex-1 truncate text-fg-2"><span className="font-medium text-fg">Turn {item.turn}</span>&ensp;{item.text}</span>
            {item.tools > 0 && <span className="flex-none rounded-full border border-border px-2 font-mono text-[11px] text-fg-secondary">{item.tools} tool call{item.tools === 1 ? "" : "s"}</span>}
          </button>
        </li>
      );
    case "step": {
      const text = (run.plan.find((s) => s.position === item.step) ?? run.plan[item.step - 1])?.text;
      return (
        <li className={row}>
          {time}
          <Mark tone={item.state === "done" ? "done" : item.state === "failed" ? "error" : "gate"}>{STEP_MARK[item.state]}</Mark>
          <span className="min-w-0 flex-1 text-fg-2">Plan step {item.step} {STEP_TEXT[item.state]}{text && <span className="text-fg-secondary"> · {text}</span>}</span>
        </li>
      );
    }
    case "delegation": {
      const child = delegateChild(item.call, d.child_runs, d.events);
      return (
        <li className={row}>
          {time}
          <Mark>↳</Mark>
          <span className="min-w-0 flex-1 truncate text-fg-2">Delegated to <span className="font-medium text-fg">{child?.agent.name ?? item.agent}</span>{item.task && <span className="text-fg-secondary"> · {item.task}</span>}</span>
          {child && <StatusPill run={child} />}
          {child && <Link href={`/runs/${child.id}`} className="flex-none text-xs text-accent hover:underline">Open run ↗</Link>}
        </li>
      );
    }
    case "error":
      return (
        <li>
          <button
            type="button"
            onClick={() => onOpenTurn(item.turn)}
            aria-label={`Error${item.turn ? ` in turn ${item.turn}` : " during setup"}: ${item.text}. Open in Details`}
            className={cx(row, "shadow-[inset_2px_0_0_var(--state-blocked)] hover:bg-hover")}
          >
            {time}
            <Mark tone="error">!</Mark>
            <span className="min-w-0 flex-1 whitespace-pre-wrap text-pill-blocked-fg">Error · {item.text}</span>
            <span className="flex-none text-xs text-fg-tertiary">{item.turn ? `in Turn ${item.turn}` : "in setup"}</span>
          </button>
        </li>
      );
    case "gate":
      return (
        <li className={cx(row, waitingGate && "shadow-[inset_2px_0_0_var(--state-human)]")}>
          {time}
          <Mark tone="gate">◆</Mark>
          <span className="min-w-0 flex-1 truncate text-fg-2">{GATE_TEXT[item.gate]}{item.text && <span className="text-fg-secondary"> · {item.text}</span>}</span>
          {waitingGate && <SmallPill tone="human">Waiting</SmallPill>}
        </li>
      );
  }
}

function Feed({ d, feed, onOpenTurn, onDetails }: { d: RunDetail; feed: FeedItem[]; onOpenTurn: (turn: number) => void; onDetails: () => void }) {
  const run = d.run;
  const [follow, setFollow] = useState(true);
  const scroller = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const el = scroller.current;
    if (follow && el) el.scrollTop = el.scrollHeight;
  }, [follow, feed.length]);
  // Only the latest gate of the kind the run waits on shows "Waiting".
  const waitingAt = run.state === "waiting" ? feed.findLastIndex((f) => f.kind === "gate" && GATE_WAIT[f.gate] === run.wait_reason) : -1;
  return (
    <section aria-label="Activity" className="flex min-h-0 flex-1 flex-col">
      <h2 className="flex items-center justify-between border-b border-border-subtle px-5 py-2.5 text-[13px] font-semibold">
        Activity
        <button type="button" onClick={onDetails} className="text-xs font-normal text-accent hover:underline">Full log in Details →</button>
      </h2>
      <div
        ref={scroller}
        role="log"
        aria-label="Activity feed"
        tabIndex={0}
        onScroll={(e) => {
          const el = e.currentTarget;
          setFollow(el.scrollHeight - el.scrollTop - el.clientHeight < 24);
        }}
        className="min-h-0 flex-1 overflow-y-auto max-lg:max-h-[70dvh]"
      >
        {feed.length ? (
          <ol aria-live={follow ? "polite" : "off"}>
            {feed.map((item, i) => (
              <FeedRow key={`${item.kind}:${item.at}:${i}`} item={item} d={d} waitingGate={i === waitingAt} onOpenTurn={onOpenTurn} />
            ))}
          </ol>
        ) : (
          <EmptyState icon="terminal" title="No activity yet">
            {run.state === "queued" ? "The run starts when a runner slot is free." : "Activity appears as the harness works."}
          </EmptyState>
        )}
      </div>
    </section>
  );
}

function Outcome({ d, feed, onDetails }: { d: RunDetail; feed: FeedItem[]; onDetails: () => void }) {
  const run = d.run;
  const final = finalMessage(d.events);
  const lastError = run.state === "failed" ? [...feed].reverse().find((f) => f.kind === "error") : undefined;
  const href = workHref(run);
  return (
    <section aria-label="Outcome" className="border-b border-border-subtle px-6 py-3">
      <div className={cx("flex flex-col gap-2 rounded-xl border bg-surface p-3.5", run.state === "failed" ? "border-pill-blocked-bg" : "border-border")}>
        <h2 className="text-xs font-medium text-fg-tertiary">Outcome</h2>
        {run.outcome ? <p className="whitespace-pre-line text-[13.5px] text-fg-2">{run.outcome}</p> : <p className="text-[13px] text-fg-secondary">The run ended without an outcome.</p>}
        {lastError?.kind === "error" && (
          <p className="text-[13px] text-pill-blocked-fg">
            <span className="font-medium">Last error</span> · {lastError.text}{lastError.turn ? ` in Turn ${lastError.turn}` : ""}
          </p>
        )}
        {final && final !== run.outcome && (
          <div>
            <h3 className="text-xs font-medium text-fg-tertiary">Final message</h3>
            <p className="max-h-40 overflow-y-auto whitespace-pre-wrap text-[13px] text-fg-secondary">{final}</p>
          </div>
        )}
        <div className="flex flex-wrap items-center gap-2">
          {run.pr_url && run.pr_number ? <ButtonLink variant="outline" href={run.pr_url}>PR #{run.pr_number} ↗</ButtonLink> : null}
          {href && <ButtonLink variant="outline" href={href}>{run.purpose === "ticket" ? run.ticket_key : run.purpose === "planning" ? "Review proposals" : "Open chat"}</ButtonLink>}
          {run.state === "failed" && <Button variant="outline" onClick={onDetails}>Open in Details</Button>}
        </div>
      </div>
    </section>
  );
}

/**
 * Simple mode. The parent keeps the header controls and the action box, so
 * they behave the same in every mode.
 */
export function SimpleView({ d, ended, onOpenTurn, onDetails }: { d: RunDetail; ended: boolean; onOpenTurn: (turn: number) => void; onDetails: () => void }) {
  const run = d.run;
  const feed = condenseFeed(d.events, (n) => {
    const t = d.turns.find((x) => x.seq === n);
    return t?.status === "running" ? "Working…" : t?.status === "pending" ? "Waiting for the runner" : "No message";
  });
  const now = nowLine(run, d.events);
  const [showActivity, setShowActivity] = useState(false);
  const turns = feed.filter((f) => f.kind === "turn").length;
  return (
    <div className="flex min-h-0 flex-1 flex-col">
      {now && (
        <p className="flex items-baseline gap-3 border-b border-border-subtle px-6 py-2.5 text-[13px]">
          <span className="flex-none text-[11px] font-semibold uppercase tracking-wide text-fg-tertiary">Now</span>
          <span role="status" className="min-w-0 flex-1 truncate text-fg-2">{now}</span>
        </p>
      )}
      {ended && <Outcome d={d} feed={feed} onDetails={onDetails} />}
      {ended && (
        <button
          type="button"
          aria-expanded={showActivity}
          onClick={() => setShowActivity(!showActivity)}
          className="flex items-center gap-2 border-b border-border-subtle px-6 py-2.5 text-left text-[13px] text-fg-secondary hover:bg-hover"
        >
          <span aria-hidden className="w-3 text-fg-tertiary">{showActivity ? "▾" : "▸"}</span>
          {turns} turn{turns === 1 ? "" : "s"} · {run.tool_calls} tool call{run.tool_calls === 1 ? "" : "s"} · {d.child_runs.length} sub run{d.child_runs.length === 1 ? "" : "s"}
          <span className="ml-auto text-xs text-accent">{showActivity ? "Hide activity" : "Show activity"}</span>
        </button>
      )}
      {(!ended || showActivity) && <Feed d={d} feed={feed} onOpenTurn={onOpenTurn} onDetails={onDetails} />}
    </div>
  );
}
