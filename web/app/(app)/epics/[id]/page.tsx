import { Suspense } from "react";
import { EpicEntry } from "../../work/epic-entry";

async function Loader({ params }: { params: PageProps<"/epics/[id]">["params"] }) {
  const { id } = await params;
  return <EpicEntry epicId={decodeURIComponent(id)} />;
}

export default function EpicPage({ params }: PageProps<"/epics/[id]">) {
  // params are request-time data under Cache Components, so they stream in Suspense.
  return (
    <Suspense>
      <Loader params={params} />
    </Suspense>
  );
}
