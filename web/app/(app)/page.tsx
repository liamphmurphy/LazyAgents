import { Card } from "@/components/ui/primitives";
import { ButtonLink } from "@/components/ui/button";
import { Content, TopBar } from "@/components/shell/topbar";
import { Greeting } from "./greeting";
import { HealthStatus } from "./health-status";

export default function Dashboard() {
  return (
    <>
      <TopBar
        crumbs={[{ label: "Dashboard" }]}
        actions={
          <ButtonLink href="/tickets/new" variant="primary" icon="plus" hint="C">
            New ticket
          </ButtonLink>
        }
      />
      <Content>
        <div className="flex flex-col gap-6">
          <div className="flex flex-col gap-1">
            <Greeting />
            <p className="text-fg-secondary">KPIs, the attention list and live agents arrive in Phase 7.</p>
          </div>
          <Card title="System status" action={<span className="font-mono text-xs text-fg-tertiary">/api/v1/health</span>}>
            <HealthStatus />
          </Card>
        </div>
      </Content>
    </>
  );
}
