import { Suspense } from "react";
import { RunEntry } from "../../work/run-entry";

async function Loader({ params }: { params: PageProps<"/runs/[id]">["params"] }) {
  const { id } = await params;
  return <RunEntry id={id} />;
}

export default function RunPage({ params }: PageProps<"/runs/[id]">) {
  return (
    <Suspense>
      <Loader params={params} />
    </Suspense>
  );
}
