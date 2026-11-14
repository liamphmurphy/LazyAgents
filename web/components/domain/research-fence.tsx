"use client";

import Link from "next/link";
import { useContext, useEffect, useMemo, useState, type ReactNode } from "react";
import type { Schemas } from "@/lib/api/client";
import { useChatResearch, useControlResearch, useCreateWikiPage, useStartResearch } from "@/lib/api/hooks";
import {
  LENGTH_OPTIONS, MAX_QUESTIONS, MINUTE_RANGE, SOURCE_RANGE, SPEND_OPTIONS, TONE_OPTIONS, lengthOf, minutesLabel, parseResearchPlan, planReady, reportSections, researchActive, researchProgress, spendLabel, toneOf,
  type ResearchPlan, type Spend,
} from "@/lib/chat-research";
import { cx } from "@/lib/cx";
import { headingAnchors } from "@/lib/slug";
import { hostOf } from "@/lib/web-search";
import { Button } from "@/components/ui/button";
import { Icon } from "@/components/ui/icon";
import { Collapse } from "@/components/ui/motion";
import { CodeFence } from "./code-fence";
import { RecommendActions } from "./recommend-fence";
import { SvgFenceMessage } from "./svg-fence";

type Job = Schemas["ResearchJob"];
type Report = NonNullable<Job["report"]>;

const CARD = "flex w-full max-w-[680px] flex-col gap-3 rounded-xl border border-border bg-surface p-3.5";
const LABEL = "text-11 font-semibold uppercase tracking-wide text-fg-tertiary";
const INPUT = "w-full rounded-md border border-border-strong bg-surface px-2 py-1 text-13 text-fg focus:border-accent focus:outline-none";
const SPEND_KEY = "vesper.research-spend";
const LENGTH_KEY = "vesper.research-length";
const TONE_KEY = "vesper.research-tone";

function readPref(key: string): string | null {
  try {
    return localStorage.getItem(key);
  } catch {
    return null;
  }
}

function writePref(key: string, value: string) {
  try {
    localStorage.setItem(key, value);
  } catch {
    /* Not remembered. */
  }
}

/** A row of buttons picking one of a few options, with the picked one's hint below. */
function Segmented<T extends string>({ label, options, value, onChange }: { label: string; options: readonly { value: T; label: string; hint: string }[]; value: T; onChange: (v: T) => void }) {
  const picked = options.find((o) => o.value === value);
  return (
    <div className="flex flex-col gap-1">
      <div role="group" aria-label={label} className="flex flex-wrap items-center gap-1 rounded-lg bg-surface-sunken p-1">
        <span className="px-1.5 text-12 text-fg-tertiary">{label}</span>
        {options.map((o) => (
          <button key={o.value} type="button" title={o.hint} aria-pressed={value === o.value} onClick={() => onChange(o.value)} className={cx("flex-1 whitespace-nowrap rounded-md px-2 py-1 text-13", value === o.value ? "border border-accent bg-surface font-semibold text-accent" : "border border-transparent text-fg-secondary hover:text-fg")}>
            {o.label}
          </button>
        ))}
      </div>
      {picked && <p className="px-1 text-12 text-fg-tertiary">{picked.hint}</p>}
    </div>
  );
}

function readSpend(): Spend {
  try {
    const v = localStorage.getItem(SPEND_KEY);
    if (v === "unlimited") return v;
    const n = Number(v);
    return v && (SPEND_OPTIONS as readonly number[]).includes(n) ? n : 6;
  } catch {
    return 6;
  }
}

function NumberField({ label, value, range, onChange, suffix }: { label: string; value: number; range: { min: number; max: number }; onChange: (n: number) => void; suffix?: string }) {
  return (
    <label className="flex min-w-0 flex-1 flex-col gap-0.5 rounded-lg border border-border bg-surface-sunken px-2.5 py-1.5">
      <span className="text-12 text-fg-tertiary">{label}</span>
      <span className="flex items-baseline gap-1">
        <input type="number" aria-label={label} min={range.min} max={range.max} value={value} onChange={(e) => onChange(Number(e.target.value))} className="w-14 bg-transparent text-14 font-semibold text-fg focus:outline-none" />
        {suffix && <span className="text-12 text-fg-tertiary">{suffix}</span>}
      </span>
    </label>
  );
}

function PlanCard({ initial, messageId, chatId }: { initial: ResearchPlan; messageId?: string; chatId: string }) {
  const [plan, setPlan] = useState(initial);
  const [editing, setEditing] = useState(false);
  const [dismissed, setDismissed] = useState(false);
  const [sources, setSources] = useState(60);
  const [minutes, setMinutes] = useState(30);
  const [spend, setSpendState] = useState<Spend>(readSpend);
  // The persona's preset wins; otherwise what the person picked last time.
  const [length, setLength] = useState(() => initial.length ?? lengthOf(readPref(LENGTH_KEY)) ?? "standard");
  const [tone, setTone] = useState(() => initial.tone ?? toneOf(readPref(TONE_KEY)) ?? "report");
  const start = useStartResearch(chatId);
  const actions = useContext(RecommendActions);
  const setSpend = (s: Spend) => {
    setSpendState(s);
    try {
      localStorage.setItem(SPEND_KEY, String(s));
    } catch {
      /* Not remembered. */
    }
  };
  const setQuestion = (i: number, patch: Partial<ResearchPlan["questions"][number]>) => setPlan((p) => ({ ...p, questions: p.questions.map((q, j) => (j === i ? { ...q, ...patch } : q)) }));
  if (dismissed) return <p data-testid="research-dismissed" className="text-12 text-fg-tertiary">Research plan dismissed.</p>;
  const n = plan.questions.length;
  const ready = !!messageId && !!actions && planReady(plan, sources, minutes);
  return (
    <div data-testid="research-plan" className={CARD}>
      <div className="flex items-baseline justify-between gap-3">
        <h4 className="text-14 font-semibold text-fg">{plan.title}</h4>
        <span className="flex-none text-12 text-fg-tertiary">{n} {n === 1 ? "researcher" : "researchers"}</span>
      </div>
      <ul className="flex flex-col gap-1.5">
        {plan.questions.map((q, i) => (
          <li key={i} data-testid="research-question" className="flex items-start gap-2 rounded-lg border border-border bg-surface-sunken px-2.5 py-1.5">
            {editing ? (
              <div className="flex min-w-0 flex-1 flex-col gap-1">
                <input aria-label={`Label ${i + 1}`} value={q.label} onChange={(e) => setQuestion(i, { label: e.target.value })} className={INPUT} />
                <input aria-label={`Question ${i + 1}`} value={q.question} onChange={(e) => setQuestion(i, { question: e.target.value })} className={INPUT} />
              </div>
            ) : (
              <div className="min-w-0 flex-1">
                <p className="text-13 font-semibold text-fg">{q.label}</p>
                <p className="text-13 text-fg-secondary">{q.question}</p>
              </div>
            )}
            {editing && n > 1 && (
              <button type="button" aria-label={`Remove ${q.label}`} className="mt-1 text-fg-tertiary hover:text-fg" onClick={() => setPlan((p) => ({ ...p, questions: p.questions.filter((_, j) => j !== i) }))}>
                <Icon name="x" size={14} />
              </button>
            )}
          </li>
        ))}
      </ul>
      {editing && n < MAX_QUESTIONS && (
        <button type="button" className="self-start text-13 text-accent hover:underline" onClick={() => setPlan((p) => ({ ...p, questions: [...p.questions, { label: "", question: "" }] }))}>
          + Add a question
        </button>
      )}
      <div className="flex gap-2">
        <NumberField label="Sources, up to" value={sources} range={SOURCE_RANGE} onChange={setSources} />
        <NumberField label="Time, up to" value={minutes} range={MINUTE_RANGE} onChange={setMinutes} suffix="min" />
      </div>
      <div role="group" aria-label="Spend limit" className="flex items-center gap-1 rounded-lg bg-surface-sunken p-1">
        <span className="px-1.5 text-12 text-fg-tertiary">Spend</span>
        {[...SPEND_OPTIONS, "unlimited" as const].map((s) => (
          <button key={s} type="button" aria-pressed={spend === s} onClick={() => setSpend(s)} className={cx("flex-1 rounded-md px-2 py-1 text-13", spend === s ? "border border-accent bg-surface font-semibold text-accent" : "border border-transparent text-fg-secondary hover:text-fg")}>
            {s === "unlimited" ? "Unlimited" : `$${s}`}
          </button>
        ))}
      </div>
      <Segmented label="Length" options={LENGTH_OPTIONS} value={length} onChange={(v) => { setLength(v); writePref(LENGTH_KEY, v); }} />
      <Segmented label="Tone" options={TONE_OPTIONS} value={tone} onChange={(v) => { setTone(v); writePref(TONE_KEY, v); }} />
      {spend === "unlimited" && <p className="text-12 text-fg-secondary">Time and source limits still apply, and research pauses if a provider&apos;s usage limit is reached.</p>}
      {start.isError && <p role="alert" className="text-12 text-pill-blocked-fg">{start.error.message || "Couldn’t start the research."}</p>}
      <div className="flex flex-wrap items-center gap-2">
        <Button
          variant="primary"
          disabled={!ready || start.isPending}
          onClick={() => start.mutate({ message_id: messageId!, title: plan.title, questions: plan.questions, max_sources: sources, max_minutes: minutes, report_length: length, report_tone: tone, ...(spend === "unlimited" ? {} : { max_cost_usd: spend }) })}
        >
          {start.isPending ? "Starting…" : "Start research"}
        </Button>
        <Button variant="outline" onClick={() => setEditing((v) => !v)}>{editing ? "Done editing" : "Change it"}</Button>
        <Button variant="ghost" onClick={() => setDismissed(true)}>Cancel</Button>
      </div>
    </div>
  );
}

const TASK_NOTE: Record<string, string> = { queued: "Queued behind other work", done: "Done", failed: "Failed", stopped: "Stopped" };

function TaskMark({ state }: { state: string }) {
  if (state === "done") return <span className="grid size-[18px] flex-none place-items-center rounded-full text-state-done"><Icon name="check" size={13} /></span>;
  if (state === "running") return <span aria-hidden className="grid size-[18px] flex-none place-items-center rounded-full bg-accent-soft"><span className="size-1.5 animate-pulse rounded-full bg-accent" /></span>;
  return <span aria-hidden className="size-[18px] flex-none rounded-full border border-border-strong" />;
}

function Step({ state, children }: { state: "todo" | "now" | "done"; children: ReactNode }) {
  return (
    <div className="flex items-center gap-2.5">
      <TaskMark state={state === "done" ? "done" : state === "now" ? "running" : "queued"} />
      <span className={cx("text-13", state === "todo" ? "text-fg-secondary" : "font-medium text-fg")}>{children}</span>
    </div>
  );
}

function ProgressCard({ job, chatId }: { job: Job; chatId: string }) {
  const control = useControlResearch(chatId);
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const t = setInterval(() => setNow(Date.now()), 15000);
    return () => clearInterval(t);
  }, []);
  const reading = job.state === "running" || job.state === "finishing";
  const writing = job.state === "writing";
  const pct = reading ? researchProgress(job.tasks, job.sources_read, job.max_sources) : 1;
  return (
    <div data-testid="research-progress" className={CARD}>
      <div className="flex items-baseline justify-between gap-3">
        <h4 className="text-14 font-semibold text-fg">{job.title}</h4>
        <span className="flex-none text-12 text-fg-tertiary">{minutesLabel(now - Date.parse(job.created_at))} · {spendLabel(job.spent_usd, job.max_cost_usd)}</span>
      </div>
      <div className="h-1.5 overflow-hidden rounded-full bg-surface-sunken"><div className={cx("h-full rounded-full bg-accent transition-[width] duration-(--motion-progress)", reading && "live-sheen")} style={{ width: `${Math.round(pct * 100)}%` }} /></div>
      <ul className="flex flex-col gap-1.5">
        {job.tasks.map((t) => (
          <li key={t.label} data-testid="research-task" className={cx("flex items-center gap-2.5 rounded-lg border bg-surface-sunken px-2.5 py-1.5", t.state === "running" ? "border-accent" : "border-border")}>
            <TaskMark state={t.state} />
            <div className="min-w-0">
              <p className="text-13 font-semibold text-fg">{t.label}</p>
              <p className="truncate text-12 text-fg-tertiary">
                {t.state === "running" ? (t.reading ? `Reading ${hostOf(t.reading)}` : "Searching") : TASK_NOTE[t.state]}
                {t.sources_read > 0 && ` · ${t.sources_read} sources`}
                {t.findings > 0 && ` · ${t.findings} findings`}
              </p>
            </div>
          </li>
        ))}
      </ul>
      <div className="flex flex-col gap-1.5">
        <Step state={job.state === "writing" || job.state === "ready" ? "done" : "todo"}>Cross-check findings</Step>
        <Step state={writing ? "now" : "todo"}>Write the report</Step>
      </div>
      {job.state === "ready" && <p className="text-12 text-fg-secondary">Waiting for the chat to finish its current reply.</p>}
      {control.isError && <p role="alert" className="text-12 text-pill-blocked-fg">{control.error.message}</p>}
      {reading && (
        <div className="flex flex-wrap items-center gap-2">
          <Button variant="outline" disabled={control.isPending} onClick={() => control.mutate({ id: job.id, action: "stop" })}>Stop</Button>
          <Button variant="outline" disabled={control.isPending || job.state === "finishing"} onClick={() => control.mutate({ id: job.id, action: "finish" })}>Finish with this</Button>
          <span className="text-12 text-fg-tertiary">Safe to leave this chat.</span>
        </div>
      )}
    </div>
  );
}

function CiteLinks({ numbers, report }: { numbers?: number[]; report: Report }) {
  return (
    <>
      {(numbers ?? []).map((n) => {
        const src = report.sources.find((s) => s.n === n);
        if (!src) return null;
        return (
          <a key={n} href={src.url} target="_blank" rel="noopener noreferrer" title={src.title} data-testid="report-cite" className="mx-px inline-grid h-4 min-w-4 -translate-y-0.5 place-items-center rounded bg-accent-soft px-1 font-mono text-11 font-semibold text-accent hover:underline">
            {n}
          </a>
        );
      })}
    </>
  );
}

function ReportCard({ job, report, project }: { job: Job; report: Report; project: string }) {
  const save = useCreateWikiPage();
  const actions = useContext(RecommendActions);
  const [showSources, setShowSources] = useState(false);
  const sections = useMemo(() => reportSections(report.body_md, headingAnchors(report.body_md)), [report.body_md]);
  const href = `/artifacts/${job.artifact_id}`;
  const took = job.finished_at ? minutesLabel(Date.parse(job.finished_at) - Date.parse(job.created_at)) : "";
  const style = [LENGTH_OPTIONS.find((o) => o.value === job.report_length)?.label, TONE_OPTIONS.find((o) => o.value === job.report_tone)?.label].filter(Boolean).join(" ");
  const stats = [style, `${job.tasks.length} ${job.tasks.length === 1 ? "researcher" : "researchers"}`, `${job.sources_read} sources`, took, job.max_cost_usd !== undefined ? `$${job.spent_usd.toFixed(2)}` : ""].filter(Boolean).join(" · ");
  return (
    <div data-testid="research-report" className={cx(CARD, "gap-3.5")}>
      <div className="flex flex-wrap items-center gap-2">
        <span className="rounded-full bg-accent-soft px-2.5 py-0.5 text-12 font-semibold text-accent">Report</span>
        <span data-testid="report-stats" className="text-12 text-fg-tertiary">{stats}</span>
      </div>
      <h4 className="type-heading text-fg">{job.title}</h4>
      <div>
        <p className={LABEL}>Bottom line</p>
        <p className="mt-1 text-14 text-fg">{report.bottom_line}</p>
      </div>
      <div>
        <p className={LABEL}>Key findings</p>
        <ul className="mt-1 ml-4 list-disc space-y-1 text-14 text-fg-2">
          {report.key_findings.map((f, i) => <li key={i}>{f.text} <CiteLinks numbers={f.sources} report={report} /></li>)}
        </ul>
      </div>
      {report.disagreements.map((d, i) => (
        <div key={i} data-testid="report-disagreement" className="rounded-lg border border-border bg-surface-sunken px-3 py-2" style={{ borderLeft: "3px solid var(--agent-nova)" }}>
          <p className="text-13 font-semibold text-fg">{d.topic}</p>
          <p className="text-13 text-fg-secondary">{d.detail}</p>
        </div>
      ))}
      {job.stop_reason && <p className="text-12 text-fg-secondary">{job.stop_reason === "spend" ? "Finished early at the spend limit." : "Finished early from what had been found."}</p>}
      {sections.length > 0 && (
        <div>
          <p className={LABEL}>Sections</p>
          <div className="mt-1 flex flex-wrap gap-x-3 gap-y-1 text-13">
            {sections.map((s) => <Link key={s.anchor} href={`${href}#${s.anchor}`} className="text-accent hover:underline">{s.title}</Link>)}
          </div>
        </div>
      )}
      {save.isError && <p role="alert" className="text-12 text-pill-blocked-fg">{save.error.message || "Couldn’t save."}</p>}
      <div className="flex flex-wrap items-center gap-2">
        <Link href={href} className="inline-flex h-7 items-center rounded-md bg-accent px-2.5 text-12 font-medium !text-on-accent hover:opacity-90">Open report</Link>
        <Button variant="outline" disabled={save.isPending || save.isSuccess} onClick={() => save.mutate({ path: `Research/${job.title.replace(/[/\\]+/g, "-")}`, body_md: report.body_md, project_key: project })}>{save.isSuccess ? "Saved to wiki" : "Save to wiki"}</Button>
        {actions?.researchMore && job.artifact_id && <Button variant="outline" onClick={() => actions.researchMore!(job.artifact_id!, job.title)}>Research more</Button>}
      </div>
      {report.sources.length > 0 && (
        <div>
          <button type="button" aria-expanded={showSources} onClick={() => setShowSources((v) => !v)} className="flex items-center gap-1.5 text-13 font-semibold text-fg">
            {report.sources.length} sources
            <Icon name="chev" size={12} className={cx("text-fg-tertiary transition-transform", showSources && "rotate-180")} />
          </button>
          <Collapse open={showSources}>
            <ol className="mt-1.5 flex flex-col gap-0.5 text-13">
              {report.sources.map((s) => (
                <li key={s.n} data-testid="report-source" className="flex items-baseline gap-2">
                  <span className="flex-none font-mono text-11 text-fg-tertiary">{s.n}</span>
                  <a href={s.url} target="_blank" rel="noopener noreferrer" className="min-w-0 flex-1 truncate text-accent hover:underline">{s.title}</a>
                  <span className="flex-none text-12 text-fg-tertiary">{hostOf(s.url)}</span>
                </li>
              ))}
            </ol>
          </Collapse>
        </div>
      )}
    </div>
  );
}

/** A ```research-plan fence: the plan to approve, then its progress, then the finished report. */
export function ResearchFence({ code, info }: { code: string; info?: string }) {
  const plan = useMemo(() => parseResearchPlan(code), [code]);
  const messageId = useContext(SvgFenceMessage);
  const actions = useContext(RecommendActions);
  const chatId = actions?.chatId;
  const jobs = useChatResearch(plan ? chatId : undefined);
  if (!plan) return <CodeFence code={code} info={info} />;
  // Outside a chat (a saved page), the plan is just text.
  if (!chatId) return <CodeFence code={code} info={info} />;
  if (jobs.isPending) return <div aria-label="Loading research" className={cx(CARD, "h-64 animate-pulse")} />;
  const job = jobs.data?.jobs.find((j) => j.message_id === messageId);
  if (!job) return <PlanCard initial={plan} messageId={messageId} chatId={chatId} />;
  if (researchActive(job.state)) return <ProgressCard job={job} chatId={chatId} />;
  if (job.state === "done" && job.report) return <ReportCard job={job} report={job.report} project={actions.project ?? ""} />;
  return (
    <div data-testid="research-ended" className={CARD}>
      <h4 className="text-14 font-semibold text-fg">{job.title}</h4>
      <p className="text-13 text-fg-secondary">{job.state === "stopped" ? "Research stopped. No report was written." : (job.error ?? "This research didn’t finish.")}</p>
    </div>
  );
}
