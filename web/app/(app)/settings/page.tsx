import { ButtonLink } from "@/components/ui/button";
import { Card } from "@/components/ui/primitives";
import { Content, TopBar } from "@/components/shell/topbar";
import { ThemeSwitch } from "@/components/shell/theme";
import { WorkspaceRunner } from "./workspace-runner";
import { SecretsCard } from "./secrets-card";

export default function Settings() {
  return (
    <>
      <TopBar crumbs={[{ label: "Settings" }]} />
      <Content>
        <div className="flex max-w-[var(--reading-max)] flex-col gap-6">
          <h1 className="type-display">Settings</h1>
          <p className="text-sm text-fg-secondary">Everything runs depend on. Each section explains what is needed to start work.</p>
          <nav aria-label="Settings sections" className="flex flex-wrap gap-4 text-sm text-accent"><a href="#workspace">Workspace</a><a href="#secrets">Provider secrets</a><a href="/settings/projects">Projects</a><a href="#runner">Runner backends</a><a href="#appearance">Appearance</a></nav>
          <WorkspaceRunner />
          <Card title="Projects & repositories"><p className="mb-3 text-sm text-fg-secondary">Create projects, configure GitHub repositories and branches, or archive finished work.</p><ButtonLink href="/settings/projects" variant="outline">Manage projects</ButtonLink></Card>
          <Card title="Runner environment"><p className="mb-3 text-sm text-fg-secondary">Inspect shared tools, project dependencies, cache availability, and the last workspace setup.</p><ButtonLink href="/settings/runner" variant="outline">View runner environment</ButtonLink></Card>
          <section id="appearance"><Card title="Appearance">
            <div className="flex flex-wrap items-center justify-between gap-4">
              <div className="flex flex-col gap-0.5">
                <span className="font-medium">Theme</span>
                <span className="type-small text-fg-tertiary">System follows your OS setting.</span>
              </div>
              <ThemeSwitch />
            </div>
          </Card></section>
          <h2 id="secrets" className="type-title mt-2">Provider secrets</h2>
          <SecretsCard />
          <Card title="Project overrides"><p className="mb-3 text-sm text-fg-secondary">Add or remove skills per agent and project. Removed skills stop being injected; added skills extend the agent’s current definition. Without an override, the agent uses its defaults.</p><ButtonLink href="/agents" variant="outline">Manage agent overrides</ButtonLink></Card>
        </div>
      </Content>
    </>
  );
}
