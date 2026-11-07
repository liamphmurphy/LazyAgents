"use client";

import { TokenUsage } from "@/components/domain/token-usage";
import { eventTokenUsage, tokenBreakdown } from "@/lib/token-usage";
import { runCost } from "@/lib/format";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useQueryClient } from "@tanstack/react-query";
import { useEffect, useRef, useState, type ReactNode } from "react";
import type { Schemas } from "@/lib/api/client";
import { useComment, useControlRunById, useDelegatedRunLogs, useRunLog } from "@/lib/api/hooks";
import { cx } from "@/lib/cx";
import { latestQuestion, withMention } from "@/lib/run-question";
import { absoluteTime, compact, duration } from "@/lib/format";
import { Button, ButtonLink } from "@/components/ui/button";
import { Tabs, Toggle } from "@/components/ui/controls";
import { Icon } from "@/components/ui/icon";
import { Banner, EmptyState, Skeleton, TextArea, TextInput } from "@/components/ui/primitives";
import { AgentAvatar } from "@/components/domain/avatars";
import { Stat } from "@/components/domain/data";
import { Flag, SmallPill, type PillTone } from "@/components/domain/labels";
import { TopBar } from "@/components/shell/topbar";

type RunDetail = Schemas["RunDetail"];
type RunEvent = Schemas["RunEvent"];
type Turn = Schemas["RunTurn"];
type P = Record<string, unknown>;

const STATE: Record<string, { tone: PillTone; label: string }> = {
  queued: { tone: "neutral", label: "Queued" },
  starting: { tone: "neutral", label: "Starting" },
  running: { tone: "done", label: "Running" },
  waiting: { tone: "human", label: "Waiting" },
  paused: { tone: "human", label: "Paused" },
  blocked: { tone: "blocked", label: "Blocked" },
  completed: { tone: "review", label: "Completed" },
  failed: { tone: "blocked", label: "Failed" },
  cancelled: { tone: "neutral", label: "Cancelled" },
};
const WAIT: Record<string, string> = { approval: "approval", human_reply: "reply", pause: "paused", delegation: "sub agent", mockup_review: "mockup review" };
const ENDED = ["completed", "failed", "cancelled"];

const TRIGGER: Record<Turn["trigger"], string> = {
  start: "Started by dispatch",
  approval: "Resumed: a person decided",
  comment: "Resumed: a person replied",
  resume: "Resumed",
  delegation: "Resumed: a sub agent finished",
};

const clock = new Intl.DateTimeFormat(undefined, { hour: "2-digit", minute: "2-digit", second: "2-digit", hour12: false });
const str = (v: unknown) => (v == null ? "" : typeof v === "string" ? v : JSON.stringify(v));

/** "lazyagents.claim_ticket" for MCP tools, the tool name otherwise. */
function toolName(t: string) {
  const m = /^mcp__([^_]+(?:_[^_]+)*?)__(.+)$/.exec(t);
  return m ? `${m[1]}.${m[2]}` : t;
}

/** A compact argument summary: `(ticket: "PAY-214")`, or the command for shells. */
function argSummary(input: unknown) {
  if (!input || typeof input !== "object") return "";
  const o = input as P;
  if (typeof o.command === "string") return o.command;
  if (typeof o.file_path === "string") return o.file_path;
  const parts = Object.entries(o)
    .slice(0, 3)
    .map(([k, v]) => `${k}: ${typeof v === "string" ? JSON.stringify(v.length > 40 ? v.slice(0, 40) + "…" : v) : Array.isArray(v) ? `[${v.length}]` : JSON.stringify(v)}`);
  return `(${parts.join(", ")}${Object.keys(o).length > 3 ? ", …" : ""})`;
}

type Row =
  | { kind: "event"; e: RunEvent }
  | { kind: "tool"; call: RunEvent; result?: RunEvent };

/** Pairs each tool call with its result so they read as one card. */
function rowsOf(events: RunEvent[]): Row[] {
  const out: Row[] = [];
  const open = new Map<string, Extract<Row, { kind: "tool" }>>();
  for (const e of events) {
    const p = e.payload as P;
    if (e.type === "tool_call") {
      const row = { kind: "tool" as const, call: e };
      if (p.id) open.set(str(p.id), row);
      out.push(row);
    } else if (e.type === "tool_result" && p.id && open.has(str(p.id))) {
      open.get(str(p.id))!.result = e;
    } else {
      out.push({ kind: "event", e });
    }
  }
  return out;
}

function lifecycleText(p: P): ReactNode {
  switch (p.state) {
    case "queued":
      return "Dispatched; waiting for a runner slot";
    case "starting":
      return (
        <>
          Preparing the workspace on <code className="inline">{str(p.backend)}</code> ({str(p.runner)})
        </>
      );
    case "environment_snapshot":
      return <Link href="/settings/runner" className="underline underline-offset-2">Runner environment recorded</Link>;
    case "workspace_ready":
      return (
        <>
          Workspace ready on branch <code className="inline">{str(p.branch)}</code>
        </>
      );
    case "running":
      return p.trigger ? <>Running again: {str(p.message).split("\n")[0]}</> : "Running";
    case "turn_started":
      return (
        <>
          Turn {str(p.turn)} started{p.resume ? " · resumed the harness session" : ""}
        </>
      );
    case "harness_notice":
      return <>Harness notice: {str(p.message)}</>;
    case "harness_started":
      return (
        <>
          <code className="inline">{str(p.harness)}</code> started · session <span className="font-mono text-xs">{str(p.session_id).slice(0, 13)}</span>
        </>
      );
    case "turn_ended":
      return (
        <>
          Turn {str(p.turn)} ended · exit <span className="font-mono">{str(p.exit_code)}</span>
        </>
      );
    case "waiting":
    case "paused":
    case "completed":
    case "failed":
    case "cancelled":
      return (
        <>
          Run is now <SmallPill tone={STATE[str(p.state)]?.tone ?? "neutral"}>{STATE[str(p.state)]?.label ?? str(p.state)}</SmallPill>
          {p.reason ? <span className="ml-2 whitespace-pre-line text-fg-secondary">{str(p.reason)}</span> : null}
        </>
      );
  }
  return str(p.state);
}

function ToolCard({ call, result }: { call: RunEvent; result?: RunEvent }) {
  const p = call.payload as P;
  const r = (result?.payload ?? {}) as P;
  const denied = p.denied_by_harness === true;
  const failed = r.is_error === true;
  const out = str(r.output);
  return (
    <div className={cx("min-w-0 rounded-lg border bg-surface", denied || failed ? "border-pill-blocked-bg" : "border-border")}>
      <div className="flex items-center gap-2 px-3 py-2">
        <span className="font-mono text-[12.5px] font-medium">{toolName(str(p.tool))}</span>
        <span className="min-w-0 flex-1 truncate font-mono text-[12px] text-fg-tertiary">{argSummary(p.input)}</span>
        {denied ? (
          <Flag tone="blocked">denied by harness</Flag>
        ) : failed ? (
          <Flag tone="blocked">error{r.exit_code != null ? ` · exit ${str(r.exit_code)}` : ""}</Flag>
        ) : result ? (
          <Icon name="check" size={13} className="text-state-done" />
        ) : (
          <span className="text-[11px] text-fg-tertiary">running…</span>
        )}
      </div>
      {out && (
        <pre className="max-h-48 overflow-auto border-t border-border-subtle px-3 py-2 font-mono text-[11.5px] leading-relaxed text-fg-secondary">{out}</pre>
      )}
    </div>
  );
}

function EventRow({ row, source }: { row: Row; source?: Schemas["Run"] }) {
  const e = row.kind === "tool" ? row.call : row.e;
  const p = e.payload as P;
  const red = e.type === "error" || (row.kind === "tool" && (!!(row.call.payload as P).denied_by_harness || !!(row.result?.payload as P | undefined)?.is_error));
  const amber = e.type === "lifecycle" && (p.state === "waiting" || p.state === "paused");
  let body: ReactNode;
  switch (row.kind === "tool" ? "tool" : e.type) {
    case "tool":
      body = <ToolCard call={(row as Extract<Row, { kind: "tool" }>).call} result={(row as Extract<Row, { kind: "tool" }>).result} />;
      break;
    case "message":
      // Agent text from the harness, or a LazyAgents event logged by a tool
      // (plan posted, step moved, log_event).
      body =
        p.kind === "plan" ? (
          <span className="text-[13px] text-fg-2">Posted a plan: {Array.isArray(p.steps) ? p.steps.length : 0} steps</span>
        ) : p.kind === "step" ? (
          <span className="text-[13px] text-fg-2">
            Step {str(p.step)} → <span className="font-mono text-xs">{str(p.state)}</span>
          </span>
        ) : p.kind ? (
          <span className="font-mono text-[12px] text-fg-secondary">
            {str(p.kind)} {p.payload ? JSON.stringify(p.payload) : ""}
          </span>
        ) : (
          <p className="whitespace-pre-wrap text-[13.5px] leading-relaxed text-fg-2">{str(p.text)}</p>
        );
      break;
    case "usage":
      body = (
        <span className="font-mono text-[12px] text-fg-secondary">
          turn total · {compact(Number(p.input_tokens ?? 0))} in · {compact(Number(p.output_tokens ?? 0))} out
          {(() => { const t = tokenBreakdown(eventTokenUsage(p)); return t.hasBreakdown ? ` · ${compact(t.cached)} cached · ${compact(t.uncached)} uncached${t.unknown ? ` · ${compact(t.unknown)} cache unknown` : ""}` : " · cache breakdown unavailable"; })()}
          {` · ${runCost(Number(p.cost_usd ?? 0), p.cost_source === "harness" || p.cost_source === "computed" ? String(p.cost_source) : p.cost_known === true || (p.cost_known === undefined && Number(p.cost_usd ?? 0) > 0) ? "harness" : "unknown")}`}
        </span>
      );
      break;
    case "error":
      body = <span className="whitespace-pre-wrap text-[13px] text-pill-blocked-fg">{str(p.message)}</span>;
      break;
    case "tool_result":
      body = <span className="font-mono text-xs text-fg-tertiary">result for {toolName(str(p.tool))}</span>;
      break;
    default:
      body = <span className="text-[13px] text-fg-2">{lifecycleText(p)}</span>;
  }
  return (
    <li
      className={cx(
        "grid grid-cols-[72px_76px_1fr] items-start gap-3 border-b border-border-subtle px-5 py-2.5 max-sm:grid-cols-[60px_1fr] max-sm:px-3",
        red && "shadow-[inset_2px_0_0_var(--state-blocked)]",
        amber && "shadow-[inset_2px_0_0_var(--state-human)]",
      )}
    >
      <time dateTime={e.at} title={absoluteTime(e.at)} className="pt-0.5 font-mono text-[11.5px] text-fg-tertiary">
        {clock.format(new Date(e.at))}
      </time>
      <span className="pt-0.5 font-mono text-[11.5px] text-fg-tertiary max-sm:hidden">{row.kind === "tool" ? "tool" : e.type}</span>
      <div className="min-w-0 max-sm:col-start-2">
        {source && <Link href={`/runs/${source.id}`} className="mb-1 block text-xs font-medium text-accent hover:underline">{source.agent.name} · delegated agent</Link>}
        {body}
      </div>
    </li>
  );
}

function TurnsRail({ d }: { d: RunDetail }) {
  const run = d.run;
  const pending = d.approvals.find((a) => a.state === "pending");
  return (
    <aside aria-label="Turns" className="flex flex-col gap-2 border-r border-border-subtle px-3 py-4 lg:min-h-0 lg:flex-[1_1_240px] lg:max-w-[260px] lg:overflow-y-auto">
      <h2 className="px-2 text-xs font-medium text-fg-tertiary">Turns</h2>
      <ol className="flex flex-col gap-1">
        {d.turns.map((t) => {
          const u = (t.usage ?? {}) as P;
          return (
            <li key={t.seq} className={cx("flex flex-col gap-0.5 rounded-lg px-2.5 py-2", t.status === "running" && "bg-selected")}>
              <span className="flex items-center justify-between text-[13px]">
                <span className="font-medium">Turn {t.seq}</span>
                <span className="font-mono text-[11px] text-fg-tertiary" title={absoluteTime(t.started_at)}>
                  {clock.format(new Date(t.process_started_at ?? t.started_at)).slice(0, 5)}
                </span>
              </span>
              <span className="line-clamp-2 text-xs text-fg-secondary">{TRIGGER[t.trigger]}{t.trigger !== "start" && t.input_message ? `: ${t.input_message.split("\n")[0]}` : ""}</span>
              <span className="flex items-center gap-2 font-mono text-[11px] text-fg-tertiary">
                {t.status === "pending" ? (
                  "waiting for the runner"
                ) : t.status === "running" ? (
                  <span className="text-pill-done-fg">running</span>
                ) : (
                  <>
                    {t.process_started_at && duration(t.process_started_at, t.ended_at ?? undefined)}
                    {u.tool_calls != null && ` · ${str(u.tool_calls)} calls`}
                    {` · ${runCost(Number(u.cost_usd ?? 0), typeof u.cost_source === "string" ? u.cost_source : undefined)}`}
                    {t.exit_code ? ` · exit ${t.exit_code}` : ""}
                  </>
                )}
                {t.has_log && (
                  <a href={`/api/v1/runs/${run.id}/turns/${t.seq}/log`} className="ml-auto text-accent hover:underline" aria-label={`Raw log of turn ${t.seq}`}>
                    raw
                  </a>
                )}
              </span>
            </li>
          );
        })}
      </ol>
      {run.state === "waiting" && (
        <div className="mt-1 flex flex-col gap-1 rounded-lg border border-dashed border-state-human/40 px-3 py-2.5 text-xs">
          <span className="font-medium text-pill-human-fg">
            {run.wait_reason === "delegation" ? "Next turn starts when the delegated agent finishes" : run.wait_reason === "mockup_review" ? "Waiting for mockup review. The next turn starts when a person reviews it" : run.wait_reason === "approval" ? "Next turn starts when someone decides" : "Next turn starts when someone replies"}
          </span>
          <span className="text-fg-secondary">{run.wait_reason === "delegation" ? "Follow the delegated agent’s activity in the event timeline." : "Nothing is running. The workspace and session are kept."}</span>
          {run.wait_reason !== "delegation" && <Link href={`/tickets/${run.ticket_key}`} className="text-accent hover:underline">
            {run.wait_reason === "mockup_review" ? "Review the mockup on the ticket" : pending ? "Review the request" : `Reply with @${run.agent.handle}`}
          </Link>}
        </div>
      )}
      {run.state === "queued" && (
        <p className="px-2 text-xs text-fg-secondary">Queued: it starts when a runner slot is free.</p>
      )}
    </aside>
  );
}

const TAB_FILTER: Record<string, (r: Row) => boolean> = {
  all: () => true,
  messages: (r) => r.kind === "event" && r.e.type === "message" && !(r.e.payload as P).kind,
  tools: (r) => r.kind === "tool",
  errors: (r) =>
    (r.kind === "event" && r.e.type === "error") ||
    (r.kind === "tool" && (((r.call.payload as P).denied_by_harness as boolean) || ((r.result?.payload as P | undefined)?.is_error as boolean))),
};

function rowText(r: Row) {
  return JSON.stringify(r.kind === "tool" ? [r.call.payload, r.result?.payload] : r.e.payload).toLowerCase();
}

/** Chat-style prompt for a run waiting on `human_reply`; replies go out as a ticket comment mentioning the agent. */
function ReplyBox({ run, question, lastSeq }: { run: Schemas["Run"]; question: string | null; lastSeq: number }) {
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
    <div className="border-b border-border-subtle px-5 py-3">
      <div className="flex flex-col gap-2 rounded-xl border border-state-human/40 bg-surface p-3.5">
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
      </div>
    </div>
  );
}

export function RunLogScreen({ id }: { id: string }) {
  const { data: d, isError, error } = useRunLog(id);
  const children = useDelegatedRunLogs(d?.child_runs ?? []);
  const router = useRouter();
  const control = useControlRunById(id);
  const [tab, setTab] = useState("all");
  const [filter, setFilter] = useState("");
  const [follow, setFollow] = useState(true);
  const scroller = useRef<HTMLDivElement>(null);
  // Pair tools within each run before merging: sequence numbers and tool IDs
  // are scoped to their own run and can collide across delegated agents.
  const entries = [
    ...rowsOf(d?.events ?? []).map((row) => ({ row, source: undefined as Schemas["Run"] | undefined, runId: id })),
    ...children.flatMap((child) => child.data ? rowsOf(child.data.events).map((row) => ({ row, source: child.data!.run, runId: child.data!.run.id })) : []),
  ].sort((a, b) => {
    const ae = a.row.kind === "tool" ? a.row.call : a.row.e;
    const be = b.row.kind === "tool" ? b.row.call : b.row.e;
    return Date.parse(ae.at) - Date.parse(be.at) || a.runId.localeCompare(b.runId) || ae.seq - be.seq;
  });
  const rows = entries.map(({ row }) => row);
  const counts = Object.fromEntries(Object.entries(TAB_FILTER).map(([k, f]) => [k, rows.filter(f).length]));
  const shown = entries.filter(({ row, source }) => TAB_FILTER[tab](row) &&
    (!filter || `${source?.agent.name ?? ""} ${rowText(row)}`.toLowerCase().includes(filter.toLowerCase())));

  const loaded = !!d;
  useEffect(() => {
    const el = scroller.current;
    if (follow && el) el.scrollTop = el.scrollHeight;
  }, [follow, shown.length, tab, filter, loaded]);

  if (isError) {
    return (
      <>
        <TopBar crumbs={[{ label: "Runs" }, { label: "Not found" }]} />
        <div className="p-6">
          <Banner tone="blocked" title="Couldn't load this run">
            {error.message}
          </Banner>
        </div>
      </>
    );
  }
  if (!d) {
    return (
      <>
        <TopBar crumbs={[{ label: "Run" }]} />
        <div aria-busy="true" className="flex flex-col gap-3 p-6">
          <Skeleton className="h-10 w-full" />
          <Skeleton className="h-3 w-2/3" />
          <Skeleton className="h-3 w-1/2" />
        </div>
      </>
    );
  }
  const run = d.run;
  const st = STATE[run.state];
  const live = !ENDED.includes(run.state);
  const lastLog = [...d.turns].reverse().find((t) => t.has_log);
  return (
    <div className="flex flex-col lg:h-dvh">
      <TopBar
        crumbs={[{ label: run.planning_id ? "Plan" : run.ticket_key, href: run.planning_id ? `/plans?id=${run.planning_id}` : `/tickets/${run.ticket_key}`, mono: true }, { label: "Run log" }]}
        actions={
          <>
            <ButtonLink href={`/settings/runner?run_id=${run.id}`} variant="outline">Environment</ButtonLink>
            {lastLog && (
              <ButtonLink href={`/api/v1/runs/${run.id}/turns/${lastLog.seq}/log`} prefetch={false} variant="outline" icon="download">
                Raw log
              </ButtonLink>
            )}
            {live && run.mode === "managed" && (
              run.state === "paused" ? (
                <Button variant="outline" icon="play" disabled={control.isPending} onClick={() => control.mutate("resume")}>
                  Resume
                </Button>
              ) : (
                <Button variant="outline" icon="pause" disabled={control.isPending || run.state === "queued"} onClick={() => control.mutate("pause")}>
                  Pause
                </Button>
              )
            )}
            {(run.state === "failed" || run.state === "cancelled") && run.ticket_key && !run.parent_run_id && (
              <Button
                variant="approve"
                icon="refresh"
                disabled={control.isPending}
                onClick={() => control.mutate("retry", { onSuccess: (r) => router.push(`/runs/${r.id}`) })}
              >
                Retry run
              </Button>
            )}
            {live && (
              <Button variant="outline" icon="x" className="text-pill-blocked-fg" disabled={control.isPending} onClick={() => control.mutate("cancel")}>
                Cancel run
              </Button>
            )}
          </>
        }
      />
      <div className="flex flex-wrap items-center gap-x-8 gap-y-3 border-b border-border-subtle px-6 py-3.5">
        <div className="flex items-center gap-3">
          <AgentAvatar name={run.agent.name} colorIndex={run.agent.color_index} size="md" label={false} />
          <div className="flex flex-col leading-tight">
            <span className="flex items-center gap-2 font-semibold">
              {run.agent.name}
              <SmallPill tone={st.tone}>
                {st.label}
                {run.wait_reason && run.state === "waiting" ? ` · ${WAIT[run.wait_reason]}` : ""}
              </SmallPill>
            </span>
            <span className="text-xs text-fg-tertiary">
              {run.mode} · started {absoluteTime(run.started_at)}
              {run.dispatched_by && ` · dispatched by ${run.dispatched_by.name}`}
            </span>
          </div>
        </div>
        <Stat label="Harness" value={run.harness} />
        <Stat label="Model" value={run.model} />
        <Stat label="Session" value={run.session_id ? `${run.session_id.slice(0, 8)}…` : "–"} />
        <Stat label="Run time" value={duration(run.started_at, run.ended_at)} />
        <Stat label="Tool calls" value={run.tool_calls} />
        <Stat label="Tokens" value={compact(run.input_tokens + run.output_tokens)} />
        <Stat label="Spend" value={runCost(run.cost_usd, run.cost_source)} />
      </div>
      <TokenUsage usage={run} />
      {control.isError && (
        <div className="px-6 pt-3">
          <Banner tone="blocked" title={control.variables === "retry" ? "Couldn't retry the run" : "Couldn't change the run"}>
            {control.error.message}
          </Banner>
        </div>
      )}
      <div className="flex min-h-0 flex-1 flex-col lg:flex-row">
        <TurnsRail d={d} />
        <section aria-label="Events" className="flex min-h-0 min-w-0 flex-1 flex-col">
          <div className="flex flex-wrap items-center gap-3 border-b border-border-subtle px-5 py-2.5">
            <Tabs
              label="Event types"
              value={tab}
              onChange={setTab}
              items={[
                { key: "all", label: "All", count: counts.all },
                { key: "messages", label: "Messages", count: counts.messages },
                { key: "tools", label: "Tool calls", count: counts.tools },
                { key: "errors", label: "Errors", count: counts.errors },
              ]}
            />
            <TextInput aria-label="Filter events" placeholder="Filter events" value={filter} onChange={(e) => setFilter(e.target.value)} className="ml-auto w-56 max-sm:w-full" />
            <Toggle checked={follow} onChange={setFollow} label="Follow" />
          </div>
          {run.outcome && !live && (
            <div className="px-5 pt-3">
              <p className="whitespace-pre-line rounded-lg border border-border bg-surface px-3.5 py-2.5 text-[13px] text-fg-2">{run.outcome}</p>
            </div>
          )}
          {children.some((child) => child.isError) && (
            <div className="px-5 pt-3"><Banner tone="blocked" title="Couldn't load delegated agent logs">Some delegated activity is unavailable. Open the delegated run or try again.</Banner></div>
          )}
          {children.some((child) => child.isPending) && <p role="status" className="px-5 py-2 text-xs text-fg-secondary">Loading delegated agent logs…</p>}
          {run.state === "waiting" && run.wait_reason === "human_reply" && run.ticket_key && (
            <ReplyBox run={run} question={latestQuestion(d.events)} lastSeq={d.events.at(-1)?.seq ?? 0} />
          )}
          <div
            ref={scroller}
            role="log"
            aria-label="Run log"
            tabIndex={0}
            onScroll={(e) => {
              const el = e.currentTarget;
              setFollow(el.scrollHeight - el.scrollTop - el.clientHeight < 24);
            }}
            className="min-h-0 flex-1 overflow-y-auto max-lg:max-h-[70dvh]"
          >
          {shown.length ? (
            <ol aria-live={follow ? "polite" : "off"}>
              {shown.map(({ row, source, runId }) => (
                <EventRow key={`${runId}:${row.kind === "tool" ? row.call.seq : row.e.seq}`} row={row} source={source} />
              ))}
            </ol>
          ) : (
            <EmptyState icon="terminal" title={rows.length ? "No events match" : "No events yet"}>
              {rows.length ? "Try another tab or filter." : run.state === "queued" ? "The run starts when a runner slot is free." : "Events appear as the harness works."}
            </EmptyState>
          )}
          <p className="px-5 py-4 text-xs text-fg-tertiary">
            Normalized events. The raw harness stream of each turn is kept with the run ({run.mode === "managed" ? "raw links in Turns" : "unmanaged sessions have none"}).
          </p>
          </div>
        </section>
      </div>
      {!follow && (
        <Button
          variant="secondary"
          className="fixed bottom-6 right-6 z-20 shadow-md"
          onClick={() => {
            const el = scroller.current;
            if (el) el.scrollTop = el.scrollHeight;
            setFollow(true);
          }}
        >
          <span aria-hidden="true">↓</span> Jump to latest
        </Button>
      )}
    </div>
  );
}
