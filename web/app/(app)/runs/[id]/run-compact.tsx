"use client";

// The compact run detail (DESIGN.md §3 Responsive, frame "8 Run detail"): a header
// with back, title and a ··· menu, a step strip, then the log full screen. A waiting
// run shows its approval inline in the log and pins a reply composer to the bottom.
// The shell hides the tab bar here. The desktop page is run-log.tsx.

import Link from "next/link";
import { useRouter } from "next/navigation";
import { DropdownMenu } from "radix-ui";
import type { ReactNode, RefObject } from "react";
import type { Schemas } from "@/lib/api/client";
import type { useControlRunById } from "@/lib/api/hooks";
import { cx } from "@/lib/cx";
import { duration } from "@/lib/format";
import { stepProgress } from "@/lib/run-mobile";
import { PURPOSE_LABEL, workHref } from "@/lib/run-view";
import { RUN_ENDED, StatusPill } from "@/components/domain/run-status";
import { floatPanel } from "@/components/domain/pickers";
import { Icon } from "@/components/ui/icon";
import { Banner } from "@/components/ui/primitives";
import { ActionBox, ReplyComposer, confirmCancel, waitsForReply } from "./run-panels";

type RunDetail = Schemas["RunDetail"];
type Run = Schemas["Run"];

const item = "flex min-h-[44px] cursor-pointer select-none items-center rounded-md px-3 text-[14px] text-fg outline-none data-[highlighted]:bg-hover data-[disabled]:opacity-50";

const SEGMENT: Record<string, string> = {
  done: "bg-state-done",
  current: "bg-accent",
  needs_human: "bg-state-human",
  failed: "bg-state-blocked",
  pending: "bg-border-strong",
};
const STEP_STATE: Record<string, string> = { done: "done", current: "current", needs_human: "needs a person", failed: "failed", pending: "to do" };

/** One segment per plan step: done, current and remaining. */
export function StepStrip({ run }: { run: Run }) {
  const p = stepProgress(run);
  if (!p.total) return null;
  return (
    <div className="flex flex-none flex-col gap-1.5 border-b border-border-subtle px-4 py-2.5">
      <ol aria-label="Plan steps" className="flex gap-1">
        {p.steps.map((s) => (
          <li key={s.position} data-state={s.state} className={cx("h-1.5 min-w-0 flex-1 rounded-full", SEGMENT[s.state])}>
            <span className="sr-only">Step {s.position}: {s.text}, {STEP_STATE[s.state]}</span>
          </li>
        ))}
      </ol>
      <p className="flex items-baseline gap-2 text-[13px]">
        <span className="flex-none font-mono text-xs text-fg-tertiary">{p.done}/{p.total}</span>
        <span className="min-w-0 truncate text-fg-2">{p.current?.text ?? (p.done === p.total ? "All steps done" : "")}</span>
      </p>
    </div>
  );
}

function RunMenu({ run, lastLog, control }: { run: Run; lastLog?: number; control: ReturnType<typeof useControlRunById> }) {
  const router = useRouter();
  const live = !RUN_ENDED.includes(run.state);
  const work = run.purpose === "ticket" && run.ticket_key ? `/tickets/${encodeURIComponent(run.ticket_key)}` : workHref(run);
  return (
    <DropdownMenu.Root>
      <DropdownMenu.Trigger aria-label="Run actions" className="grid size-[44px] flex-none place-items-center rounded-md text-fg-secondary hover:bg-hover">
        <Icon name="dots" size={18} />
      </DropdownMenu.Trigger>
      <DropdownMenu.Portal>
        <DropdownMenu.Content align="end" sideOffset={4} className={`${floatPanel} z-50 w-[230px] p-1`}>
          {work && <DropdownMenu.Item className={item} onSelect={() => router.push(work)}>{run.purpose === "ticket" ? "Open ticket" : run.purpose === "planning" ? "Open plan" : "Open chat"}</DropdownMenu.Item>}
          {run.parent_run_id && <DropdownMenu.Item className={item} onSelect={() => router.push(`/runs/${run.parent_run_id}`)}>Open delegating run</DropdownMenu.Item>}
          {lastLog != null && <DropdownMenu.Item asChild className={item}><a href={`/api/v1/runs/${run.id}/turns/${lastLog}/log`}>Raw log</a></DropdownMenu.Item>}
          {live && run.mode === "managed" && (run.state === "paused" ? (
            <DropdownMenu.Item className={item} disabled={control.isPending} onSelect={() => control.mutate("resume")}>Resume</DropdownMenu.Item>
          ) : (
            <DropdownMenu.Item className={item} disabled={control.isPending || run.state === "queued"} onSelect={() => control.mutate("pause")}>Pause</DropdownMenu.Item>
          ))}
          {(run.state === "failed" || run.state === "cancelled") && run.ticket_key && !run.parent_run_id && (
            <DropdownMenu.Item className={item} disabled={control.isPending} onSelect={() => control.mutate("retry", { onSuccess: (r) => router.push(`/runs/${r.id}`) })}>Retry run</DropdownMenu.Item>
          )}
          {live && (
            <DropdownMenu.Item className={cx(item, "text-pill-blocked-fg")} disabled={control.isPending} onSelect={() => { if (confirmCancel(run.agent.name)) control.mutate("cancel"); }}>
              Cancel run
            </DropdownMenu.Item>
          )}
        </DropdownMenu.Content>
      </DropdownMenu.Portal>
    </DropdownMenu.Root>
  );
}

/** The compact run page. `log` is the run's events, rendered by run-log.tsx as on desktop. */
export function CompactRunDetail({ d, log, scroller, onScroll, control }: {
  d: RunDetail;
  log: ReactNode;
  scroller: RefObject<HTMLDivElement | null>;
  onScroll: (el: HTMLDivElement) => void;
  control: ReturnType<typeof useControlRunById>;
}) {
  const run = d.run;
  const live = !RUN_ENDED.includes(run.state);
  const lastLog = [...d.turns].reverse().find((t) => t.has_log)?.seq;
  const title = run.work_title || run.ticket_key || PURPOSE_LABEL[run.purpose];
  return (
    <div className="flex h-app flex-col">
      <header className="flex flex-none items-center gap-1 border-b border-border-subtle px-1 pt-safe">
        <Link href="/runs" aria-label="Back to runs" className="grid size-[44px] flex-none place-items-center rounded-md text-fg-secondary hover:bg-hover">
          <Icon name="chev" size={18} className="rotate-90" />
        </Link>
        <div className="flex min-w-0 flex-1 flex-col py-1.5 leading-tight">
          <h1 className="truncate text-[15px] font-semibold">{title}</h1>
          <span className="flex min-w-0 items-center gap-1.5 text-xs text-fg-tertiary">
            <span className="truncate">{run.agent.name}{run.ticket_key && <> · <span className="font-mono">{run.ticket_key}</span></>} · {duration(run.started_at, run.ended_at)}</span>
            <StatusPill run={run} />
          </span>
        </div>
        <RunMenu run={run} lastLog={lastLog} control={control} />
      </header>
      <StepStrip run={run} />
      {control.isError && (
        <div className="px-3 pt-2">
          <Banner tone="blocked" title={control.variables === "retry" ? "Couldn't retry the run" : "Couldn't change the run"}>{control.error.message}</Banner>
        </div>
      )}
      <div
        ref={scroller}
        role="log"
        aria-label="Run log"
        tabIndex={0}
        onScroll={(e) => onScroll(e.currentTarget)}
        className="min-h-0 min-w-0 flex-1 overflow-y-auto overflow-x-hidden [overflow-wrap:anywhere]"
      >
        {run.outcome && !live && (
          <p className="mx-3 mt-3 whitespace-pre-line rounded-lg border border-border bg-surface px-3.5 py-2.5 text-[13px] text-fg-2">{run.outcome}</p>
        )}
        {log}
        <ActionBox d={d} compact />
      </div>
      {waitsForReply(run) && <ReplyComposer run={run} lastSeq={d.events.at(-1)?.seq ?? 0} />}
    </div>
  );
}
