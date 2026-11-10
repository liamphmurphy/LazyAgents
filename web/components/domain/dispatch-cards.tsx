"use client";

import Link from "next/link";
import { useContext, useEffect, useMemo, useState } from "react";
import type { Schemas } from "@/lib/api/client";
import { useChatTicketRuns, useControlRunById, useDispatchPreview, useStartChatTicketRun, useTicket } from "@/lib/api/hooks";
import { parseDispatch, planLine, runEnded } from "@/lib/chat-dispatch";
import { minutesLabel } from "@/lib/chat-research";
import { cx } from "@/lib/cx";
import { repoShortName } from "@/lib/persona-ui";
import { Button, buttonClass } from "@/components/ui/button";
import { usePulse } from "@/components/ui/motion";
import { AgentAvatar } from "./avatars";
import { CodeFence } from "./code-fence";
import { StartsLine } from "./dispatch-dialog";
import { RecommendActions } from "./recommend-fence";
import { RUN_STATE, WAIT_LABEL } from "./run-status";
import { SmallPill } from "./labels";
import { SvgFenceMessage } from "./svg-fence";

type TicketRun = Schemas["ChatTicketRun"];

const CARD = "flex w-full max-w-[680px] flex-col gap-3 rounded-xl border border-border bg-surface p-3.5";
const TERM = "text-[12.5px] text-fg-tertiary";

/** The run's state, bumping when it changes so a glance at the tray catches it. */
function RunPill({ run }: { run: Pick<TicketRun, "state" | "wait_reason"> }) {
  const ref = usePulse<HTMLSpanElement>(run.state);
  const s = RUN_STATE[run.state] ?? { tone: "neutral" as const, label: run.state };
  const wait = run.state === "waiting" && run.wait_reason ? WAIT_LABEL[run.wait_reason] ?? run.wait_reason : "";
  return (
    <span ref={ref} className="inline-flex flex-none">
      <SmallPill tone={s.tone}>{s.label}{wait && ` · ${wait}`}</SmallPill>
    </span>
  );
}

/** Minutes since the run started (or how long it took), refreshed while it's live. */
function useElapsed(run: TicketRun) {
  const ended = runEnded(run.state);
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    if (ended) return;
    const t = setInterval(() => setNow(Date.now()), 15000);
    return () => clearInterval(t);
  }, [ended]);
  return minutesLabel((run.ended_at ? Date.parse(run.ended_at) : now) - Date.parse(run.started_at));
}

/** One ticket run a chat started: live in the tray, or as its card's summary once it ends. */
export function TicketRunCard({ run }: { run: TicketRun }) {
  const control = useControlRunById(run.run_id);
  const elapsed = useElapsed(run);
  const live = !runEnded(run.state);
  const step = planLine(run);
  const waiting = run.state === "waiting" || run.state === "paused";
  const pct = run.steps_total > 0 ? Math.round((run.steps_done / run.steps_total) * 100) : 0;
  return (
    <div data-testid="ticket-run" className={cx(CARD, "gap-2.5 transition-colors", waiting ? "border-state-human" : run.state === "failed" || run.state === "blocked" ? "border-state-blocked" : "")}>
      <div className="flex min-w-0 items-center gap-2">
        <Link href={`/tickets/${run.ticket}`} className="flex-none font-mono text-xs text-fg-tertiary hover:underline">{run.ticket}</Link>
        <span className="min-w-0 flex-1 truncate text-[13.5px] font-medium text-fg">{run.ticket_title}</span>
        <RunPill run={run} />
      </div>
      {live && (
        <div className="h-1.5 overflow-hidden rounded-full bg-surface-sunken">
          <div className={cx("h-full rounded-full transition-[width,background-color] duration-500", waiting ? "bg-state-human" : "bg-accent")} style={{ width: `${Math.max(pct, 4)}%` }} />
        </div>
      )}
      <p className="truncate text-xs text-fg-tertiary">
        @{run.agent} · {elapsed} · ${run.cost_usd.toFixed(2)}{step && ` · ${step}`}
      </p>
      {control.isError && <p role="alert" className="text-xs text-pill-blocked-fg">{control.error.message}</p>}
      <div className="flex flex-wrap items-center gap-2">
        {run.pr_url && <a href={run.pr_url} target="_blank" rel="noreferrer" className={buttonClass(live ? "outline" : "primary")}>Open PR</a>}
        <Link href={`/runs/${run.run_id}`} className={buttonClass("outline")}>Open run</Link>
        {live && <Button variant="ghost" disabled={control.isPending} onClick={() => control.mutate("cancel")}>Stop</Button>}
        {live && <span className="text-xs text-fg-tertiary">Safe to leave this chat.</span>}
      </div>
    </div>
  );
}

/** Ticket runs still going in this chat, above the composer so they never scroll away. */
export function ChatTicketRunTray({ chatId }: { chatId: string }) {
  const runs = useChatTicketRuns(chatId);
  const live = (runs.data?.runs ?? []).filter((r) => !runEnded(r.state));
  if (live.length === 0) return null;
  return (
    <div data-testid="ticket-run-tray" className="mb-2 flex flex-col gap-2">
      {live.map((r) => <TicketRunCard key={r.run_id} run={r} />)}
    </div>
  );
}

/** The Start card: what a run of this ticket would get, and nothing starts until Start. */
function StartCard({ ticketKey, chatId, messageId, onDismiss }: { ticketKey: string; chatId: string; messageId?: string; onDismiss: () => void }) {
  const preview = useDispatchPreview(ticketKey);
  const ticket = useTicket(ticketKey);
  const start = useStartChatTicketRun(chatId);
  if (preview.isPending) return <div aria-label={`Loading ${ticketKey}`} className={cx(CARD, "h-44 animate-pulse")} />;
  if (preview.isError) {
    return (
      <div data-testid="dispatch-card" className={CARD}>
        <h4 className="text-[14px] font-semibold text-fg">Start a run for {ticketKey}?</h4>
        <p role="alert" className="text-[13px] text-fg-secondary">{preview.error.message || `Couldn’t load ${ticketKey}.`}</p>
        <div><Link href={`/tickets/${ticketKey}`} className={buttonClass("outline")}>Open ticket</Link></div>
      </div>
    );
  }
  const p = preview.data;
  const refused = p.blockers.length > 0;
  return (
    <div data-testid="dispatch-card" className={CARD}>
      <div className="flex items-baseline justify-between gap-3">
        <h4 className="text-[14px] font-semibold text-fg">Start a run for {ticketKey}?</h4>
        <span className="flex-none font-mono text-xs text-fg-tertiary">{p.resolved.harness} · {p.resolved.model}</span>
      </div>
      {ticket.data && <p className="truncate text-[13px] text-fg">{ticket.data.title}</p>}
      <dl className="grid grid-cols-[92px_1fr] items-center gap-x-3 gap-y-1.5 text-[13px]">
        <dt className={TERM}>Agent</dt>
        <dd className="flex items-center gap-1.5"><AgentAvatar name={p.agent.name} colorIndex={p.agent.color_index} label={false} />@{p.agent.handle}</dd>
        {p.repo_url && (
          <>
            <dt className={TERM}>Repository</dt>
            <dd className="font-mono text-xs">{repoShortName(p.repo_url)}{p.base_branch && ` · ${p.base_branch}`}</dd>
          </>
        )}
        <dt className={TERM}>Starts</dt>
        <dd className="min-w-0"><StartsLine p={p} /></dd>
        {p.queued_behind.length > 0 && (
          <>
            <dt className={TERM}>Waits for</dt>
            <dd>{p.queued_behind.join(", ")}</dd>
          </>
        )}
        <dt className={TERM}>Progress</dt>
        <dd className="text-fg-secondary">Above the message box</dd>
      </dl>
      {refused && <p role="alert" className="text-xs text-pill-blocked-fg">{p.blockers.join(" ")}</p>}
      {start.isError && <p role="alert" className="text-xs text-pill-blocked-fg">{start.error.message || "Couldn’t start the run."}</p>}
      <div className="flex flex-wrap items-center gap-2">
        <Button variant="primary" disabled={refused || start.isPending || !messageId} onClick={() => start.mutate({ ticket: ticketKey, message_id: messageId })}>
          {start.isPending ? "Starting…" : "Start run"}
        </Button>
        <Link href={`/tickets/${ticketKey}`} className={buttonClass("outline")}>Open ticket</Link>
        <Button variant="ghost" onClick={onDismiss}>Not now</Button>
      </div>
    </div>
  );
}

/** ```dispatch: a persona's offer to start a ticket's run. Once started it points at the run. */
export function DispatchFence({ code, info }: { code: string; info?: string }) {
  const ticketKey = useMemo(() => parseDispatch(code), [code]);
  const messageId = useContext(SvgFenceMessage);
  const chatId = useContext(RecommendActions)?.chatId;
  const runs = useChatTicketRuns(ticketKey ? chatId : undefined);
  const [dismissed, setDismissed] = useState(false);
  // Outside a chat (a saved page) or while it's still streaming, it's just text.
  if (!ticketKey || !chatId) return <CodeFence code={code} info={info} />;
  if (runs.isPending) return <div aria-label={`Loading ${ticketKey}`} className={cx(CARD, "h-44 animate-pulse")} />;
  const run = runs.data?.runs.find((r) => r.message_id === messageId);
  if (run) {
    return runEnded(run.state) ? <TicketRunCard run={run} /> : (
      <p data-testid="dispatch-started" className="text-[12.5px] text-fg-tertiary">
        Started <Link className="text-accent hover:underline" href={`/tickets/${run.ticket}`}>{run.ticket}</Link> with @{run.agent}. Follow it above the message box.
      </p>
    );
  }
  if (dismissed) return <p data-testid="dispatch-dismissed" className="text-xs text-fg-tertiary">Not started.</p>;
  return <StartCard ticketKey={ticketKey} chatId={chatId} messageId={messageId} onDismiss={() => setDismissed(true)} />;
}
