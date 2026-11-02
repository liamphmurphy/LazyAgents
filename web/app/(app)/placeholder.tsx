import type { IconName } from "@/components/ui/icon";
import { EmptyState } from "@/components/ui/primitives";
import { Content, TopBar } from "@/components/shell/topbar";

/** Stand-in for screens that later phases build (PLAN.md §9). */
export function Placeholder({ title, icon, phase, children }: { title: string; icon: IconName; phase: number; children: string }) {
  return (
    <>
      <TopBar crumbs={[{ label: title }]} />
      <Content>
        <EmptyState icon={icon} title={title}>
          {children} Arrives in Phase {phase}.
        </EmptyState>
      </Content>
    </>
  );
}
