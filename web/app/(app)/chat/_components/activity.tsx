"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import type { LinkTarget } from "@/lib/api/client";
import { useChatRunEvents, useResolvedLinks } from "@/lib/api/hooks";
import { activityOf, chatCalls, chatStatus, chatStatusOf, chatSteps, foldSteps, formatSpan, interimLine, NEXT_STEP_KEY, quietFor, turnClock, turnNotes, turnPulse, turnTrace, type Activity, type ChatStep, type IntentTag, type ToolCall, type TraceStep, type TurnPulse } from "@/lib/chat-ui";
import { useConnectorApprovals } from "@/lib/api/connector-hooks";
import { useApiSecretApprovals } from "@/lib/api/secret-hooks";
import { LinkTargets, WikiLink } from "@/components/domain/wikilink";
import { Button } from "@/components/ui/button";
import { Icon } from "@/components/ui/icon";
import { ActivityScene, useHeldScene } from "./activity-scene";
import { ConnectorApprovalCards } from "./connector-requests";
import { SecretApprovalCards } from "./secret-requests";
import { StepGlyph } from "./step-glyph";
import { PersonaAvatar, PersonaName, type PersonaLook } from "./persona";

const str = (v: unknown) => (v == null ? "" : typeof v === "string" ? v : JSON.stringify(v));

/** The page or ticket the newest call reads, linked once it resolves. Unknown refs stay plain text (no ghost "create" link). */
export function ActivityRef({ refText, label }: { refText: string; label: string }) {
  const resolved = useResolvedLinks([refText]);
  const targets = useMemo(() => {
    const m = new Map<string, LinkTarget>();
    for (const l of resolved.data ?? []) if (l.target.kind !== "unresolved" && l.target.href) m.set(l.ref, l.target);
    return m;
  }, [resolved.data]);
  if (!targets.has(refText)) return <span className="text-fg">{label}</span>;
  return (
    <LinkTargets.Provider value={targets}>
      <WikiLink refText={refText}>{label}</WikiLink>
    </LinkTargets.Provider>
  );
}

function LiveLine({ a }: { a: Activity }) {
  return (
    <p data-testid="live-activity" className="flex min-w-0 items-center gap-2 text-[12.5px] text-fg-secondary">
      <span aria-hidden className="size-3 flex-none animate-spin rounded-full border-2 border-border-strong border-t-accent" />
      <span className="min-w-0 truncate">
        {a.verb}
        {a.ref && a.label && (
          <>
            {" "}
            <ActivityRef refText={a.ref} label={a.label} />
          </>
        )}
        {a.text && ` ${a.text}`}…
      </span>
    </p>
  );
}

function CallList({ calls }: { calls: ToolCall[] }) {
  return (
    <details className="w-full text-[12.5px]">
      <summary className="cursor-pointer text-fg-tertiary">Tool calls ({calls.length})</summary>
      <ul className="mt-2 flex flex-col gap-1 font-mono text-xs">
        {calls.map((c) => (
          <li key={c.key} className="flex gap-2">
            <span className="text-fg">{c.tool}</span>
            <span className="min-w-0 flex-1 truncate text-fg-tertiary">{str(c.input)}</span>
            <span className="text-fg-tertiary">{c.done ? "done" : "running"}</span>
          </li>
        ))}
      </ul>
    </details>
  );
}

const TAG_LABEL: Record<IntentTag, string> = { approach: "changed approach", surprise: "surprise" };

function TagPill({ tag }: { tag: IntentTag }) {
  return (
    <span data-testid="intent-tag" className={`flex-none rounded-full px-1.5 text-[10.5px] ${tag === "surprise" ? "bg-[var(--state-review)]/15 text-fg-secondary" : "bg-accent/15 text-fg-secondary"}`}>
      {TAG_LABEL[tag]}
    </span>
  );
}

/** Where "View reasoning" on a chat step lands in the Activity tab. */
export const traceStepId = (key: string) => `trace-step-${key.replace(/[^\w-]/g, "_")}`;

/**
 * A turn as the steps the chat tab showed, each with the decisions the assistant
 * narrated, its thinking (folded) and every tool call. `focus` opens and scrolls to one step.
 */
function TurnTrace({ steps, live, focus }: { steps: TraceStep[]; live?: boolean; focus?: { key: string; at: number } }) {
  const root = useRef<HTMLOListElement>(null);
  useEffect(() => {
    if (!focus || !root.current) return;
    const el = root.current.querySelector<HTMLElement>(`#${traceStepId(focus.key)}`);
    if (!el) return;
    el.querySelector("details[data-thinking]")?.setAttribute("open", "");
    el.scrollIntoView({ block: "start", behavior: "smooth" });
  }, [focus]);
  if (steps.length === 0) return null;
  return (
    <ol ref={root} data-testid="turn-trace" className="flex w-full flex-col gap-2">
      {steps.map((s) => {
        // A finished turn's last step was still "active" when it ended.
        const state = !live && s.state === "active" ? "done" : s.state;
        return (
          <li key={s.key} id={live ? traceStepId(s.key) : undefined} data-testid="trace-step" data-step-state={state}
            className={`flex w-full scroll-mt-4 flex-col gap-1.5 rounded-lg border bg-surface px-3 py-2 ${focus?.key === s.key && live ? "border-accent" : "border-border"}`}>
            <div className="flex min-w-0 items-center gap-2 text-[13px]">
              <StepGlyph step={{ state, kind: s.kind }} />
              <span className={`min-w-0 truncate ${state === "active" ? "font-medium text-fg" : "text-fg-secondary"}`}>{s.title}</span>
              {s.tag && <TagPill tag={s.tag} />}
            </div>
            {s.intents.map((t, j) => (
              <p key={j} data-testid="trace-intent" className="rounded-md bg-accent/10 px-2.5 py-1.5 text-[12.5px] italic text-fg-secondary">{t}</p>
            ))}
            {s.thinking.length > 0 && (
              <details data-thinking className="text-[12.5px]">
                <summary className="cursor-pointer text-fg-tertiary">Thinking</summary>
                <div className="mt-1.5 flex flex-col gap-2 border-l-2 border-border pl-3">
                  {s.thinking.map((t, j) => (
                    <p key={j} data-testid="trace-thinking" className="whitespace-pre-wrap leading-relaxed text-fg-secondary">{t}</p>
                  ))}
                </div>
              </details>
            )}
            {s.calls.length > 0 && <CallList calls={s.calls} />}
          </li>
        );
      })}
    </ol>
  );
}

/**
 * While a turn runs: the newest tool call as a live line ("Reading [[ADR-012]]…",
 * ADR Decision 5), above the turn's steps with their intent, thinking and calls. Shown in the Activity tab only.
 */
export function RunActivity({ runId, since, focus }: { runId?: string; since?: string; focus?: { key: string; at: number } }) {
  const q = useChatRunEvents(runId);
  const steps = useMemo(() => turnTrace(q.data, since), [q.data, since]);
  const latest = steps.flatMap((s) => s.calls).at(-1);
  if (!latest && steps.length === 0) return null;
  return (
    <>
      {latest && <LiveLine a={activityOf(latest)} />}
      <TurnTrace steps={steps} live focus={focus} />
    </>
  );
}

/** A finished turn's steps, with their intent, thinking and tool calls, in the Activity tab. */
export function TurnToolCalls({ runId, since, until }: { runId?: string; since?: string; until?: string }) {
  const q = useChatRunEvents(runId, true, false);
  const steps = useMemo(() => turnTrace(q.data, since, until), [q.data, since, until]);
  return <TurnTrace steps={steps} />;
}

const STEP_BAR: Record<ChatStep["state"], string> = {
  planned: "bg-border",
  active: "bg-accent",
  done: "bg-[var(--state-done)]",
  failed: "bg-[var(--state-blocked)]",
};

/** Pulses the step under way, until the person presses Stop. */
const stepPulse = (s: ChatStep, stopping?: boolean) => (s.state === "active" && !stopping ? "motion-safe:animate-pulse" : "");

/** How long a step took, or has taken so far while it's under way; nothing under a second. */
function StepTimer({ step, now }: { step: ChatStep; now: number }) {
  if (step.startedAt === undefined) return null;
  const active = step.state === "active";
  const ms = (active ? now : (step.endedAt ?? step.startedAt)) - step.startedAt;
  if (ms < 1000) return null;
  return (
    <span data-testid="step-timer" className={`flex-none text-[10.5px] tabular-nums ${active ? "rounded-full bg-accent/15 px-1.5 text-fg-secondary" : "text-fg-tertiary"}`}>
      {formatSpan(ms)}
    </span>
  );
}

function StepRail({ steps, now, stopping, awaiting, onOpenStep }: { steps: ChatStep[]; now: number; stopping?: boolean; awaiting?: boolean; onOpenStep?: (key: string) => void }) {
  const { earlier, shown, later } = foldSteps(steps);
  return (
    <ol data-testid="chat-steps" className="flex flex-col gap-2 border-t border-border px-3.5 py-3 text-[12.5px]">
      {earlier > 0 && (
        <li className="flex items-center gap-2.5 text-fg-tertiary">
          <StepGlyph step={{ state: "done" }} />
          {earlier} earlier steps
        </li>
      )}
      {shown.map((s) => (
        <li key={s.key} data-step-state={s.state} className="flex min-w-0 items-start gap-2.5 motion-safe:animate-[scene-in_240ms_ease-out]">
          <StepGlyph step={s} stopping={stopping} />
          <div className="min-w-0 flex-1">
            <div className="flex min-w-0 items-center gap-2.5">
              <span className={`min-w-0 truncate ${s.state === "active" ? "font-medium text-fg" : s.state === "done" || s.state === "failed" ? "text-fg-secondary" : "text-fg-tertiary"}`}>{s.title}</span>
              {s.state === "failed" && <span className="sr-only">(failed)</span>}
              {awaiting && s.state === "active" && <span data-testid="step-awaiting" className="flex-none rounded-full bg-pill-human-bg px-2 text-[10.5px] text-pill-human-fg">Waiting for approval</span>}
              {s.added && <span className="flex-none rounded bg-[var(--state-progress)]/15 px-1.5 text-[10.5px] text-fg-secondary">added</span>}
              {s.tag && <TagPill tag={s.tag} />}
              <span className="ml-auto flex flex-none items-center gap-2.5">
                {onOpenStep && (s.reasoning || s.intent) && (
                  <button type="button" data-testid="view-reasoning" onClick={() => onOpenStep(s.key)} className="flex-none text-xs text-fg-tertiary hover:text-fg-secondary">
                    View reasoning ›
                  </button>
                )}
                <StepTimer step={s} now={now} />
              </span>
            </div>
            {s.detail && <p data-testid="chat-step-detail" className="truncate text-xs text-fg-tertiary">{s.detail}</p>}
            {s.intent && <p data-testid="chat-step-intent" className="line-clamp-2 text-xs italic text-fg-secondary">{s.intent}</p>}
          </div>
        </li>
      ))}
      {later > 0 && <li className="pl-[26px] text-fg-tertiary">+{later} more</li>}
    </ol>
  );
}

/**
 * The chat tab's card while a turn runs: "<Persona> is reading the code…" beside a small
 * scene for that kind of work, from the newest tool call at a high level only. The exact
 * calls stay in the Activity tab. Once a turn has two steps, or one with a detail to show, a rail below lists them; the active one says what it's on, by safe label only.
 * A step the assistant narrated a decision on shows that line, tagged when it changed approach or hit a surprise,
 * and links to its full reasoning in the Activity tab. It
 * collapses to a progress strip, and folds old steps so a long turn stays short.
 *
 * Under the headline, a pulse: how long the turn has run and how long since anything happened,
 * so a quiet stretch and a stuck one look different. Once the newest call has finished for a
 * few seconds the card says it's working out the next step instead of repeating that call; after
 * a long silence it says so, and offers to ask what it's doing (an aside) or to stop.
 */
export function ChatStatus({ chatId, disabled, runId, since, persona, stopping, onOpenStep, onAsk, onStop }: { chatId: string; disabled?: boolean; runId?: string; since?: string; persona: PersonaLook; stopping?: boolean; onOpenStep?: (key: string) => void; onAsk?: () => void; onStop?: () => void }) {
  const q = useChatRunEvents(runId);
  const now = useNow();
  const calls = useMemo(() => chatCalls(q.data, since), [q.data, since]);
  const clock = useMemo(() => turnClock(q.data, since), [q.data, since]);
  const pulse = turnPulse(calls, clock, now);
  const status = useMemo(() => chatStatus(calls, pulse.between), [calls, pulse.between]);
  const notes = useMemo(() => turnNotes(q.data, since), [q.data, since]);
  const { steps, planned } = useMemo(() => chatSteps(calls, notes, { between: pulse.between, clock }), [calls, notes, pulse.between, clock]);
  const [collapsed, setCollapsed] = useState(false);
  // Asking once per quiet stretch: the button comes back only after something happens.
  const [askedAt, setAskedAt] = useState<number>();
  const scene = useHeldScene(status.kind);
  const hasRail = steps.length > 1 || !!steps[0]?.detail || !!steps[0]?.intent;
  const done = steps.filter((s) => s.state === "done").length;
  const work = steps.filter((s) => s.key !== NEXT_STEP_KEY).length;
  // A tool call or API request waiting on the person is part of this turn, so it opens in this card.
  const connectorWaiting = useConnectorApprovals(chatId).data?.approvals.length ?? 0;
  const secretWaiting = useApiSecretApprovals(chatId).data?.approvals.length ?? 0;
  const awaiting = !stopping && connectorWaiting + secretWaiting > 0;
  const quiet = !stopping && !awaiting && pulse.quiet;
  const asked = askedAt !== undefined && askedAt >= (clock.last ?? 0);
  const newest = calls.at(-1);
  return (
    <div data-testid="chat-status-card" data-awaiting={awaiting || undefined} data-quiet={quiet || undefined}
      className={`@container w-full max-w-[min(720px,100%)] rounded-xl border bg-surface transition-[border-color,box-shadow] duration-300 ${awaiting ? "border-pill-human-fg/60 shadow-[0_0_0_3px_var(--pill-human-bg)]" : quiet ? "border-[var(--state-progress)]/50" : "border-border"}`}>
      <div className="flex h-[72px] items-center gap-3 px-3.5">
        <span className={`flex-none ${awaiting ? "" : "motion-safe:animate-pulse"}`}>
          <PersonaAvatar persona={persona} size={28} />
        </span>
        <div className="min-w-0 flex-1">
          <p role="status" aria-live="polite" className={`truncate text-[13.5px] ${awaiting ? "text-pill-human-fg" : "text-fg"}`} data-testid="chat-status">
            {stopping
              ? "Stopping…"
              : awaiting
                ? `${persona.name} needs your approval`
                : quiet
                  ? `${persona.name} has been quiet for ${quietFor(pulse.idle ?? 0)}`
                  : `${persona.name} is ${status.phrase}…`}
          </p>
          <p aria-hidden data-testid="chat-pulse" className="flex min-w-0 items-center gap-1.5 truncate text-xs text-fg-tertiary @max-[300px]:hidden">
            {awaiting ? (
              "Paused until you decide"
            ) : quiet ? (
              ["Still running", `last: ${newest ? chatStatusOf(newest).phrase : "thinking"}`, pulse.elapsed !== undefined && `${formatSpan(pulse.elapsed)} in all`].filter(Boolean).join(" · ")
            ) : (
              <PulseLine planned={planned ? `Step ${Math.min(done + 1, steps.length)} of ${steps.length}` : undefined} work={planned ? 0 : work} pulse={pulse} heard={clock.last !== undefined} />
            )}
          </p>
        </div>
        {awaiting ? (
          <span aria-hidden className="grid h-[38px] w-14 flex-none place-items-center rounded-md border border-pill-human-fg/50 text-pill-human-fg motion-safe:animate-pulse">
            <Icon name="shield" size={20} />
          </span>
        ) : (
          <ActivityScene current={scene.current} previous={scene.previous} stopping={stopping} />
        )}
      </div>
      {/* Always mounted so the panel can animate open and shut instead of popping in. */}
      <div className={`grid transition-[grid-template-rows] duration-300 ease-out motion-reduce:transition-none ${awaiting ? "grid-rows-[1fr]" : "grid-rows-[0fr]"}`}>
        <div className="overflow-hidden">
          <div role="group" aria-label="Waiting for your approval" className="flex flex-col gap-2 px-3.5 pb-3">
            <ConnectorApprovalCards chatId={chatId} disabled={disabled} />
            <SecretApprovalCards chatId={chatId} disabled={disabled} />
          </div>
        </div>
      </div>
      {quiet && (onAsk || onStop) && (
        <div data-testid="quiet-actions" className="flex flex-wrap gap-2 px-3.5 pb-3 pl-[54px]">
          {onAsk && !asked && (
            <Button variant="outline" icon="branch" disabled={disabled} onClick={() => { setAskedAt(Date.now()); onAsk(); }}>
              Ask what it&apos;s doing
            </Button>
          )}
          {onStop && (
            <Button variant="outline" icon="pause" onClick={onStop}>
              Stop
            </Button>
          )}
        </div>
      )}
      {hasRail && (
        <>
          {collapsed ? (
            <div aria-hidden className="flex gap-[3px] px-3.5">
              {steps.map((s) => (
                <i key={s.key} className={`h-1 flex-1 rounded-sm ${STEP_BAR[s.state]} ${stepPulse(s, stopping)}`} />
              ))}
            </div>
          ) : (
            <StepRail steps={steps} now={now} stopping={stopping} awaiting={awaiting} onOpenStep={onOpenStep} />
          )}
          <button type="button" aria-expanded={!collapsed} onClick={() => setCollapsed((c) => !c)} className="px-3.5 py-1.5 text-xs text-fg-tertiary hover:text-fg-secondary">
            {collapsed ? "Show steps" : "Hide steps"}
          </button>
        </>
      )}
    </div>
  );
}

/** "4m 12s · 23 steps · ● last activity 3s ago": the turn's time, its work so far, and a heartbeat. */
function PulseLine({ planned, work, pulse, heard }: { planned?: string; work: number; pulse: TurnPulse; heard: boolean }) {
  const parts = [planned, pulse.elapsed !== undefined && formatSpan(pulse.elapsed), work > 1 && `${work} steps`].filter(Boolean);
  return (
    <>
      <span className="truncate">{parts.join(" · ")}</span>
      {heard && pulse.idle !== undefined && (
        <>
          {parts.length > 0 && <span>·</span>}
          <span className="size-1.5 flex-none rounded-full bg-[var(--state-done)]" />
          <span className="truncate tabular-nums">last activity {formatSpan(pulse.idle)} ago</span>
        </>
      )}
    </>
  );
}

/** The time now, ticking every second while mounted: the card's timers count without new events. */
function useNow() {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const timer = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(timer);
  }, []);
  return now;
}

/**
 * The running turn's opening line ("Good question, let me check the runs."), shown as a
 * reply bubble while the assistant works. The prompt asks for one only on turns that
 * need real work; when the turn ends the server saves it as its own message ahead of the reply.
 */
export function InterimReply({ runId, since, persona }: { runId?: string; since?: string; persona: PersonaLook }) {
  const q = useChatRunEvents(runId);
  const line = useMemo(() => interimLine(q.data, since), [q.data, since]);
  if (!line) return null;
  return (
    <div className="flex flex-col items-start gap-1" data-testid="interim-reply">
      <span className="flex items-center gap-1.5 text-xs text-fg-tertiary">
        <PersonaAvatar persona={persona} size={22} />
        <PersonaName persona={persona} className="text-[13px]" />
      </span>
      <p className="max-w-[min(720px,100%)] whitespace-pre-wrap rounded-xl border border-border bg-surface px-3.5 py-2.5 text-[13.5px]">{line}</p>
    </div>
  );
}
