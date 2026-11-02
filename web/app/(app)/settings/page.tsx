import { ButtonLink } from "@/components/ui/button";
import { Card } from "@/components/ui/primitives";
import { Content, TopBar } from "@/components/shell/topbar";
import { ThemeSwitch } from "@/components/shell/theme";
import { SecretsCard } from "./secrets-card";

export default function Settings() {
  return (
    <>
      <TopBar crumbs={[{ label: "Settings" }]} />
      <Content>
        <div className="flex max-w-[var(--reading-max)] flex-col gap-6">
          <h1 className="type-display">Settings</h1>
          <Card title="Projects & repositories"><p className="mb-3 text-sm text-fg-secondary">Create projects, configure GitHub repositories and branches, or archive finished work.</p><ButtonLink href="/settings/projects" variant="outline">Manage projects</ButtonLink></Card>
          <Card title="Appearance">
            <div className="flex flex-wrap items-center justify-between gap-4">
              <div className="flex flex-col gap-0.5">
                <span className="font-medium">Theme</span>
                <span className="type-small text-fg-tertiary">System follows your OS setting.</span>
              </div>
              <ThemeSwitch />
            </div>
          </Card>
          <h2 className="type-title mt-2">Connections &amp; secrets</h2>
          <SecretsCard />
        </div>
      </Content>
    </>
  );
}
