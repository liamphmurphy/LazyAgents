"use client";

import { useState } from "react";
import { useApproveCourse, useEditCourseOutline, type Course, type CourseOutline } from "@/lib/api/learning-hooks";
import { MINUTE_RANGE, SOURCE_RANGE } from "@/lib/chat-research";
import { BUILD_STEPS, buildStepIndex, estimatedRuns, moveLesson, type BuildState, type OutlineUnitOrder } from "@/lib/learning";
import { cx } from "@/lib/cx";
import { Button } from "@/components/ui/button";
import { Icon } from "@/components/ui/icon";
import { Banner, Card, Field, TextInput } from "@/components/ui/primitives";

export function BuildStepper({ state }: { state: BuildState }) {
  const at = buildStepIndex(state);
  return (
    <ol className="flex flex-wrap items-center gap-x-5 gap-y-2" aria-label="Build progress">
      {BUILD_STEPS.map((s, i) => {
        const done = at >= 0 && i < at;
        const current = i === at;
        return (
          <li key={s.state} aria-current={current ? "step" : undefined} className={cx("flex items-center gap-2 text-xs", current ? (state === "outline_ready" ? "font-semibold text-pill-human-fg" : "font-semibold text-fg") : done ? "text-fg-secondary" : "text-fg-tertiary")}>
            <span className={cx("grid size-[18px] place-items-center rounded-full border text-[10px]", done ? "border-fg-2 bg-fg-2 text-bg" : current ? (state === "outline_ready" ? "border-state-human" : "border-accent") : "border-border-strong")}>{done && "✓"}</span>
            {s.label}
          </li>
        );
      })}
    </ol>
  );
}

const orderOf = (o: CourseOutline): OutlineUnitOrder[] => o.units.map((u) => ({ title: u.title, lessons: u.lessons.map((l) => l.key) }));
const knownOf = (o: CourseOutline) => o.units.flatMap((u) => u.lessons.filter((l) => l.known).map((l) => l.key));

export function OutlineReview({ course }: { course: Course }) {
  const id = course.summary.id;
  const outline = course.outline!;
  const edit = useEditCourseOutline(id);
  const approve = useApproveCourse(id);
  const [sources, setSources] = useState(course.max_sources);
  const [minutes, setMinutes] = useState(course.max_minutes);
  const [spend, setSpend] = useState("");
  const send = (units: OutlineUnitOrder[], known: string[]) => edit.mutate({ units, known });
  const known = knownOf(outline);
  const number = new Map<string, string>();
  outline.units.forEach((u, i) => u.lessons.forEach((l, j) => number.set(l.key, `${i + 1}.${j + 1}`)));
  const limitsOk = sources >= SOURCE_RANGE.min && sources <= SOURCE_RANGE.max && minutes >= MINUTE_RANGE.min && minutes <= MINUTE_RANGE.max && (spend === "" || Number(spend) > 0);
  const nothingLeft = estimatedRuns(outline.units) === 0;
  const start = () => approve.mutate({ max_sources: sources, max_minutes: minutes, max_cost_usd: spend === "" ? undefined : Number(spend) });
  return (
    <div className="flex flex-col gap-5">
      <Banner title="Review this outline. Research starts when you approve it." />
      <div className="grid items-start gap-6 lg:grid-cols-[minmax(0,1fr)_320px]">
        <div className="flex min-w-0 flex-col gap-3">
          <div className="space-y-1">
            <h1 className="type-title">{course.summary.topic}</h1>
            <p className="text-sm text-fg-secondary">{outline.summary}</p>
            <p className="font-mono text-xs text-fg-tertiary">{course.summary.units} units · {course.summary.lessons} lessons · about {Math.round((course.summary.minutes ?? 0) / 6) / 10}h</p>
          </div>
          {edit.error && <Banner tone="blocked" title={edit.error.message} />}
          {outline.units.map((u, i) => (
            <Card key={u.title} bodyClassName="!p-0">
              <div className="border-b border-border-subtle px-4 py-2 text-xs font-semibold text-fg-2">Unit {i + 1} · {u.title}</div>
              <ul>
                {u.lessons.map((l) => (
                  <li key={l.key} className="flex items-center gap-3 border-b border-border-row px-4 py-2 last:border-b-0">
                    <span className="flex flex-none flex-col">
                      <button type="button" aria-label={`Move ${l.title} up`} disabled={edit.isPending} onClick={() => send(moveLesson(orderOf(outline), l.key, -1), known)} className="text-fg-tertiary hover:text-fg disabled:opacity-50"><Icon name="chev" size={12} className="rotate-180" /></button>
                      <button type="button" aria-label={`Move ${l.title} down`} disabled={edit.isPending} onClick={() => send(moveLesson(orderOf(outline), l.key, 1), known)} className="text-fg-tertiary hover:text-fg disabled:opacity-50"><Icon name="chev" size={12} /></button>
                    </span>
                    <span className="w-7 flex-none font-mono text-xs text-fg-tertiary">{number.get(l.key)}</span>
                    <span className="min-w-0 flex-1">
                      <span className={cx("block truncate text-[13px] font-medium", l.known ? "text-fg-tertiary" : "text-fg")}>{l.title}</span>
                      <span className="block truncate text-xs text-fg-tertiary">{l.objectives[0]}</span>
                    </span>
                    {l.prereqs.length > 0 && <span className="flex-none font-mono text-[11.5px] text-fg-secondary">needs {l.prereqs.map((p) => number.get(p) ?? p).join(", ")}</span>}
                    <Button variant={l.known ? "secondary" : "outline"} aria-pressed={!!l.known} disabled={edit.isPending} onClick={() => send(orderOf(outline), l.known ? known.filter((k) => k !== l.key) : [...known, l.key])}>
                      {l.known ? "✓ Known" : "Known"}
                    </Button>
                  </li>
                ))}
              </ul>
            </Card>
          ))}
          <p className="text-xs text-fg-tertiary">Use the arrows to reorder. Every change is checked against the prerequisites.</p>
        </div>
        <aside className="flex flex-col gap-4">
          <section className="space-y-2">
            <h2 className="type-heading">What will be researched</h2>
            <ul className="space-y-2 text-[12.5px] text-fg-2">
              {outline.units.filter((u) => u.lessons.some((l) => !l.known)).map((u) => <li key={u.title}>{u.question}</li>)}
            </ul>
          </section>
          <section className="flex flex-col gap-3 border-t border-border-subtle pt-4">
            <h2 className="type-heading">Limits</h2>
            <Field label="Sources" htmlFor="course-sources"><TextInput id="course-sources" mono type="number" min={SOURCE_RANGE.min} max={SOURCE_RANGE.max} value={sources} onChange={(e) => setSources(Number(e.target.value))} /></Field>
            <Field label="Minutes per run" htmlFor="course-minutes"><TextInput id="course-minutes" mono type="number" min={MINUTE_RANGE.min} max={MINUTE_RANGE.max} value={minutes} onChange={(e) => setMinutes(Number(e.target.value))} /></Field>
            <Field label="Spend limit, USD" hint="Leave empty for no limit" htmlFor="course-spend"><TextInput id="course-spend" mono type="number" min={0} step="any" placeholder="no limit" value={spend} onChange={(e) => setSpend(e.target.value)} /></Field>
            <p className="text-xs text-fg-tertiary">About {estimatedRuns(outline.units)} runs, more if a lesson needs its one rewrite. They queue behind other work.</p>
            {approve.error && <Banner tone="blocked" title={approve.error.message} />}
            <Button variant="primary" className="justify-center" disabled={!limitsOk || nothingLeft || approve.isPending} onClick={start}>{approve.isPending ? "Starting…" : "Approve and start research"}</Button>
          </section>
        </aside>
      </div>
    </div>
  );
}
