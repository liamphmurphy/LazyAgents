"use client";

import { Dialog } from "radix-ui";
import { useMemo, useState, type ReactNode } from "react";
import type { AgentDetail } from "@/lib/api/client";
import { collapseDiff, effectiveMode, scopeChanges, type ScopeChange } from "@/lib/agent-scopes";
import { cx } from "@/lib/cx";
import { diffLines } from "@/lib/diff";
import { enforcementFor, MODE_LABEL } from "@/lib/enforcement";
import { Button, IconButton } from "@/components/ui/button";
import { Field, TextInput } from "@/components/ui/primitives";
import { Flag } from "@/components/domain/labels";
import { describeChanges, type Draft } from "./definition-editor";

const KIND: Record<ScopeChange["kind"], { tone: "human" | "blocked" | "neutral"; label: string }> = {
  approval: { tone: "human", label: "Requires approval" },
  widens: { tone: "blocked", label: "Widens access" },
  tightens: { tone: "neutral", label: "Tightens" },
  none: { tone: "neutral", label: "No effect at run time" },
};

const plural = (n: number, one: string, many = `${one}s`) => `${n} ${n === 1 ? one : many}`;

function Section({ title, meta, children }: { title: string; meta?: ReactNode; children: ReactNode }) {
  return (
    <section className="flex flex-col gap-2">
      <h3 className="flex items-center gap-2 text-[13px] font-semibold">
        {title}
        {meta}
      </h3>
      {children}
    </section>
  );
}

/**
 * Everything a save would change, before it's saved: the instructions diff,
 * each scope change with what it means at run time, and the rest. Saving is
 * an explicit second step (docs/dev/plan.md: a new definition version).
 */
export function ReviewDialog({
  detail,
  base,
  draft,
  liveRuns,
  pending,
  error,
  onSave,
  onClose,
}: {
  detail: AgentDetail;
  base: Draft;
  draft: Draft;
  liveRuns: number;
  pending: boolean;
  error?: string;
  onSave: (note: string) => void;
  onClose: () => void;
}) {
  const next = detail.definition.version + 1;
  const [note, setNote] = useState(() => describeChanges(base, draft));
  const [showAll, setShowAll] = useState(false);
  const diff = useMemo(() => diffLines(base.spec.instructions, draft.spec.instructions), [base.spec.instructions, draft.spec.instructions]);
  const rows = useMemo(() => (showAll ? collapseDiff(diff, Infinity) : collapseDiff(diff, 2)), [diff, showAll]);
  const adds = diff.filter((l) => l.op === "add").length;
  const dels = diff.filter((l) => l.op === "del").length;
  const scopes = scopeChanges(base.spec.scopes, draft.spec.scopes);
  const approvals = scopes.filter((s) => s.kind === "approval");
  const widens = scopes.filter((s) => s.kind === "widens");
  // Everything else, described the same way as the version note.
  const otherNote = describeChanges(base, { ...draft, spec: { ...draft.spec, instructions: base.spec.instructions, scopes: base.spec.scopes } });
  const others = otherNote === "No changes" ? [] : otherNote.split("; ");
  // Project overrides that keep a changed scope tighter than the new definition.
  const held = detail.overrides.flatMap((o) =>
    scopes
      .map((s) => ({ s, eff: effectiveMode(draft.spec.scopes, o.override.scopes, s.scope) }))
      .filter(({ s, eff }) => eff.source === "project" && eff.mode !== s.effectiveTo)
      .map(({ s, eff }) => `in ${o.project.name}, ${s.scope} stays ${MODE_LABEL[eff.mode]}`),
  );
  const save = () => !pending && onSave(note.trim() || describeChanges(base, draft));

  return (
    <Dialog.Root open onOpenChange={(o) => !o && onClose()}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 z-40 bg-scrim" />
        <Dialog.Content
          aria-describedby="review-summary"
          onKeyDown={(e) => {
            if ((e.metaKey || e.ctrlKey) && e.key === "Enter") {
              e.preventDefault();
              save();
            }
          }}
          className="fixed left-1/2 top-[6vh] z-50 flex max-h-[88vh] w-[min(760px,calc(100%-32px))] -translate-x-1/2 flex-col overflow-hidden rounded-2xl border border-border-float bg-surface-raised shadow-[var(--shadow-float)] outline-none"
        >
          <div className="flex items-center gap-2 px-[18px] pt-3.5">
            <Dialog.Title className="type-heading">Review changes to {detail.agent.name}</Dialog.Title>
            <Dialog.Close asChild>
              <IconButton icon="x" label="Back to editing" className="ml-auto" />
            </Dialog.Close>
          </div>
          <div className="flex flex-col gap-5 overflow-y-auto px-[18px] pb-4 pt-2">
            <p id="review-summary" className="text-[13px] text-fg-secondary">
              Saving creates definition <b className="font-mono font-medium text-fg">v{next}</b>. New runs use it;{" "}
              {liveRuns > 0 ? `the ${plural(liveRuns, "live run")} keep${liveRuns === 1 ? "s" : ""} the v${detail.definition.version} snapshot.` : "runs in progress keep the definition they started with."}
            </p>
            {(approvals.length > 0 || widens.length > 0) && (
              <div role="note" className="flex gap-3 rounded-lg border border-pill-human-bg bg-pill-human-bg/40 px-3 py-2.5 text-[13px]">
                <span aria-hidden className="flex size-5 flex-none items-center justify-center rounded-full bg-state-human font-semibold text-on-accent">!</span>
                <div className="flex flex-col gap-0.5">
                  <b className="font-medium">
                    {[approvals.length && `${plural(approvals.length, "change")} require${approvals.length === 1 ? "s" : ""} approval at run time`, widens.length && `${plural(widens.length, "change")} widen${widens.length === 1 ? "s" : ""} access`]
                      .filter(Boolean)
                      .join(" · ")}
                  </b>
                  {approvals.length > 0 && (
                    <span className="text-fg-secondary">
                      {approvals.map((s) => s.scope).join(" and ")} will pause the run until a person approves each use in the Inbox.
                    </span>
                  )}
                  {widens.length > 0 && <span className="text-fg-secondary">{widens.map((s) => s.scope).join(" and ")} will run with no prompt.</span>}
                </div>
              </div>
            )}

            {diff.some((l) => l.op !== "same") && (
              <Section
                title="Instructions"
                meta={
                  <span className="font-mono text-xs font-normal">
                    <span className="text-state-done">+{adds}</span> <span className="text-pill-blocked-fg">−{dels}</span>
                  </span>
                }
              >
                <div role="table" aria-label="Instructions diff" className="overflow-x-auto rounded-lg border border-border bg-surface font-mono text-[12px]">
                  {rows.map((r, i) =>
                    r.kind === "gap" ? (
                      <div key={i} role="row" className="flex items-center gap-2 border-y border-border-subtle bg-surface-sunken px-3 py-1 text-[11.5px] text-fg-tertiary">
                        {plural(r.count, "unchanged line")} hidden ·{" "}
                        <button type="button" className="text-accent hover:underline" onClick={() => setShowAll(true)}>
                          Show all
                        </button>
                      </div>
                    ) : (
                      <div
                        key={i}
                        role="row"
                        className={cx("flex whitespace-pre", r.op === "add" && "bg-pill-done-bg/60", r.op === "del" && "bg-pill-blocked-bg/60")}
                      >
                        <span className="w-9 flex-none select-none pr-2 text-right text-fg-tertiary">{r.op === "del" ? r.oldNo : r.newNo}</span>
                        <span className="w-4 flex-none select-none text-fg-tertiary">{r.op === "add" ? "+" : r.op === "del" ? "−" : " "}</span>
                        <span className="pr-3">{r.text || " "}</span>
                      </div>
                    ),
                  )}
                </div>
              </Section>
            )}

            {scopes.length > 0 && (
              <Section title="Scopes" meta={<span className="text-xs font-normal text-fg-tertiary">{plural(scopes.length, "change")}</span>}>
                <ul className="flex flex-col divide-y divide-border-row rounded-lg border border-border bg-surface">
                  {scopes.map((s) => (
                    <li key={s.scope} className="flex flex-col gap-1 px-3 py-2.5">
                      <div className="flex flex-wrap items-center gap-2 text-[13px]">
                        <span className="font-mono">{s.scope}</span>
                        <span className="text-fg-secondary">
                          {s.from ? MODE_LABEL[s.from] : "Not granted"} → {s.to ? MODE_LABEL[s.to] : "Not granted"}
                        </span>
                        <span className="ml-auto">
                          <Flag tone={KIND[s.kind].tone}>{KIND[s.kind].label}</Flag>
                        </span>
                      </div>
                      <span className="text-xs text-fg-tertiary">
                        {s.to && s.effectiveTo !== s.to
                          ? `A wildcard in the definition keeps it at ${MODE_LABEL[s.effectiveTo]}.`
                          : s.to
                            ? enforcementFor(s.scope, s.to, draft.spec.harness, detail).how
                            : s.effectiveTo === "denied"
                              ? "Removed from the definition, so it's denied."
                              : `Removed, but a wildcard still grants it at ${MODE_LABEL[s.effectiveTo]}.`}
                      </span>
                    </li>
                  ))}
                </ul>
              </Section>
            )}

            {others.length > 0 && (
              <Section title="Other" meta={<span className="text-xs font-normal text-fg-tertiary">{plural(others.length, "change")}</span>}>
                <ul className="flex flex-col divide-y divide-border-row rounded-lg border border-border bg-surface text-[13px]">
                  {others.map((o) => (
                    <li key={o} className="px-3 py-2">
                      {o}
                    </li>
                  ))}
                </ul>
              </Section>
            )}

            <Field label="Version note" htmlFor="review-note">
              <TextInput id="review-note" value={note} maxLength={2000} onChange={(e) => setNote(e.target.value)} />
            </Field>
            {held.length > 0 && <p className="-mt-3 text-xs text-fg-tertiary">Project overrides still apply on top: {held.join("; ")}.</p>}
          </div>
          <div className="flex flex-wrap items-center justify-end gap-3 border-t border-track bg-surface-sunken px-[18px] py-3">
            {error ? (
              <span role="alert" className="mr-auto text-xs text-pill-blocked-fg">
                {error}
              </span>
            ) : (
              <span className="mr-auto text-xs text-fg-tertiary">⌘↵ saves · Esc returns to editing</span>
            )}
            <Dialog.Close asChild>
              <Button variant="ghost">Back to editing</Button>
            </Dialog.Close>
            <Button variant="primary" disabled={pending} onClick={save}>
              Save as v{next}
            </Button>
          </div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}
