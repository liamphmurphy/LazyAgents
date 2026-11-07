"use client";

import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { useState, type FormEvent } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, unwrap, contentUrl, type Schemas } from "@/lib/api/client";
import { useBootstrap, useResolved } from "@/lib/api/hooks";
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
  const [agent, setAgent] = useState(data.agents[0]?.id || "");
  const [goal, setGoal] = useState("");
  const selected = data.agents.find((a) => a.id === agent);
  const epic = search.get("epic") || undefined,
    artifact = search.get("artifact") || undefined;
  const create = useMutation({
    mutationFn: () =>
      unwrap(
        api.POST("/planning-sessions", {
          body: {
            project,
            agent_id: agent,
            goal,
            epic_id: epic,
            artifact_id: artifact,
          },
        }),
      ),
    onSuccess: (p) => {
      void qc.invalidateQueries({ queryKey: ["planning"] });
      router.push(`/plans?id=${p.id}`);
    },
  });
  function submit(e: FormEvent) {
    e.preventDefault();
    create.mutate();
  }
  return (
    <Card title="Plan with an agent">
      <form onSubmit={submit} className="space-y-5">
        <p className="text-sm text-fg-secondary">
          Turn a goal or source document into draft tickets. You review the
          proposals before any tickets are created.
        </p>
        <div className="grid gap-4 sm:grid-cols-2">
          <div className="flex flex-col gap-1.5">
            <span className="text-[12.5px] font-medium text-fg-2">Project</span>
            <span className="flex h-[38px] items-center gap-2 rounded-md border border-border-strong bg-surface-sunken px-2.5 text-sm">
              <span aria-hidden className="size-2.5 flex-none rounded-[3px]" style={{ background: scoped.color }} />
              <span className="flex-1 truncate">{scoped.name}</span>
              <span className="font-mono text-xs text-fg-tertiary">{scoped.key}</span>
            </span>
            <span className="text-xs text-fg-tertiary">Switch projects in the sidebar.</span>
          </div>
          <Field label="Planning agent">
            <select
              className={control}
              aria-label="Planning agent"
              value={agent}
              onChange={(e) => setAgent(e.target.value)}
            >
              {data.agents.map((a) => (
                <option key={a.id} value={a.id}>
                  {a.name}
                </option>
              ))}
            </select>
          </Field>
        </div>
        {selected && <Runtime handle={selected.handle} project={project} />}
        {epic && (
          <p className="text-sm text-fg-secondary">
            Epic: {data.epics.find((e) => e.id === epic)?.name || epic}
          </p>
        )}
        {artifact && (
          <ButtonLink variant="ghost" href={`/artifacts/${artifact}`}>
            View source document
          </ButtonLink>
        )}
        <Field label={artifact || epic ? "Additional goal (optional)" : "Goal"}>
          <textarea
            className={control}
            aria-label="Goal"
            rows={5}
            maxLength={20000}
            value={goal}
            onChange={(e) => setGoal(e.target.value)}
            required={!artifact && !epic}
            placeholder="What should this plan accomplish?"
          />
        </Field>
        <Banner tone="human" title="Approve a restricted planning run">
          The planner can read context and submit proposals. Project setup,
          repository cloning, write credentials and extra MCP servers are
          excluded. Starting a plan may incur model costs. It waits in the queue
          if no runner is online.
        </Banner>
        {create.error && (
          <p role="alert" className="text-sm text-pill-blocked-fg">
            {message(create.error)}
          </p>
        )}
        <Button
          type="submit"
          variant="approve"
          disabled={create.isPending || !agent}
        >
          {create.isPending ? "Starting…" : "Approve & start planning"}
        </Button>
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
            <ButtonLink variant="outline" href={`/epics?epic=${plan.epic_id}`}>
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

function PlanDetail({ plan, data }: { plan: Plan; data: Bootstrap }) {
  return (
    <div className="space-y-5">
      <Card
        title={plan.state === "planning" ? "Planning run" : "Plan review"}
        action={
          <ButtonLink href={`/runs/${plan.run_id}`} variant="outline">
            View run log
          </ButtonLink>
        }
      >
        <div className="space-y-3">
          <h2 className="type-heading whitespace-pre-wrap break-words">
            {plan.goal}
          </h2>
          <p className="font-mono text-xs text-fg-tertiary">
            {plan.project} · {plan.harness} · {plan.model} ·{" "}
            {plan.state === "planning" ? plan.run_state : plan.state}
          </p>
          {plan.artifact_id && (
            <ButtonLink href={`/artifacts/${plan.artifact_id}`} variant="ghost">
              Source document
            </ButtonLink>
          )}
          {plan.source_version_id && plan.artifact_id && (
            <ButtonLink
              variant="ghost"
              href={contentUrl(plan.artifact_id, plan.source_version_id)}
            >
              View the source version used by the planner
            </ButtonLink>
          )}
          {plan.state === "planning" && (
            <p role="status" className="text-sm text-fg-secondary">
              {["failed", "cancelled", "blocked"].includes(plan.run_state)
                ? plan.outcome || "Planning stopped. Start a new plan to retry."
                : "The planner will submit draft tickets here. No tickets have been created."}
            </p>
          )}
        </div>
      </Card>
      {plan.state !== "planning" && (
        <Review key={plan.id} plan={plan} data={data} />
      )}
    </div>
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
            <h2 className="type-micro mb-3 text-fg-tertiary">RECENT PLANS</h2>
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
              <Link
                key={p.id}
                href={`/plans?id=${p.id}`}
                className={cx(
                  "block rounded-md border border-border px-3 py-2 hover:bg-hover",
                  p.id === id && "bg-selected",
                )}
              >
                <span className="line-clamp-2 text-sm">{p.goal}</span>
                <span className="mt-1 block text-xs text-fg-tertiary">
                  {p.project} · {p.state === "planning" ? p.run_state : p.state}
                </span>
              </Link>
            ))}
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
              ) : bootstrap.data.agents.length ? (
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
                  Planning needs an active project and at least one agent.
                </EmptyState>
              ))}
          </main>
        </div>
      </Content>
    </>
  );
}
