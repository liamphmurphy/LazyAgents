"use client";
import Link from "next/link";
import { useState, type FormEvent } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, unwrap, type Schemas } from "@/lib/api/client";
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

type Project = Schemas["Project"];
const errorText = (e: unknown) =>
  e instanceof Error ? e.message : "Couldn’t save the project.";
function ProjectForm({
  project,
  onDone,
}: {
  project?: Project;
  onDone: () => void;
}) {
  const qc = useQueryClient();
  const [revision] = useState(project?.revision ?? 1);
  const [key, setKey] = useState(project?.key ?? "");
  const [name, setName] = useState(project?.name ?? "");
  const [color, setColor] = useState(project?.color ?? "#7c8aff");
  const [repo, setRepo] = useState(project?.repo_url ?? "");
  const [branch, setBranch] = useState(project?.default_branch ?? "main");
  const save = useMutation({
    mutationFn: (archived: boolean) => {
      const config = {
        name,
        color,
        repo_url: repo.trim(),
        default_branch: branch.trim(),
      };
      return project
        ? unwrap(
            api.PUT("/projects/{key}", {
              params: { path: { key: project.key } },
              body: { ...config, revision, archived },
            }),
          )
        : unwrap(api.POST("/projects", { body: { ...config, key } }));
    },
    onSuccess: () => {
      for (const prefix of [
        "projects",
        "bootstrap",
        "tickets",
        "dashboard",
        "epics",
        "planning",
        "inbox",
        "agents",
        "agent-overviews",
      ])
        void qc.invalidateQueries({ queryKey: [prefix] });
      onDone();
    },
  });
  function submit(e: FormEvent) {
    e.preventDefault();
    save.mutate(project?.archived ?? false);
  }
  return (
    <form onSubmit={submit} className="space-y-4">
      <div className="grid gap-4 sm:grid-cols-2">
        <Field
          label="Project key"
          htmlFor={`project-key-${project?.id ?? "new"}`}
          hint="2–10 letters or digits. Keys stay fixed to preserve ticket IDs."
        >
          <TextInput
            id={`project-key-${project?.id ?? "new"}`}
            mono
            required
            maxLength={10}
            value={key}
            disabled={!!project}
            onChange={(e) => setKey(e.target.value.toUpperCase())}
          />
        </Field>
        <Field label="Name" htmlFor={`project-name-${project?.id ?? "new"}`}>
          <TextInput
            id={`project-name-${project?.id ?? "new"}`}
            required
            maxLength={100}
            value={name}
            onChange={(e) => setName(e.target.value)}
          />
        </Field>
      </div>
      <Field label="Color">
        <input
          aria-label="Project color"
          type="color"
          value={color}
          onChange={(e) => setColor(e.target.value)}
          className="h-9 w-16 rounded-md border border-border-strong bg-surface"
        />
      </Field>
      <Field
        label="GitHub repository"
        hint="Optional HTTPS URL. Leave empty for an empty run workspace. Store access tokens in Connections & secrets."
      >
        <TextInput
          aria-label="GitHub repository"
          type="url"
          placeholder="git@github.com:your-team/repository.git"
          value={repo}
          onChange={(e) => setRepo(e.target.value)}
        />
      </Field>
      <Field
        label="Default branch"
        hint="New runs clone this branch. Approved runs keep the repository settings they were dispatched with."
      >
        <TextInput
          aria-label="Default branch"
          mono
          required
          value={branch}
          maxLength={200}
          onChange={(e) => setBranch(e.target.value)}
        />
      </Field>
      {project && project.revision !== revision && (
        <Banner tone="human" title="This project changed">
          Your edits are preserved. Close and reopen the editor to review the
          latest settings before saving.
        </Banner>
      )}
      {save.error && (
        <p role="alert" className="text-sm text-pill-blocked-fg">
          {errorText(save.error)}
        </p>
      )}
      <div className="flex flex-wrap gap-2">
        <Button type="submit" variant="primary" disabled={save.isPending}>
          {project ? "Save changes" : "Create project"}
        </Button>
        <Button variant="ghost" disabled={save.isPending} onClick={onDone}>
          Cancel
        </Button>
        {project && (
          <Button
            variant="outline"
            disabled={save.isPending}
            onClick={() => save.mutate(!project.archived)}
          >
            {project.archived ? "Restore project" : "Archive project"}
          </Button>
        )}
      </div>
      {project && !project.archived && (
        <p className="text-xs text-fg-tertiary">
          Archiving preserves history and removes the project from active work.
          Finish or cancel queued and active runs first.
        </p>
      )}
    </form>
  );
}

export function ProjectsScreen() {
  const [editing, setEditing] = useState<string | null>(null);
  const [showArchived, setShowArchived] = useState(false);
  const query = useQuery({
    queryKey: ["projects"],
    queryFn: () => unwrap(api.GET("/projects")),
  });
  const shown =
    query.data?.filter((p) => (showArchived ? p.archived : !p.archived)) ?? [];
  return (
    <>
      <TopBar
        crumbs={[
          { label: "Settings", href: "/settings" },
          { label: "Projects & repos" },
        ]}
        actions={
          <Button variant="primary" onClick={() => setEditing("new")}>
            New project
          </Button>
        }
      />
      <Content>
        <div className="max-w-[900px] space-y-5">
          <h1 className="type-display">Projects & repositories</h1>
          <p className="text-sm text-fg-secondary">
            Manage where your agents work. One project maps to one GitHub
            repository.
          </p>
          <ButtonLink href="/settings" variant="outline">
            Connections & secrets
          </ButtonLink>
          {editing === "new" && (
            <Card title="New project">
              <ProjectForm onDone={() => setEditing(null)} />
            </Card>
          )}
          <div className="flex gap-2">
            <Button
              variant={!showArchived ? "secondary" : "ghost"}
              onClick={() => setShowArchived(false)}
            >
              Active
            </Button>
            <Button
              variant={showArchived ? "secondary" : "ghost"}
              onClick={() => setShowArchived(true)}
            >
              Archived
            </Button>
          </div>
          {query.isLoading && <Skeleton className="h-48" />}
          {query.isError && (
            <Banner
              tone="blocked"
              title="Couldn’t load projects"
              actions={<Button onClick={() => query.refetch()}>Retry</Button>}
            >
              {query.error.message}
            </Banner>
          )}
          {query.isSuccess && shown.length === 0 && (
            <EmptyState
              icon="epics"
              title={showArchived ? "No archived projects" : "No projects yet"}
            >
              {showArchived
                ? "Archived projects can be restored here."
                : "Create a project to start tracking work."}
            </EmptyState>
          )}
          {shown.map((p) => (
            <Card
              key={p.id}
              title={
                <span className="flex items-center gap-2">
                  <span
                    aria-hidden
                    className="size-2.5 rounded-sm"
                    style={{ background: p.color }}
                  />
                  {p.name}
                  <span className="font-mono text-xs text-fg-tertiary">
                    {p.key}
                  </span>
                </span>
              }
              action={
                <Button
                  variant="outline"
                  onClick={() => setEditing(editing === p.id ? null : p.id)}
                >
                  {editing === p.id ? "Close" : "Edit"}
                </Button>
              }
            >
              {editing === p.id ? (
                <ProjectForm
                  key={`${p.id}-${editing}`}
                  project={p}
                  onDone={() => setEditing(null)}
                />
              ) : (
                <div className="space-y-2 text-sm">
                  <p className="break-all font-mono text-xs text-fg-secondary">
                    {p.repo_url || "No repository configured"} ·{" "}
                    {p.default_branch}
                  </p>
                  <Link
                    href={`/tickets?project=${p.key}&view=all`}
                    className="text-accent"
                  >
                    View tickets{p.archived ? " & history" : ""} →
                  </Link>
                </div>
              )}
            </Card>
          ))}
        </div>
      </Content>
    </>
  );
}
