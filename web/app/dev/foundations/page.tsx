import type { Metadata } from "next";
import type { ReactNode } from "react";
import { Button, IconButton } from "@/components/ui/button";
import { Mark } from "@/components/ui/icon";
import { AddFilterChip, Banner, Card, EmptyState, Field, FilterChip, Kbd, Skeleton, TextArea, TextInput } from "@/components/ui/primitives";
import { AgentAvatar, AGENT_HUES, AvatarStack, HumanAvatar, UnassignedAvatar } from "@/components/domain/avatars";
import { KpiTile, ProgressBar, PropertyRow, StackedBar, Stat } from "@/components/domain/data";
import { PriorityGlyph, StatusGlyph, STATUS_LABEL, type GlyphState } from "@/components/domain/glyphs";
import { DocTypeTag, EnforcementChip, Flag, LabelChip, MonoChip, Pill, ScopeMode, SmallPill, type PillTone } from "@/components/domain/labels";
import { ThemeSwitch } from "@/components/shell/theme";
import { RangeDemo, ScopeModeDemo, TabsDemo, ToggleDemo } from "./interactive";

export const metadata: Metadata = { title: "Foundations · LazyAgents" };

const SURFACES: Array<[string, string]> = [
  ["Ground", "--bg"],
  ["Sidebar", "--bg-sidebar"],
  ["Surface", "--surface"],
  ["Raised", "--surface-raised"],
  ["Border", "--border"],
  ["Text", "--text"],
  ["Secondary", "--text-secondary"],
  ["Tertiary", "--text-tertiary"],
  ["Hover", "--hover"],
  ["Accent", "--accent"],
];

const STATES: Array<[GlyphState, PillTone, string, string]> = [
  ["backlog", "neutral", "Backlog", "--state-backlog"],
  ["todo", "neutral", "Todo", "--state-todo"],
  ["in_progress", "done", "Running", "--state-progress"],
  ["in_review", "review", "PR ready", "--state-review"],
  ["done", "done", "Merged", "--state-done"],
  ["human", "human", "Waiting", "--state-human"],
  ["blocked", "blocked", "Blocked", "--state-blocked"],
];

const AGENT_NAMES = ["Atlas", "Quill", "Sentry", "Juno", "Nova", "Relay", "Pike", "Echo"];

function Section({ title, children, className }: { title: string; children: ReactNode; className?: string }) {
  return (
    <section className={`flex flex-col gap-4 rounded-2xl border border-border bg-surface p-6 ${className ?? ""}`}>
      <h2 className="type-heading">{title}</h2>
      {children}
    </section>
  );
}

function Specimen({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="flex flex-col gap-2.5">
      <span className="text-[11.5px] text-fg-tertiary">{label}</span>
      <div className="flex flex-wrap items-center gap-2">{children}</div>
    </div>
  );
}

function TypeRow({ children, spec }: { children: ReactNode; spec: string }) {
  return (
    <div className="flex items-baseline justify-between gap-3">
      {children}
      <span className="flex-none font-mono text-[11px] text-fg-tertiary">{spec}</span>
    </div>
  );
}

export default function Foundations() {
  return (
    <div className="mx-auto flex max-w-[1440px] flex-col gap-10 px-16 py-14 max-md:px-4 max-md:py-8">
      <header className="flex flex-wrap items-end justify-between gap-6">
        <div className="flex items-center gap-4">
          <Mark size={52} />
          <div className="flex flex-col gap-0.5">
            <h1 className="text-[32px] font-semibold leading-[1.1] tracking-[-0.03em]">LazyAgents Foundations</h1>
            <span className="text-sm text-fg-secondary">The design language for supervising autonomous coding agents</span>
          </div>
        </div>
        <div className="flex items-center gap-4">
          <span className="font-mono text-xs text-fg-tertiary">v0.2 · live components</span>
          <ThemeSwitch />
        </div>
      </header>

      <Section title="Surfaces · current theme">
        <div className="grid grid-cols-[repeat(auto-fill,minmax(110px,1fr))] gap-2.5">
          {SURFACES.map(([name, v]) => (
            <div key={v} className="flex flex-col gap-1.5">
              <span className="h-[52px] rounded-[9px] border border-border-float" style={{ background: `var(${v})` }} />
              <span className="text-xs font-medium">{name}</span>
              <span className="font-mono text-[11px] text-fg-tertiary">{v}</span>
            </div>
          ))}
        </div>
      </Section>

      <div className="grid grid-cols-[repeat(auto-fit,minmax(320px,1fr))] gap-6">
        <Section title="Type · Inter & JetBrains Mono">
          <div className="flex flex-col gap-3.5">
            <TypeRow spec="26 / 600"><span className="type-display">Good morning, Maya</span></TypeRow>
            <TypeRow spec="19 / 600"><span className="type-title">Ledger v2</span></TypeRow>
            <TypeRow spec="14 / 600"><span className="type-heading">Needs your attention</span></TypeRow>
            <TypeRow spec="13 / 400"><span className="text-fg-2">Partition the table by posted_at month.</span></TypeRow>
            <TypeRow spec="12 / 400"><span className="type-small text-fg-secondary">Atlas · 12 min ago</span></TypeRow>
            <TypeRow spec="Mono 12"><span className="type-mono text-fg-2">PAY-214 · opus-5.5</span></TypeRow>
            <p className="prose-code text-fg-2">
              Inline <code>ledger_entries</code> code.
            </p>
          </div>
        </Section>

        <Section title="Workflow states">
          <div className="flex flex-col gap-[11px]">
            {STATES.map(([state, tone, pill, token]) => (
              <div key={state} className="flex items-center gap-3">
                <StatusGlyph state={state} surface="var(--surface)" label={false} />
                <span className="flex-1">{STATUS_LABEL[state]}</span>
                <SmallPill tone={tone}>{pill}</SmallPill>
                <span className="w-[120px] text-right font-mono text-[11px] text-fg-tertiary">{token}</span>
              </div>
            ))}
          </div>
        </Section>

        <Section title="Priority & identity">
          <div className="flex items-center gap-[18px]">
            {(["urgent", "high", "medium", "low", "none"] as const).map((p) => (
              <span key={p} className="flex flex-col items-center gap-1.5 text-[11.5px] capitalize text-fg-secondary">
                <PriorityGlyph priority={p} label={false} />
                {p}
              </span>
            ))}
          </div>
          <div className="h-px bg-border-subtle" />
          <div className="flex flex-wrap gap-1">
            {AGENT_HUES.map((h, i) => (
              <AgentAvatar key={h} name={AGENT_NAMES[i]} colorIndex={i} size="md" />
            ))}
          </div>
          <p className="text-[12.5px] leading-normal text-fg-secondary">
            <b className="font-medium text-fg">Agents are rounded squares</b>, each with a fixed hue.{" "}
            <b className="font-medium text-fg">People are circles</b> in neutral grey, so you can tell at a glance who did the work.
          </p>
          <div className="flex items-center gap-3">
            <HumanAvatar name="Maya Chen" size={24} />
            <HumanAvatar name="Dev Patel" size={24} color="#3A2F4A" />
            <HumanAvatar name="Lena Sato" size={24} color="#2F3A44" />
            <UnassignedAvatar />
            <AvatarStack
              actors={[
                { kind: "agent", name: "Atlas", colorIndex: 0 },
                { kind: "agent", name: "Quill", colorIndex: 1 },
                { kind: "human", name: "Maya Chen" },
                { kind: "agent", name: "Sentry", colorIndex: 2 },
                { kind: "agent", name: "Juno", colorIndex: 3 },
              ]}
            />
            <AgentAvatar name="Atlas" colorIndex={0} size="lg" />
          </div>
        </Section>
      </div>

      <Section title="Components">
        <div className="flex flex-wrap items-start gap-8">
          <Specimen label="Buttons · 30px">
            <Button variant="primary" icon="plus" hint="C">New ticket</Button>
            <Button variant="secondary">Assign agent</Button>
            <Button variant="outline">Cancel</Button>
            <Button variant="ghost" icon="history">History</Button>
            <Button variant="approve">Approve &amp; continue</Button>
            <Button variant="approve-outline">Deny</Button>
            <IconButton icon="dots" label="More actions" />
            <Button variant="primary" disabled>Disabled</Button>
          </Specimen>
          <Specimen label="Labels & flags">
            <LabelChip name="migration" color="var(--accent)" />
            <Flag tone="human">Needs approval</Flag>
            <Flag tone="review">PR #482</Flag>
            <Flag tone="blocked">Blocked</Flag>
            <MonoChip>repo:write</MonoChip>
            <MonoChip variant="outline">opus-5.5</MonoChip>
          </Specimen>
          <Specimen label="Status pills">
            <Pill tone="done">Running</Pill>
            <Pill tone="human">Waiting</Pill>
            <Pill tone="blocked">Off track</Pill>
            <Pill tone="neutral">Idle</Pill>
          </Specimen>
          <Specimen label="Artifact tags">
            {(["prd", "spec", "adr", "report", "design"] as const).map((t) => (
              <DocTypeTag key={t} type={t} />
            ))}
          </Specimen>
          <Specimen label="Segmented control">
            <RangeDemo />
          </Specimen>
          <Specimen label="Progress">
            <div className="flex w-[200px] flex-col gap-2">
              <ProgressBar value={0.62} label="Epic progress" />
              <StackedBar
                segments={[
                  { value: 12, color: "var(--state-done)", label: "done" },
                  { value: 1, color: "var(--state-review)", label: "in review" },
                  { value: 4, color: "var(--state-progress)", label: "in progress" },
                  { value: 2, color: "var(--track)", label: "todo" },
                ]}
              />
            </div>
          </Specimen>
        </div>
        <div className="grid grid-cols-[repeat(auto-fit,minmax(260px,1fr))] gap-4 border-t border-border-subtle pt-[18px]">
          <div className="flex flex-col gap-1">
            <span className="text-[13px] font-semibold">Color means state</span>
            <span className="text-[12.5px] leading-normal text-fg-secondary">Amber always asks for a human. Red always means blocked. The accent is kept for primary actions and focus.</span>
          </div>
          <div className="flex flex-col gap-1">
            <span className="text-[13px] font-semibold">Mono for machine output</span>
            <span className="text-[12.5px] leading-normal text-fg-secondary">Ticket IDs, branches, models, counts and permissions are set in JetBrains Mono, so they read as exact values.</span>
          </div>
          <div className="flex flex-col gap-1">
            <span className="text-[13px] font-semibold">Borders over shadows</span>
            <span className="text-[12.5px] leading-normal text-fg-secondary">Surfaces sit on 1px borders 4–6% brighter than the ground. Shadows are kept for floating menus and command bars.</span>
          </div>
        </div>
      </Section>

      <Section title="Added in v0.2">
        <div className="grid grid-cols-[repeat(auto-fit,minmax(300px,1fr))] gap-8">
          <div className="flex flex-col gap-4">
            <Field label="Title" htmlFor="f-title" hint="Agents read this first.">
              <TextInput id="f-title" defaultValue="Partition ledger_entries by month" />
            </Field>
            <Field label="Handle" htmlFor="f-handle">
              <TextInput id="f-handle" mono prefix="@" defaultValue="atlas" />
            </Field>
            <Field label="Instructions" htmlFor="f-instr">
              <TextArea id="f-instr" mono rows={3} defaultValue={"You own backend changes.\nPost a plan before editing."} />
            </Field>
          </div>
          <div className="flex flex-col gap-5">
            <Specimen label="View tabs">
              <TabsDemo />
            </Specimen>
            <Specimen label="Filter chips">
              <FilterChip label="Status" value="is not Done" />
              <FilterChip label="Agent" value="Atlas" />
              <AddFilterChip />
            </Specimen>
            <Specimen label="Toggle">
              <ToggleDemo />
            </Specimen>
            <Specimen label="Scope mode · enforcement">
              <div className="flex flex-col gap-2">
                <span className="flex items-center gap-3"><ScopeMode mode="allowed" /><EnforcementChip level="credential" /></span>
                <span className="flex items-center gap-3"><ScopeMode mode="requires_approval" /><EnforcementChip level="brokered" /></span>
                <span className="flex items-center gap-3"><ScopeMode mode="denied" /><EnforcementChip level="harness" /></span>
                <ScopeModeDemo />
              </div>
            </Specimen>
            <Specimen label="Keycaps">
              <Kbd>⌘K</Kbd><Kbd>C</Kbd><Kbd>⌘↵</Kbd>
            </Specimen>
          </div>
          <div className="flex flex-col gap-4">
            <Card title="Properties" bodyClassName="px-2.5 py-2">
              <PropertyRow label="Status"><StatusGlyph state="in_progress" surface="var(--surface)" label={false} />In progress</PropertyRow>
              <PropertyRow label="Priority"><PriorityGlyph priority="urgent" label={false} />Urgent</PropertyRow>
              <PropertyRow label="Agent"><AgentAvatar name="Atlas" colorIndex={0} label={false} />Atlas</PropertyRow>
              <PropertyRow label="Branch"><span className="font-mono text-xs">atlas/pay-214-partition</span></PropertyRow>
            </Card>
            <div className="flex gap-6">
              <Stat label="Run time" value="38m" />
              <Stat label="Tool calls" value="142" />
              <Stat label="Spend" value="$4.18" />
            </div>
          </div>
        </div>
        <div className="flex flex-col gap-3">
          <Banner
            lead={<AgentAvatar name="Atlas" colorIndex={0} size="md" />}
            title="Atlas is waiting for your approval"
            actions={<><Button variant="approve-outline">Deny</Button><Button variant="outline">View plan</Button><Button variant="approve">Approve &amp; continue</Button></>}
          >
            Run <code>make migrate-staging</code> against <code>payments-staging</code> to partition <code>ledger_entries</code>.
          </Banner>
          <Banner tone="blocked" title="Runner offline" actions={<Button variant="outline" icon="refresh">Retry</Button>}>
            Managed runs are paused until the runner reconnects.
          </Banner>
        </div>
        <div className="grid grid-cols-[repeat(auto-fit,minmax(300px,1fr))] gap-4">
          <KpiTile label="Tickets in flight" value="27" context="+4 vs last week" visual={<ProgressBar value={0.45} />} />
          <Card>
            <EmptyState icon="inbox" title="Nothing needs you" actions={<Button variant="outline">View tickets</Button>} className="py-6">
              Approvals, mentions and dispatch requests will show up here.
            </EmptyState>
          </Card>
          <Card title="Loading">
            <div aria-busy="true" className="flex flex-col gap-3">
              {[0.9, 0.7, 0.8].map((w, i) => (
                <div key={i} className="flex h-6 items-center gap-3">
                  <Skeleton className="size-3.5 rounded-full" />
                  <Skeleton className="h-2.5" style={{ width: `${w * 100}%` }} />
                </div>
              ))}
            </div>
          </Card>
        </div>
      </Section>
    </div>
  );
}
