"use client";

import { useState } from "react";
import type { Artifact } from "@/lib/api/client";
import { useReviewMockup } from "@/lib/api/hooks";
import { absoluteTime, timelineTime } from "@/lib/format";
import type { Zoom } from "@/lib/mockup";
import { Button } from "@/components/ui/button";
import { Segmented } from "@/components/ui/controls";
import { Field, TextArea } from "@/components/ui/primitives";
import { HumanAvatar } from "@/components/domain/avatars";
import { MockupImage, ReviewChip } from "@/components/domain/mockup";

export const ZOOM_OPTIONS = [
  { value: "fit", label: "Fit" },
  { value: "100", label: "100%" },
  { value: "200", label: "200%" },
] as const;

export function ZoomControl({ value, onChange }: { value: Zoom; onChange: (z: Zoom) => void }) {
  return <Segmented small label="Zoom" value={value} onChange={onChange} options={ZOOM_OPTIONS} />;
}

/** The selected version's SVG with a zoom control. */
export function MockupViewer({ title, source }: { title: string; source: string | undefined }) {
  const [zoom, setZoom] = useState<Zoom>("fit");
  return (
    <div className="flex flex-col gap-2">
      <div className="flex items-center justify-end">
        <ZoomControl value={zoom} onChange={setZoom} />
      </div>
      {source == null ? (
        <p className="text-[13px] text-fg-tertiary">Loading mockup…</p>
      ) : (
        <MockupImage source={source} alt={title} zoom={zoom} className="max-h-[75vh]" />
      )}
    </div>
  );
}

/**
 * Review state of the viewed version. Only the current version can be
 * reviewed: older versions show their recorded outcome with no actions.
 */
export function MockupReviewBanner({ a, versionId, isOld }: { a: Artifact; versionId: string; isOld: boolean }) {
  const v = a.versions.find((x) => x.id === versionId) ?? a.current_version;
  const review = v.review;
  const mut = useReviewMockup(a.id);
  const [asking, setAsking] = useState(false);
  const [note, setNote] = useState("");
  const state = review?.state ?? "pending";

  const approve = () => mut.mutate({ versionId: v.id, decision: "approved" });
  const requestChanges = () =>
    mut.mutate(
      { versionId: v.id, decision: "changes_requested", note: note.trim() },
      {
        onSuccess: () => {
          setAsking(false);
          setNote("");
        },
      },
    );

  return (
    <section aria-label="Mockup review" className="flex flex-col gap-3 rounded-xl border border-border bg-surface px-4 py-3">
      <div className="flex flex-wrap items-center gap-2.5">
        <ReviewChip state={state} />
        <span className="font-mono text-xs text-fg-tertiary">{v.label}</span>
        {review?.reviewed_by && (
          <span className="flex items-center gap-1.5 text-[12.5px] text-fg-secondary">
            <HumanAvatar name={review.reviewed_by.name} initials={review.reviewed_by.initials} color={review.reviewed_by.avatar_color} src={review.reviewed_by.avatar_url} size={18} label={false} />
            {review.reviewed_by.name}
            {review.reviewed_at && (
              <time dateTime={review.reviewed_at} title={absoluteTime(review.reviewed_at)} className="text-fg-tertiary">
                · {timelineTime(review.reviewed_at)}
              </time>
            )}
          </span>
        )}
        {!isOld && (
          <div className="ml-auto flex flex-wrap items-center gap-2">
            <Button variant="approve" icon="check" disabled={state === "approved" || mut.isPending} onClick={approve}>
              Approve
            </Button>
            <Button variant="approve-outline" disabled={mut.isPending || asking} onClick={() => setAsking(true)}>
              Request changes
            </Button>
          </div>
        )}
      </div>
      {review?.note && <p className="whitespace-pre-line text-[13px] text-fg-2">“{review.note}”</p>}
      {isOld && <p className="text-xs text-fg-tertiary">Only the current version can be reviewed.</p>}
      {state === "pending" && !isOld && <p className="text-xs text-fg-tertiary">Agents that need an approved mockup wait for this review.</p>}
      {asking && !isOld && (
        <div className="flex flex-col gap-2">
          <Field label="What needs to change?" htmlFor="review-note">
            <TextArea id="review-note" autoFocus rows={3} value={note} onChange={(e) => setNote(e.target.value)} placeholder="e.g. The empty state is missing; move filters above the table." />
          </Field>
          <div className="flex justify-end gap-2">
            <Button variant="ghost" onClick={() => setAsking(false)}>
              Cancel
            </Button>
            <Button variant="primary" disabled={!note.trim() || mut.isPending} onClick={requestChanges}>
              Send feedback
            </Button>
          </div>
        </div>
      )}
      {mut.isError && (
        <p role="alert" className="text-xs text-pill-blocked-fg">
          {mut.error.message}
        </p>
      )}
    </section>
  );
}
