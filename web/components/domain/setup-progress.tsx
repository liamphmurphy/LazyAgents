"use client";

import { useSyncExternalStore } from "react";
import { cx } from "@/lib/cx";
import { rowState, setupProgress, SETUP_ROWS, type WizardStep } from "@/lib/onboarding";

// Where the Welcome Wizard stands, shared with the sidebar's "Getting set up"
// card. null while no wizard is on screen.

let current: WizardStep | null = null;
const listeners = new Set<() => void>();

export function setWizardStep(step: WizardStep | null) {
  if (step === current) return;
  current = step;
  listeners.forEach((l) => l());
}

function subscribe(l: () => void) {
  listeners.add(l);
  return () => listeners.delete(l);
}

export function useWizardStep(): WizardStep | null {
  return useSyncExternalStore(subscribe, () => current, () => null);
}

/** The bar uses the Vesper mark's dusk gradient, as tokens.css allows for brand moments. */
export const DUSK = "linear-gradient(90deg, var(--brand-deep), var(--brand-violet), var(--brand-star))";

/** The sidebar's checklist while the Welcome Wizard runs: a progress bar and a dot per step. */
export function SetupProgress() {
  const step = useWizardStep();
  if (step === null) return null;
  const pct = Math.round(setupProgress(step) * 100);
  return (
    <section aria-label="Getting set up" className="rounded-[10px] border border-border bg-surface p-2.5">
      <h2 className="text-12 font-semibold text-fg">Getting set up</h2>
      <div role="progressbar" aria-label="Setup progress" aria-valuemin={0} aria-valuemax={100} aria-valuenow={pct} className="mb-0.5 mt-2 h-[3px] overflow-hidden rounded-full bg-track">
        <i className="wizard-progress block h-full" style={{ width: `${pct}%`, background: DUSK }} />
      </div>
      <ol className="flex flex-col">
        {SETUP_ROWS.map((label, i) => {
          const state = rowState(i, step);
          return (
            <li key={label} data-state={state} aria-current={state === "current" ? "step" : undefined} className={cx("wizard-row flex items-center gap-2 py-[3px] text-12", state === "todo" ? "text-fg-secondary" : "text-fg")}>
              <span aria-hidden className="wizard-dot grid size-3.5 flex-none place-items-center rounded-full border-[1.5px]">
                <svg width="8" height="8" viewBox="0 0 10 10">
                  <path d="M2 5l2 2 4-4" stroke="var(--text-on-accent)" strokeWidth="2" fill="none" />
                </svg>
              </span>
              {label}
              {state === "done" && <span className="sr-only">(done)</span>}
            </li>
          );
        })}
      </ol>
    </section>
  );
}
