import { Suspense } from "react";
import { AgentScreen } from "./agent-screen";

async function Loader({ params }: { params: PageProps<"/agents/[handle]">["params"] }) {
  const { handle } = await params;
  return <AgentScreen handle={decodeURIComponent(handle).replace(/^@/, "").toLowerCase()} />;
}

export default function AgentPage({ params }: PageProps<"/agents/[handle]">) {
  return (
    <Suspense>
      <Loader params={params} />
    </Suspense>
  );
}
