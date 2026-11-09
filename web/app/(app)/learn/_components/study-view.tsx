"use client";

import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { useEffect, useRef, useState } from "react";
import { useAttemptCheck, useGradeAttempt, useTutor, type Course, type CourseCheck, type CourseLesson } from "@/lib/api/learning-hooks";
import { CONFIDENCE, pickLesson } from "@/lib/learning";
import { cx } from "@/lib/cx";
import { Markdown } from "@/components/domain/markdown";
import { LessonVisual } from "@/components/domain/visuals";
import { Button } from "@/components/ui/button";
import { Segmented } from "@/components/ui/controls";
import { Banner, TextArea } from "@/components/ui/primitives";

/** Markdown links, so a citation sits at the end of its paragraph. */
const cites = (ns?: number[]) => (ns ?? []).map((n) => ` [\\[${n}\\]](#source-${n})`).join("");

function Body({ lesson }: { lesson: CourseLesson }) {
  return (
    <div className="flex flex-col gap-3 text-[14px] leading-[1.65] text-fg-2">
      {lesson.body.map((b, i) => {
        if (b.kind === "heading") return <h2 key={i} className="type-heading mt-2">{b.text}</h2>;
        if (b.kind === "code") return <pre key={i} className="overflow-x-auto rounded-md border border-border bg-surface-sunken p-3 font-mono text-xs"><code>{b.text}</code></pre>;
        const visual = b.kind === "diagram" || b.kind === "chart" ? <LessonVisual kind={b.kind} text={b.text} alt={b.alt} steps={b.steps} caption={b.caption || b.sources?.length ? <Markdown>{(b.caption ?? "") + cites(b.sources)}</Markdown> : undefined} /> : null;
        if (b.provenance === "model_knowledge") {
          return (
            <div key={i} className="flex flex-col gap-1 border-l border-dashed border-fg-tertiary pl-3">
              <div className="text-[10px] font-semibold tracking-[.06em] text-fg-tertiary">MODEL KNOWLEDGE · NOT SOURCED</div>
              {visual ?? <Markdown>{b.text}</Markdown>}
            </div>
          );
        }
        if (visual) return <div key={i}>{visual}</div>;
        return <Markdown key={i}>{b.text + cites(b.sources)}</Markdown>;
      })}
    </div>
  );
}

function Reveal({ check }: { check: CourseCheck }) {
  if (!check.reveal) return null;
  return (
    <div className="space-y-1 rounded-md bg-surface-sunken p-3 text-[13px] text-fg-2">
      {check.kind === "short" && <p><span className="font-semibold">A good answer: </span>{check.reveal.answer[0]}</p>}
      {check.kind === "short" && <p><span className="font-semibold">It should cover: </span>{check.reveal.rubric}</p>}
      {check.kind === "mcq" && <p><span className="font-semibold">Answer: </span>{check.reveal.answer[0]}</p>}
      {check.kind === "order" && <ol className="list-decimal pl-5">{check.reveal.answer.map((a) => <li key={a}>{a}</li>)}</ol>}
      <p>{check.reveal.explanation}</p>
    </div>
  );
}

function CheckCard({ course, check, index, total }: { course: string; check: CourseCheck; index: number; total: number }) {
  const attempt = useAttemptCheck(course, check.id);
  const grade = useGradeAttempt(course);
  const [choice, setChoice] = useState("");
  const [text, setText] = useState("");
  const [order, setOrder] = useState(check.options);
  const [confidence, setConfidence] = useState<"1" | "2" | "3">("2");
  const move = (i: number, d: -1 | 1) => setOrder((o) => {
    const n = [...o];
    if (i + d < 0 || i + d >= n.length) return o;
    [n[i], n[i + d]] = [n[i + d], n[i]];
    return n;
  });
  const response = check.kind === "mcq" ? [choice] : check.kind === "order" ? order : [text];
  const ready = check.kind === "mcq" ? !!choice : check.kind === "order" || !!text.trim();
  const waitingMark = !!check.ungraded_attempt_id;
  const error = attempt.error ?? grade.error;
  return (
    <section className="flex flex-col gap-3 rounded-lg border border-border-strong bg-surface p-5" aria-label={`Check ${index + 1} of ${total}`}>
      <div className="text-[10px] font-semibold tracking-[.06em] text-fg-tertiary">CHECK {index + 1} OF {total}</div>
      <p className="text-[14px] font-semibold text-fg">{check.prompt}</p>
      {check.kind === "mcq" && (
        <div className="flex flex-col gap-2" role="radiogroup">
          {check.options.map((o) => (
            <label key={o} className={cx("flex cursor-pointer items-center gap-3 rounded-md border px-3 py-2.5 text-[13px]", choice === o ? "border-accent bg-accent-soft text-fg" : "border-border-strong bg-surface-sunken text-fg-2")}>
              <input type="radio" name={check.id} checked={choice === o} onChange={() => setChoice(o)} className="accent-[var(--accent)]" />{o}
            </label>
          ))}
        </div>
      )}
      {check.kind === "order" && (
        <ol className="flex flex-col gap-2">
          {order.map((o, i) => (
            <li key={o} className="flex items-center gap-2 rounded-md border border-border-strong bg-surface-sunken px-3 py-2 text-[13px] text-fg-2">
              <span className="flex-1">{o}</span>
              <Button variant="ghost" aria-label={`Move ${o} up`} onClick={() => move(i, -1)}>↑</Button>
              <Button variant="ghost" aria-label={`Move ${o} down`} onClick={() => move(i, 1)}>↓</Button>
            </li>
          ))}
        </ol>
      )}
      {check.kind === "short" && <TextArea aria-label="Your answer" rows={3} value={text} onChange={(e) => setText(e.target.value)} />}
      {waitingMark ? (
        <div className="flex flex-wrap items-center gap-2">
          <span className="text-[13px] text-pill-human-fg">Compare with the answer below. Did you get it?</span>
          <Button variant="secondary" disabled={grade.isPending} onClick={() => grade.mutate({ attemptId: check.ungraded_attempt_id!, correct: true })}>Yes</Button>
          <Button variant="outline" disabled={grade.isPending} onClick={() => grade.mutate({ attemptId: check.ungraded_attempt_id!, correct: false })}>Not quite</Button>
        </div>
      ) : (
        <div className="flex flex-wrap items-center gap-3">
          <span className="text-xs text-fg-secondary">How sure are you?</span>
          <Segmented small label="How sure are you?" options={CONFIDENCE} value={confidence} onChange={setConfidence} />
          <Button variant="primary" className="ml-auto" disabled={!ready || attempt.isPending} onClick={() => attempt.mutate({ response, confidence: Number(confidence) })}>Check answer</Button>
        </div>
      )}
      {check.last_correct === true && <p role="status" className="text-[13px] text-state-done">Correct.</p>}
      {check.last_correct === false && !check.solved && <p role="status" className="text-[13px] text-fg-secondary">{check.reveal ? "Not this time." : "Not quite. Try again, or ask your tutor for a hint."}</p>}
      <Reveal check={check} />
      {error && <Banner tone="blocked" title={error.message} />}
    </section>
  );
}

const QUICK = ["Explain this differently", "Give me an example", "Quiz me on this"];

function TutorPanel({ course, lesson }: { course: Course; lesson: string }) {
  const { messages, send, running } = useTutor(course.summary.id, course.chat_id);
  const [body, setBody] = useState("");
  const end = useRef<HTMLDivElement>(null);
  const list = (messages.data?.messages ?? []).filter((m) => m.role !== "system");
  const count = list.length;
  useEffect(() => { end.current?.scrollIntoView?.({ block: "end" }); }, [count, running]);
  const post = (text: string) => {
    if (!text.trim() || running) return;
    send.mutate({ body: text.trim(), lesson }, { onSuccess: () => setBody("") });
  };
  return (
    <aside className="flex min-h-0 flex-col border-l border-border-subtle bg-panel lg:h-full" aria-label="Tutor">
      <div className="flex h-[52px] flex-none items-center justify-between border-b border-border-subtle px-4">
        <span className="text-[13px] font-semibold text-fg">Your tutor</span>
        <span className="text-[11px] text-fg-tertiary">Replies aren&apos;t reviewed</span>
      </div>
      <div className="flex min-h-[200px] flex-1 flex-col gap-3 overflow-y-auto p-4">
        {count === 0 && <p className="text-[13px] text-fg-tertiary">Stuck on something? Ask about this lesson and your tutor will work through it with you.</p>}
        {list.map((m) => m.role === "user" ? (
          <div key={m.id} className="ml-10 rounded-lg bg-surface-raised px-3 py-2 text-[13px] text-fg">{m.body_md}</div>
        ) : (
          <div key={m.id} className="text-[13px] text-fg-2"><Markdown visualBlocks>{m.body_md}</Markdown></div>
        ))}
        {running && <p role="status" className="text-xs text-fg-tertiary">Thinking…</p>}
        <div ref={end} />
      </div>
      <div className="flex flex-none flex-col gap-2 p-4 pt-2">
        <div className="flex flex-wrap gap-1.5">
          {QUICK.map((q) => <button key={q} type="button" disabled={running || send.isPending} onClick={() => post(q)} className="rounded-full border border-border-strong px-2.5 py-1 text-xs text-fg-secondary hover:bg-hover disabled:opacity-50">{q}</button>)}
        </div>
        {send.error && <Banner tone="blocked" title={send.error.message} />}
        <form onSubmit={(e) => { e.preventDefault(); post(body); }}>
          <TextArea aria-label="Ask about this lesson" rows={2} placeholder="Ask about this lesson…" value={body} onChange={(e) => setBody(e.target.value)} onKeyDown={(e) => { if (e.key === "Enter" && !e.shiftKey) { e.preventDefault(); post(body); } }} />
        </form>
      </div>
    </aside>
  );
}

function ProgressDot({ lesson, current }: { lesson: CourseLesson; current: boolean }) {
  const done = lesson.progress !== "new" || lesson.status === "known";
  return (
    <span aria-hidden className={cx("grid size-3.5 flex-none place-items-center rounded-full border text-[9px] font-bold", done ? "border-fg-2 bg-fg-2 text-bg" : current ? "border-2 border-accent" : lesson.status === "flagged" ? "border-state-human" : "border-border-strong")}>
      {done && "✓"}
    </span>
  );
}

export function StudyView({ course, onDelete }: { course: Course; onDelete: () => void }) {
  const router = useRouter();
  const search = useSearchParams();
  const id = course.summary.id;
  const lesson = pickLesson(course.lessons, search.get("lesson"));
  const units = [...new Set(course.lessons.map((l) => l.unit_title))];
  const open = course.lessons.filter((l) => l.status !== "known");
  const at = lesson ? open.findIndex((l) => l.key === lesson.key) : -1;
  const next = at >= 0 ? open[at + 1] : undefined;
  const scroller = useRef<HTMLDivElement>(null);
  const key = lesson?.key;
  useEffect(() => { scroller.current?.scrollTo?.(0, 0); }, [key]);
  const { summary } = course;
  const cited = new Set(lesson?.body.flatMap((b) => b.sources ?? []) ?? []);
  return (
    <div className="grid min-h-0 flex-1 lg:h-[100dvh] lg:grid-cols-[256px_minmax(0,1fr)_340px]">
      <nav className="overflow-y-auto border-r border-border-subtle bg-panel" aria-label="Lessons">
        <div className="flex h-[52px] items-center justify-between border-b border-border-subtle px-4 text-[13px]">
          <Link href="/learn" className="text-fg-tertiary hover:text-fg">← Learn</Link>
          <button type="button" onClick={onDelete} className="text-xs text-fg-tertiary hover:text-fg">Delete</button>
        </div>
        <div className="space-y-2 p-4">
          <h1 className="text-[14px] font-semibold text-fg">{summary.topic}</h1>
          <p className="font-mono text-xs text-fg-tertiary">{summary.complete} of {summary.lessons} complete · {summary.solid} solid</p>
          <div className="h-1 overflow-hidden rounded-full bg-track"><div className="h-full rounded-full bg-fg-2" style={{ width: `${summary.lessons ? (summary.complete / summary.lessons) * 100 : 0}%` }} /></div>
        </div>
        {units.map((u) => (
          <div key={u} className="px-2 pb-3">
            <div className="px-2 pb-1 text-[11px] font-semibold uppercase text-fg-tertiary">{u}</div>
            {course.lessons.filter((l) => l.unit_title === u).map((l) => (
              <button key={l.key} type="button" aria-current={l.key === key ? "page" : undefined} disabled={l.status === "known"} onClick={() => router.push(`/learn/${id}?lesson=${l.key}`)}
                className={cx("flex w-full items-start gap-2 rounded-md px-2 py-1.5 text-left text-[13px] disabled:cursor-default", l.key === key ? "bg-selected font-medium text-fg" : "text-fg-secondary hover:bg-hover")}>
                <span className="mt-0.5"><ProgressDot lesson={l} current={l.key === key} /></span>
                <span className="min-w-0">
                  <span className="block truncate">{l.title}</span>
                  {l.status === "flagged" && <span className="block text-[10.5px] text-pill-human-fg">Flagged: see reviewer note</span>}
                  {l.status === "known" && <span className="block text-[10.5px] text-fg-tertiary">You know this</span>}
                </span>
              </button>
            ))}
          </div>
        ))}
      </nav>
      <div ref={scroller} className="min-w-0 overflow-y-auto">
        <div className="flex h-[52px] items-center justify-between border-b border-border-subtle px-6">
          <span className="truncate text-[13px] text-fg-tertiary">{lesson?.unit_title}{lesson && <span className="font-medium text-fg"> / {lesson.title}</span>}</span>
          {next && <Button variant="outline" onClick={() => router.push(`/learn/${id}?lesson=${next.key}`)}>Next →</Button>}
        </div>
        {lesson && (
          <article className="mx-auto flex max-w-[680px] flex-col gap-5 px-6 py-8">
            <header className="space-y-2">
              <div className="text-[10px] font-semibold tracking-[.06em] text-fg-tertiary">LESSON {at + 1} OF {open.length}</div>
              <h2 className="type-display">{lesson.title}</h2>
              <p className="flex flex-wrap items-center gap-3 text-xs">
                <span className="font-mono text-fg-tertiary">{lesson.minutes} min · {cited.size} sources</span>
                {lesson.status === "verified" && <span className="text-state-done">Reviewed</span>}
                <span className="text-fg-secondary">Goal: {lesson.objectives.join("; ")}</span>
              </p>
            </header>
            {lesson.shaky_prereqs.length > 0 && <Banner title="Prerequisite shaky">This builds on {lesson.shaky_prereqs.join(", ")}, which you haven&apos;t completed. You can carry on anyway.</Banner>}
            {lesson.status === "flagged" && <Banner title="The reviewer flagged this lesson">Check what it says before relying on it.{lesson.issues.length > 0 && <ul className="mt-1 list-disc pl-4">{lesson.issues.map((i) => <li key={i}>{i}</li>)}</ul>}</Banner>}
            <Body lesson={lesson} />
            {lesson.checks.map((k, i) => <CheckCard key={k.id} course={id} check={k} index={i} total={lesson.checks.length} />)}
            {cited.size > 0 && (
              <section className="space-y-1">
                <h3 className="text-xs font-semibold text-fg-secondary">Sources</h3>
                <ul className="space-y-1 text-xs text-fg-tertiary">
                  {course.sources.filter((s) => cited.has(s.n)).map((s) => (
                    <li key={s.n} id={`source-${s.n}`}><span className="text-accent">[{s.n}]</span> <a href={s.url} target="_blank" rel="noreferrer" className="hover:underline">{s.title}</a></li>
                  ))}
                </ul>
              </section>
            )}
          </article>
        )}
      </div>
      {lesson && <TutorPanel course={course} lesson={lesson.key} />}
    </div>
  );
}
