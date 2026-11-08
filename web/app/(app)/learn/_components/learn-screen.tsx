"use client";

import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { useState } from "react";
import { useBootstrap, useChatModels } from "@/lib/api/hooks";
import { useCourses, useCreateCourse, type CourseSummary } from "@/lib/api/learning-hooks";
import { usePersonas } from "@/lib/api/persona-hooks";
import { currentProject } from "@/lib/project-scope";
import { BUILD_STEPS, COURSE_STATE_LABELS, buildStepIndex, type BuildState } from "@/lib/learning";
import { cx } from "@/lib/cx";
import { useRememberedProject } from "@/components/shell/project-scope";
import { Content, TopBar } from "@/components/shell/topbar";
import { Button } from "@/components/ui/button";
import { Segmented } from "@/components/ui/controls";
import { Banner, Card, EmptyState, Field, Skeleton, TextInput } from "@/components/ui/primitives";
import { ModelPicker, type Selection } from "@/app/(app)/chat/_components/model-picker";

const LEVELS = [
  { value: "new", label: "New" },
  { value: "some", label: "Some background" },
  { value: "experienced", label: "Experienced" },
] as const;
const TIMES = [
  { value: "60", label: "1h" },
  { value: "180", label: "3h" },
  { value: "480", label: "8h" },
] as const;

type Level = (typeof LEVELS)[number]["value"];
type Time = (typeof TIMES)[number]["value"];

function NewModule() {
  const router = useRouter();
  const search = useSearchParams();
  const boot = useBootstrap();
  const remembered = useRememberedProject();
  const personas = usePersonas();
  const models = useChatModels();
  const create = useCreateCourse();
  const [topic, setTopic] = useState("");
  const [goal, setGoal] = useState("");
  const [level, setLevel] = useState<Level>("some");
  const [time, setTime] = useState<Time>("180");
  const [tutor, setTutor] = useState("");
  const [picked, setPicked] = useState<Selection | null>(null);
  const projects = boot.data?.projects ?? [];
  // A module belongs to a project only for its model and permissions, so any open one will do.
  const project = currentProject("/learn", search, remembered, projects) ?? projects.find((p) => !p.archived);
  const groups = models.data;
  const fallback = groups?.flatMap((g) => g.models.filter((m) => m.default).map((m) => ({ harness: g.harness, model: m.id })))[0];
  const selection = picked ?? fallback;
  const list = personas.data?.personas ?? [];
  const submit = () => {
    if (!project || !topic.trim()) return;
    create.mutate(
      { project: project.key, topic: topic.trim(), goal: goal.trim() || undefined, level, minutes_budget: Number(time), persona_id: tutor || undefined, harness: selection?.harness, model: selection?.model },
      { onSuccess: (c) => router.push(`/learn/${c.summary.id}`) },
    );
  };
  return (
    <Card>
      <form className="flex flex-col gap-3" onSubmit={(e) => { e.preventDefault(); submit(); }}>
        <Field label="What do you want to learn?" htmlFor="learn-topic">
          <TextInput id="learn-topic" maxLength={200} placeholder="How Postgres MVCC and vacuum actually work" value={topic} onChange={(e) => setTopic(e.target.value)} />
        </Field>
        <Field label="What's it for? This shapes what gets taught (optional)" htmlFor="learn-goal">
          <TextInput id="learn-goal" maxLength={1000} placeholder="I debug slow queries at work and want to understand table bloat" value={goal} onChange={(e) => setGoal(e.target.value)} />
        </Field>
        <div className="flex flex-wrap items-end gap-x-6 gap-y-3">
          <Field label="Your level"><Segmented label="Your level" options={LEVELS} value={level} onChange={setLevel} /></Field>
          <Field label="Time"><Segmented label="Time" options={TIMES} value={time} onChange={setTime} /></Field>
          <Field label="Tutor" htmlFor="learn-tutor">
            <select id="learn-tutor" value={tutor} onChange={(e) => setTutor(e.target.value)} className="h-[30px] rounded-md border border-border-strong bg-btn-outline px-2 text-[13px] text-fg-2">
              <option value="">Default persona</option>
              {list.map((p) => <option key={p.id} value={p.id}>{p.name}</option>)}
            </select>
          </Field>
          {selection && <Field label="Model"><ModelPicker groups={groups} value={selection} onChange={setPicked} /></Field>}
          <Button variant="primary" type="submit" className="ml-auto" disabled={!topic.trim() || !project || create.isPending}>{create.isPending ? "Starting…" : "Draft outline"}</Button>
        </div>
        <p className="text-xs text-fg-tertiary">Drafting takes one short run. You approve the outline before any research starts.</p>
        {create.error && <Banner tone="blocked" title={create.error.message} />}
      </form>
    </Card>
  );
}

function StatePill({ state }: { state: BuildState }) {
  const human = state === "outline_ready";
  const blocked = state === "failed";
  return (
    <span className={cx("inline-block rounded px-1.5 py-0.5 text-[11px] font-medium", human ? "bg-pill-human-bg text-pill-human-fg" : blocked ? "bg-pill-blocked-bg text-pill-blocked-fg" : "bg-selected text-fg-2")}>
      {COURSE_STATE_LABELS[state]}
    </span>
  );
}

function ProgressBar({ value, total }: { value: number; total: number }) {
  return (
    <div className="h-1 w-full overflow-hidden rounded-full bg-track" role="progressbar" aria-valuemin={0} aria-valuemax={total} aria-valuenow={value}>
      <div className="h-full rounded-full bg-fg-2" style={{ width: `${total ? (value / total) * 100 : 0}%` }} />
    </div>
  );
}

function BuildCard({ c }: { c: CourseSummary }) {
  const step = buildStepIndex(c.state);
  return (
    <Link href={`/learn/${c.id}`} className="flex flex-col gap-2 rounded-lg border border-border bg-surface p-4 hover:border-border-hover">
      <span className="truncate text-[14px] font-semibold text-fg">{c.topic}</span>
      <span><StatePill state={c.state} /></span>
      <span className="truncate font-mono text-xs text-fg-tertiary">
        {c.state === "failed" ? c.error : c.build_note || (c.state === "outline_ready" ? `${c.units} units · ${c.lessons} lessons` : " ")}
      </span>
      {step >= 0 && (
        <span className="flex gap-1" aria-hidden>
          {BUILD_STEPS.map((s, i) => <span key={s.state} className={cx("h-1 flex-1 rounded-full", i < step ? "bg-fg-2" : "bg-track")} />)}
        </span>
      )}
    </Link>
  );
}

function ModuleCard({ c }: { c: CourseSummary }) {
  return (
    <Link href={`/learn/${c.id}`} className="flex flex-col gap-2 rounded-lg border border-border bg-surface p-4 hover:border-border-hover">
      <span className="truncate text-[14px] font-semibold text-fg">{c.topic}</span>
      <span className="font-mono text-xs text-fg-tertiary">{c.complete} of {c.lessons} complete · {c.solid} solid</span>
      <ProgressBar value={c.complete} total={c.lessons} />
      <span className="flex items-center justify-between gap-2 text-[13px]">
        <span className={cx("truncate font-medium", c.next_lesson ? "text-accent" : "text-fg-secondary")}>{c.next_lesson ? `Continue: ${c.next_lesson}` : "Finished"}</span>
        {c.flagged > 0 && <span className="flex-none text-xs text-pill-human-fg">{c.flagged} flagged</span>}
      </span>
    </Link>
  );
}

const GRID = "grid gap-4 [grid-template-columns:repeat(auto-fill,minmax(300px,1fr))]";

export function LearnScreen() {
  const courses = useCourses();
  const all = courses.data ?? [];
  const building = all.filter((c) => c.state !== "ready");
  const ready = all.filter((c) => c.state === "ready");
  return (
    <>
      <TopBar crumbs={[{ label: "Learn" }]} />
      <Content>
        <div className="flex flex-col gap-6">
          <div className="space-y-1">
            <h1 className="type-display">Learn</h1>
            <p className="text-sm text-fg-secondary">Turn a topic into a sourced, properly ordered module, then work through it with a tutor.</p>
          </div>
          <NewModule />
          {courses.error && <Banner tone="blocked" title={courses.error.message} />}
          {courses.isPending ? (
            <Skeleton className="h-28 w-full rounded-lg" />
          ) : all.length === 0 ? (
            <EmptyState icon="learn" title="No modules yet">Name a topic above and you&apos;ll get an outline to review.</EmptyState>
          ) : (
            <>
              {building.length > 0 && (
                <section className="flex flex-col gap-3">
                  <h2 className="type-heading">In progress</h2>
                  <div className={GRID}>{building.map((c) => <BuildCard key={c.id} c={c} />)}</div>
                </section>
              )}
              {ready.length > 0 && (
                <section className="flex flex-col gap-3">
                  <h2 className="type-heading">Your modules</h2>
                  <div className={GRID}>{ready.map((c) => <ModuleCard key={c.id} c={c} />)}</div>
                </section>
              )}
            </>
          )}
        </div>
      </Content>
    </>
  );
}
