"use client";

import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { useCourse, useDeleteCourse, useStopCourse } from "@/lib/api/learning-hooks";
import { COURSE_STATE_LABELS, courseBuilding, type BuildState } from "@/lib/learning";
import { Content, TopBar } from "@/components/shell/topbar";
import { Button } from "@/components/ui/button";
import { Banner, Skeleton } from "@/components/ui/primitives";
import { BuildStepper, OutlineReview } from "./outline-review";
import { StudyView } from "./study-view";

export function CourseScreen({ id }: { id: string }) {
  const router = useRouter();
  const search = useSearchParams();
  const course = useCourse(id);
  const stop = useStopCourse(id);
  const del = useDeleteCourse();
  const discard = () => {
    if (window.confirm("Delete this module, its progress and its tutor conversation?")) del.mutate(id, { onSuccess: () => router.push("/learn") });
  };
  const c = course.data;
  const state = c?.summary.state as BuildState | undefined;
  // A later part's outline is reviewed here; otherwise what's built is studied.
  const laterPart = !!c && c.summary.part > 0;
  const actions = c && (laterPart ? (
    <>
      <Link href={`/learn/${id}`} className="inline-flex h-7 items-center rounded-md px-2.5 text-xs text-fg-secondary hover:bg-hover">Back to studying</Link>
      <Button variant="outline" disabled={stop.isPending} onClick={() => stop.mutate(undefined, { onSuccess: () => router.push(`/learn/${id}`) })}>Discard this part</Button>
    </>
  ) : (
    <>
      {courseBuilding(c.summary.state) && <Button disabled={stop.isPending} onClick={() => stop.mutate()}>Stop</Button>}
      <Button variant="outline" disabled={del.isPending} onClick={discard}>Delete</Button>
    </>
  ));
  const crumbs = [{ label: "Learn", href: "/learn" }, { label: c?.summary.topic ?? "Module" }];
  if (c && c.summary.studyable && !(search.get("review") === "1" && state === "outline_ready")) return <StudyView course={c} onDelete={discard} />;
  return (
    <>
      <TopBar crumbs={crumbs} actions={actions} />
      <Content>
        <div className="flex flex-col gap-5">
          {course.error && <Banner tone="blocked" title={course.error.message} />}
          {(stop.error || del.error) && <Banner tone="blocked" title={(stop.error ?? del.error)?.message} />}
          {course.isPending && <Skeleton className="h-64 w-full rounded-lg" />}
          {c && state && (
            <>
              <BuildStepper state={state} />
              {state === "failed" && <Banner tone="blocked" title="This module couldn't be built">{c.summary.error}</Banner>}
              {state === "stopped" && <Banner tone="blocked" title="This module was stopped">Delete it, or start a new one from Learn.</Banner>}
              {state === "outline_ready" ? (
                <OutlineReview course={c} />
              ) : (
                <div className="space-y-1">
                  <h1 className="type-title">{c.summary.topic}</h1>
                  {courseBuilding(state) && (
                    <p className="text-sm text-fg-secondary">
                      {COURSE_STATE_LABELS[state]}{c.summary.build_note ? ` · ${c.summary.build_note}` : ""}. This takes a while; you can leave and come back.
                    </p>
                  )}
                </div>
              )}
            </>
          )}
        </div>
      </Content>
    </>
  );
}
