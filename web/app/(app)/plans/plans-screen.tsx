"use client";

import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { useMemo, useRef, useState, type FormEvent } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, unwrap, contentUrl, type LinkTarget, type Schemas } from "@/lib/api/client";
import { useBootstrap, useChatRunEvents, useResolved } from "@/lib/api/hooks";
import { Button, ButtonLink } from "@/components/ui/button";
import {
  Banner,
  Card,
  EmptyState,
  Field,
  Skeleton,
  TextInput,
} from "@/components/ui/primitives";
import { Content, TopBar } from "@/components/shell/topbar";
import { ProjectScopeState, projectCrumbs, useProjectScope } from "@/components/shell/project-scope";
import { DispatchDialog } from "@/components/domain/dispatch-dialog";
import { LinkEditor } from "@/components/domain/link-editor";
import { Pill, SmallPill } from "@/components/domain/labels";
import { Icon } from "@/components/ui/icon";
import { AddContext, ChipList, SentChips, acceptTarget } from "@/app/(app)/chat/_components/context-chips";
import { ActivityRef } from "@/app/(app)/chat/_components/activity";
import {
  MAX_CONTEXT_REFS,
  addChip,
  chipFromRef,
  chipFromTarget,
  parseAttach,
  removeChip,
  toContextRefs,
  type Chip,
  type ContextRef,
} from "@/lib/chat-context";
import { firstLine, planCount, planLive, planStatus, readSources } from "@/lib/plans";
import { ago } from "@/lib/format";
import { cx } from "@/lib/cx";

type Plan = Schemas["PlanningSession"];
type Proposal = Schemas["TicketProposal"];
type Bootstrap = Schemas["Bootstrap"];
type Project = Schemas["Project"];
const control =
  "w-full rounded-md border border-border-strong bg-surface px-2.5 py-2 text-sm text-fg";
const message = (e: unknown) =>
  e instanceof Error ? e.message : "Something went wrong. Please try again.";

function Runtime({ handle, project }: { handle: string; project: string }) {
  const q = useResolved(handle, project);
  if (q.isLoading) return <Skeleton className="h-10" />;
  if (q.isError) return <p role="alert">Couldn’t load the agent runtime.</p>;
  return (
    <p className="font-mono text-xs text-fg-secondary">
      {q.data?.harness} · {q.data?.model}
    </p>
  );
}

/** project comes from the sidebar switcher (?project=, then the remembered one). */
function NewPlan({ data, project: scoped }: { data: Bootstrap; project: Project }) {
  const search = useSearchParams(),
    router = useRouter(),
    qc = useQueryClient();
  const project = scoped.key;
  // Planner agents are main agents: sub agents only run when delegated to.
  const planners = data.agents.filter((a) => a.kind === "main");
  const [agent, setAgent] = useState(planners[0]?.id || "");
  const [goal, setGoal] = useState("");
  // "Plan with agent" links arrive with ?attach=, ?epic= or ?artifact= (a PRD/SPEC whose version is pinned).
  const [chips, setChips] = useState<Chip[]>(() => parseAttach(search.getAll("attach"), search.get("attach_title")));
  const epics = data.epics.filter((e) => e.project_id === scoped.id && e.status === "open");
  const [epic, setEpic] = useState(() => {
    const id = search.get("epic");
    return id && data.epics.some((e) => e.id === id && e.project_id === scoped.id) ? id : "";
  });
  const [artifact, setArtifact] = useState(search.get("artifact") || undefined);
  const area = useRef<HTMLTextAreaElement | null>(null);
  const selected = planners.find((a) => a.id === agent);
  const attach = (t: LinkTarget) => {
    const chip = chipFromTarget(t);
    if (chip) setChips((cs) => addChip(cs, chip));
  };
  const create = useMutation({
    mutationFn: () =>
      unwrap(
        api.POST("/planning-sessions", {
          body: {
            project,
            agent_id: agent,
            goal,
            epic_id: epic || undefined,
            artifact_id: artifact,
            context_refs: toContextRefs(chips),
          },
        }),
      ),
    onSuccess: (p) => {
      void qc.invalidateQueries({ queryKey: ["planning"] });
      router.push(`/plans?id=${p.id}`);
    },
  });
  const needsGoal = !artifact && !epic;
  const ready = !!agent && (!needsGoal || !!goal.trim()) && !create.isPending;
  function submit(e?: FormEvent) {
    e?.preventDefault();
    if (ready) create.mutate();
  }
  return (
    <Card
      title="New plan"
      action={
        <span className="flex h-6 items-center gap-2 rounded-md border border-border-strong bg-surface-sunken px-2 text-xs" title="Switch projects in the sidebar">
          <span aria-hidden className="size-2 flex-none rounded-[2px]" style={{ background: scoped.color }} />
          <span className="max-w-[140px] truncate">{scoped.name}</span>
          <span className="font-mono text-[10.5px] text-fg-tertiary">{scoped.key}</span>
        </span>
      }
    >
      <form onSubmit={submit} className="space-y-5" aria-label="New plan">
        <p className="text-sm text-fg-secondary">
          Describe the goal and attach what the planner should read. You review
          proposals before any tickets exist.
        </p>
        <div className="flex flex-col gap-1.5">
          <label htmlFor="plan-goal" className="text-[12.5px] font-medium text-fg-2">
            {needsGoal ? "Goal" : "Additional goal (optional)"}
          </label>
          <div className="flex flex-col gap-2 rounded-lg border border-border-strong bg-surface-sunken p-2 focus-within:border-accent">
            <LinkEditor
              id="plan-goal"
              inputRef={(el) => {
                area.current = el;
              }}
              mono={false}
              aria-label="Goal"
              rows={8}
              maxLength={20000}
              value={goal}
              onChange={setGoal}
              onPick={attach}
              accept={acceptTarget}
              disabled={create.isPending}
              placeholder="What should this plan accomplish? Type [[ to attach wiki pages, artifacts and tickets."
              className="!border-0 !bg-transparent !p-1 focus-visible:!outline-none"
              onKeyDown={(e) => {
                if (e.key === "Enter" && (e.metaKey || e.ctrlKey) && !e.nativeEvent.isComposing) {
                  e.preventDefault();
                  submit();
                }
              }}
            />
            {(artifact || chips.length > 0) && (
              <div className="flex flex-wrap items-center gap-1.5 px-1">
                {artifact && (
                  <span className="inline-flex h-6 items-center gap-1.5 rounded-md border border-border bg-surface px-2 text-xs text-fg-secondary" data-testid="plan-source">
                    <Link href={`/artifacts/${artifact}`} className="hover:text-fg">Source document (pinned version)</Link>
                    <button
                      type="button"
                      aria-label="Remove source document"
                      onClick={() => setArtifact(undefined)}
                      className="grid size-4 cursor-pointer place-items-center rounded-xs text-fg-tertiary hover:bg-hover hover:text-fg"
                    >
                      ×
                    </button>
                  </span>
                )}
                <ChipList chips={chips} onRemove={(k) => setChips((cs) => removeChip(cs, k))} disabled={create.isPending} />
              </div>
            )}
            <div className="flex flex-wrap items-center gap-2 border-t border-border px-1 pt-2">
              <AddContext onPick={attach} onClosed={() => area.current?.focus()} disabled={create.isPending || chips.length >= MAX_CONTEXT_REFS} />
              <span className="text-xs text-fg-tertiary">
                or type [[ to attach wiki pages, artifacts and tickets · {chips.length} of {MAX_CONTEXT_REFS}
              </span>
              <span className="flex-1" />
              <span className="text-xs text-fg-tertiary">⌘↵ to start</span>
            </div>
          </div>
        </div>
        <div className="grid gap-4 sm:grid-cols-2">
          <Field label="Planner agent">
            <select
              className={control}
              aria-label="Planner agent"
              value={agent}
              onChange={(e) => setAgent(e.target.value)}
            >
              {planners.map((a) => (
                <option key={a.id} value={a.id}>
                  {a.name} · @{a.handle}
                </option>
              ))}
            </select>
            {selected && <Runtime handle={selected.handle} project={project} />}
          </Field>
          <Field
            label="Epic (optional)"
            hint={epic ? "Accepted tickets join this epic." : "Accepting creates a new epic. Pick one to extend it instead."}
          >
            <select className={control} aria-label="Epic" value={epic} onChange={(e) => setEpic(e.target.value)}>
              <option value="">No epic</option>
              {epics.map((e) => (
                <option key={e.id} value={e.id}>
                  {e.name}
                </option>
              ))}
            </select>
          </Field>
        </div>
        <Banner tone="human" title="Approve a restricted planning run">
          The planner can read context and submit proposals. Repository writes,
          setup, credentials and extra MCP servers are excluded. Starting a plan
          may incur model costs; it waits in the queue if no runner is online.
        </Banner>
        {create.error && (
          <p role="alert" className="text-sm text-pill-blocked-fg">
            {message(create.error)}
          </p>
        )}
        <div className="flex flex-wrap items-center gap-3">
          <Button type="submit" variant="approve" disabled={!ready}>
            {create.isPending ? "Starting…" : "Approve & start planning"}
          </Button>
          <span className="text-xs text-fg-tertiary">The planner gets the attached documents when its run starts.</span>
        </div>
      </form>
    </Card>
  );
}

function ExistingBlockers({
  initial,
  onChange,
}: {
  initial: string[];
  onChange: (keys: string[]) => void;
}) {
  const [text, setText] = useState(initial.join(", "));
  return (
    <Field
      label="Existing blocker tickets"
      hint="Optional ticket keys, separated by commas. These tickets must also be Done."
    >
      <TextInput
        aria-label="Existing blocker tickets"
        value={text}
        placeholder="PAY-214, PAY-224"
        onChange={(e) => {
          setText(e.target.value);
          onChange(
            e.target.value
              .split(/[,\s]+/)
              .filter(Boolean)
              .map((key) => key.toUpperCase()),
          );
        }}
      />
    </Field>
  );
}

function Review({ plan, data }: { plan: Plan; data: Bootstrap }) {
  const qc = useQueryClient();
  const [items, setItems] = useState<Proposal[]>(() =>
    plan.proposals.map((p, i) => ({ ...p, ref: p.ref || `ticket-${i + 1}` })),
  );
  const [revision, setRevision] = useState(plan.revision);
  const [dispatch, setDispatch] = useState(false);
  const review = useMutation({
    mutationFn: (action: "save" | "accept" | "dismiss") =>
      unwrap(
        api.PATCH("/planning-sessions/{id}", {
          params: { path: { id: plan.id } },
          body: { revision, action, proposals: items },
        }),
      ),
    onSuccess: (p) => {
      setRevision(p.revision);
      setItems(p.proposals);
      qc.setQueryData(["planning", plan.id], p);
      void qc.invalidateQueries({ queryKey: ["planning"] });
      void qc.invalidateQueries({ queryKey: ["tickets"] });
      void qc.invalidateQueries({ queryKey: ["bootstrap"] });
    },
  });
  const edit = (index: number, patch: Partial<Proposal>) =>
    setItems(items.map((p, i) => (i === index ? { ...p, ...patch } : p)));
  if (plan.state === "accepted") {
    const assigned = plan.ready_keys;
    return (
      <Card title="Tickets created">
        <div className="space-y-4">
          <p className="text-sm text-fg-secondary">
            Assigned tickets whose blockers are Done are ready now.
            Dependent tickets can be dispatched too: they queue and start
            once their blockers are Done.
          </p>
          {plan.epic_id && (
            <ButtonLink variant="outline" href={`/epics?epic=${plan.epic_id}&project=${encodeURIComponent(plan.project)}`}>
              View epic
            </ButtonLink>
          )}
          {plan.ticket_keys.map((key, i) => (
            <Link
              key={key}
              className="block text-sm text-accent"
              href={`/tickets/${key}`}
            >
              {key} · {plan.proposals[i]?.title}
            </Link>
          ))}
          <ButtonLink
            variant="outline"
            href={`/tickets?project=${plan.project}&view=all&ready=true`}
          >
            View ready tickets
          </ButtonLink>
          {assigned.length > 0 && (
            <Button variant="secondary" onClick={() => setDispatch(true)}>
              Review & dispatch {assigned.length} ready tickets…
            </Button>
          )}
          <DispatchDialog
            keys={assigned}
            open={dispatch}
            onOpenChange={setDispatch}
          />
        </div>
      </Card>
    );
  }
  if (plan.state === "dismissed")
    return (
      <EmptyState icon="epics" title="Plan dismissed">
        No tickets were created.
      </EmptyState>
    );
  return (
    <div className="space-y-4">
      {plan.revision !== revision && (
        <Banner tone="human" title="This draft has changed">
          Your edits are preserved. Reload the page to review the latest draft
          before saving.
        </Banner>
      )}
      {data.projects.find((p) => p.key === plan.project)?.archived && (
        <Banner tone="human" title="This project is archived">
          Restore the project in Settings before accepting these proposals.
        </Banner>
      )}
      <p className="text-sm text-fg-secondary">
        Edit, reassign or drop proposals. To merge two, combine their text and
        criteria in one proposal, then drop the other.
      </p>
      {items.map((item, index) => (
        <Card
          key={item.ref}
          title={`Ticket ${index + 1}`}
          action={
            <Button
              variant="ghost"
              disabled={
                review.isPending ||
                items.some(
                  (p, i) => i !== index && p.depends_on?.includes(item.ref!),
                )
              }
              title="Remove dependencies on this proposal before dropping it"
              onClick={() => setItems(items.filter((_, i) => i !== index))}
            >
              Drop
            </Button>
          }
        >
          <div className="space-y-3">
            <Field label="Title">
              <TextInput
                aria-label={`Ticket ${index + 1} title`}
                value={item.title}
                maxLength={300}
                onChange={(e) => edit(index, { title: e.target.value })}
              />
            </Field>
            <Field label="Description">
              <textarea
                className={control}
                aria-label={`Ticket ${index + 1} description`}
                rows={5}
                value={item.description}
                onChange={(e) => edit(index, { description: e.target.value })}
              />
            </Field>
            <Field label="Acceptance criteria (one per line)">
              <textarea
                className={control}
                aria-label={`Ticket ${index + 1} criteria`}
                rows={3}
                value={item.criteria.join("\n")}
                onChange={(e) =>
                  edit(index, { criteria: e.target.value.split("\n") })
                }
              />
            </Field>
            <Field
              label="Waits for these proposals"
              hint="Blockers must be Done before this ticket can be dispatched."
            >
              {items.length < 2 && (
                <p className="text-xs text-fg-tertiary">No other proposals.</p>
              )}
              {items.map((other, i) =>
                i === index ? null : (
                  <label
                    key={other.ref}
                    className="flex items-start gap-2 text-sm"
                  >
                    <input
                      type="checkbox"
                      className="mt-1 accent-[var(--accent)]"
                      checked={item.depends_on?.includes(other.ref!) ?? false}
                      onChange={(e) =>
                        edit(index, {
                          depends_on: e.target.checked
                            ? [...(item.depends_on ?? []), other.ref!]
                            : (item.depends_on ?? []).filter(
                                (ref) => ref !== other.ref,
                              ),
                        })
                      }
                    />
                    <span>
                      Ticket {i + 1} · {other.title || "Untitled"}
                    </span>
                  </label>
                ),
              )}
            </Field>
            <ExistingBlockers
              initial={item.blocked_by ?? []}
              onChange={(blocked_by) => edit(index, { blocked_by })}
            />
            <Field label="Assign to">
              <select
                className={control}
                aria-label={`Ticket ${index + 1} agent`}
                value={item.agent_id || ""}
                onChange={(e) =>
                  edit(index, { agent_id: e.target.value || undefined })
                }
              >
                <option value="">Unassigned</option>
                {data.agents.map((a) => (
                  <option key={a.id} value={a.id}>
                    {a.name}
                  </option>
                ))}
              </select>
            </Field>
            <label className="flex items-center gap-2 text-sm text-fg-secondary">
              <input
                type="checkbox"
                checked={!!item.requires_mockup}
                onChange={(e) =>
                  edit(index, { requires_mockup: e.target.checked })
                }
              />
              Requires mockup
            </label>
          </div>
        </Card>
      ))}
      {review.error && (
        <p role="alert" className="text-sm text-pill-blocked-fg">
          {message(review.error)}
        </p>
      )}
      <div className="flex flex-wrap gap-2">
        <Button
          variant="outline"
          disabled={review.isPending || items.length >= 50}
          onClick={() =>
            setItems([
              ...items,
              {
                ref: crypto.randomUUID(),
                title: "",
                description: "",
                criteria: [],
                depends_on: [],
              },
            ])
          }
        >
          Add proposal
        </Button>
        <Button
          variant="ghost"
          disabled={review.isPending}
          onClick={() => review.mutate("dismiss")}
        >
          Dismiss plan
        </Button>
        <Button
          variant="secondary"
          disabled={review.isPending || !items.length}
          onClick={() => review.mutate("save")}
        >
          Save draft
        </Button>
        <Button
          variant="primary"
          disabled={review.isPending || !items.length}
          onClick={() => review.mutate("accept")}
        >
          Accept & create {items.length} tickets{plan.epic_id ? "" : " + epic"}
        </Button>
      </div>
    </div>
  );
}

/** What the planner has read, from its run's tool calls (polled while live). */
function SourcesRead({ plan, live }: { plan: Plan; live: boolean }) {
  const events = useChatRunEvents(plan.run_id);
  const sources = useMemo(() => readSources(events.data), [events.data]);
  if (!live && sources.length === 0) return null;
  const list = (
    <ul className="flex flex-col gap-1.5" data-testid="plan-sources">
      {sources.map((src) => (
        <li key={src.key} className="flex min-w-0 items-center gap-2 text-[12.5px] text-fg-secondary">
          {src.done ? (
            <Icon name="check" size={12} className="flex-none text-pill-done-fg" />
          ) : (
            <span aria-hidden className="size-3 flex-none animate-spin rounded-full border-2 border-border-strong border-t-accent" />
          )}
          <span className="min-w-0 truncate">
            {src.activity.verb}{" "}
            {src.activity.ref && src.activity.label ? (
              <ActivityRef refText={src.activity.ref} label={src.activity.label} />
            ) : (
              <span className="text-fg">{src.activity.text}</span>
            )}
          </span>
        </li>
      ))}
    </ul>
  );
  if (!live)
    return (
      <details className="rounded-lg border border-border bg-surface px-3 py-2">
        <summary className="cursor-pointer text-[12.5px] text-fg-secondary">Sources read ({sources.length})</summary>
        <div className="mt-2">{list}</div>
      </details>
    );
  return (
    <section aria-label="Sources read so far" className="space-y-2">
      <h3 className="type-micro text-fg-tertiary">SOURCES READ SO FAR</h3>
      {sources.length === 0 ? (
        <p role="status" className="text-[12.5px] text-fg-tertiary">
          {plan.run_state === "queued" ? "Waiting for a runner…" : "The planner hasn’t read anything yet."}
        </p>
      ) : (
        list
      )}
    </section>
  );
}

function PlanDetail({ plan, data }: { plan: Plan; data: Bootstrap }) {
  const live = planLive(plan);
  const status = planStatus(plan);
  const attached = plan.context_refs.map((r) => chipFromRef(r as ContextRef));
  const epicName = plan.epic_id && data.epics.find((e) => e.id === plan.epic_id)?.name;
  return (
    <div className="space-y-5">
      <Card
        title={live ? "Planning run" : "Plan"}
        action={
          <ButtonLink href={`/runs/${plan.run_id}`} variant="outline">
            View run log
          </ButtonLink>
        }
      >
        <div className="space-y-4">
          <div className="flex items-center gap-2">
            <Pill tone={status.tone}>{status.label}</Pill>
            <span className="font-mono text-xs text-fg-tertiary">
              {plan.project} · {plan.harness} · {plan.model}
            </span>
          </div>
          <h2 className="type-heading whitespace-pre-wrap break-words">
            {plan.goal}
          </h2>
          {epicName && <p className="text-sm text-fg-secondary">Epic: {epicName}</p>}
          {(plan.artifact_id || attached.length > 0) && (
            <div className="flex flex-wrap items-center gap-1.5">
              {plan.artifact_id && (
                <ButtonLink
                  href={plan.source_version_id ? contentUrl(plan.artifact_id, plan.source_version_id) : `/artifacts/${plan.artifact_id}`}
                  variant="ghost"
                >
                  Source document
                </ButtonLink>
              )}
              <SentChips chips={attached} />
            </div>
          )}
          {plan.state === "planning" && (
            <p role="status" className="text-sm text-fg-secondary">
              {live
                ? "The planner will submit draft tickets here. No tickets have been created."
                : plan.outcome || "Planning stopped. Start a new plan to retry."}
            </p>
          )}
          {plan.state === "review" && (
            <Banner tone="human" title={`${plan.proposals.length} proposals ready for review`}>
              <a href="#proposal-review" className="text-accent hover:underline">
                Review the proposals
              </a>{" "}
              below; no tickets exist until you accept them.
            </Banner>
          )}
          <SourcesRead plan={plan} live={live} />
        </div>
      </Card>
      {plan.state !== "planning" && (
        <section id="proposal-review" aria-label="Proposal review" className="scroll-mt-4">
          <Review key={plan.id} plan={plan} data={data} />
        </section>
      )}
    </div>
  );
}

function PlanRow({ plan, active }: { plan: Plan; active: boolean }) {
  const status = planStatus(plan);
  return (
    <Link
      href={`/plans?id=${plan.id}`}
      aria-current={active ? "page" : undefined}
      className={cx(
        "block rounded-md border border-border px-3 py-2.5 hover:bg-hover",
        active && "bg-selected",
      )}
      data-testid="plan-row"
    >
      <span className="flex items-center gap-2">
        <SmallPill tone={status.tone}>{status.label}</SmallPill>
        <span className="flex-1" />
        <span className="text-[11px] text-fg-tertiary">{planCount(plan)}</span>
      </span>
      <span className="mt-1.5 block truncate text-sm">{firstLine(plan.goal)}</span>
      <span className="mt-0.5 block text-xs text-fg-tertiary">
        {plan.project} · {ago(plan.created_at)}
      </span>
    </Link>
  );
}

export function PlansScreen() {
  const search = useSearchParams();
  const id = search.get("id");
  const bootstrap = useBootstrap();
  // A new plan needs one project; an open plan (?id=) carries its own.
  const scope = useProjectScope("required", { skip: !!id });
  const scoped = scope.status === "ready" ? scope.project : undefined;
  const list = useQuery({
    queryKey: ["planning", "list"],
    queryFn: () => unwrap(api.GET("/planning-sessions")),
    refetchInterval: 15000,
  });
  const detail = useQuery({
    queryKey: ["planning", id],
    enabled: !!id,
    queryFn: () =>
      unwrap(
        api.GET("/planning-sessions/{id}", { params: { path: { id: id! } } }),
      ),
    refetchInterval: 5000,
  });
  return (
    <>
      <TopBar
        crumbs={projectCrumbs(
          id ? bootstrap.data?.projects.find((p) => p.key === detail.data?.project) : scoped,
          "Plans",
        )}
        actions={
          <ButtonLink href="/plans" variant="outline">
            New plan
          </ButtonLink>
        }
      />
      <Content>
        <div className="grid min-w-0 gap-6 lg:grid-cols-[240px_minmax(0,1fr)]">
          <aside className="space-y-2">
            <h2 className="type-micro mb-3 flex text-fg-tertiary">
              <span className="flex-1">PLANS</span>
              {list.data && <span>{list.data.length}</span>}
            </h2>
            {list.isLoading && <Skeleton className="h-24" />}
            {list.isError && (
              <p role="alert" className="text-sm text-pill-blocked-fg">
                Couldn’t load plans.
              </p>
            )}
            {list.data?.length === 0 && (
              <p className="text-sm text-fg-tertiary">
                Your plans will appear here.
              </p>
            )}
            {list.data?.map((p) => (
              <PlanRow key={p.id} plan={p} active={p.id === id} />
            ))}
            {!!list.data?.length && (
              <p className="pt-1 text-[11.5px] text-fg-tertiary">
                Status · first line of the goal · tickets accepted / proposals made
              </p>
            )}
          </aside>
          <main className="min-w-0 max-w-[900px]">
            {(bootstrap.isLoading || (id && detail.isLoading)) && (
              <Skeleton className="h-64" />
            )}
            {(bootstrap.isError || (id && detail.isError)) && (
              <Banner tone="blocked" title="Couldn’t load this plan">
                {message(detail.error || bootstrap.error)}
              </Banner>
            )}
            {bootstrap.data &&
              (id ? (
                detail.data && (
                  <PlanDetail
                    key={id}
                    plan={detail.data}
                    data={bootstrap.data}
                  />
                )
              ) : !scoped ? (
                <ProjectScopeState base="/plans" scope={scope} />
              ) : bootstrap.data.agents.some((a) => a.kind === "main") ? (
                <NewPlan key={search.toString()} data={bootstrap.data} project={scoped} />
              ) : (
                <EmptyState
                  icon="epics"
                  title="Create an agent first"
                  actions={
                    <ButtonLink href="/agents" variant="outline">
                      Go to agents
                    </ButtonLink>
                  }
                >
                  Planning needs an active project and at least one main agent.
                </EmptyState>
              ))}
          </main>
        </div>
      </Content>
    </>
  );
}
