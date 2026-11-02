import { Card } from "@/components/ui/primitives";
import { Content, TopBar } from "@/components/shell/topbar";
import { ThemeSwitch } from "@/components/shell/theme";

export default function Settings() {
  return (
    <>
      <TopBar crumbs={[{ label: "Settings" }]} />
      <Content>
        <div className="flex max-w-[var(--reading-max)] flex-col gap-6">
          <h1 className="type-display">Settings</h1>
          <Card title="Appearance">
            <div className="flex flex-wrap items-center justify-between gap-4">
              <div className="flex flex-col gap-0.5">
                <span className="font-medium">Theme</span>
                <span className="type-small text-fg-tertiary">System follows your OS setting.</span>
              </div>
              <ThemeSwitch />
            </div>
          </Card>
          <p className="type-small text-fg-tertiary">Connections and secrets arrive with the runner in Phase 5.</p>
        </div>
      </Content>
    </>
  );
}
