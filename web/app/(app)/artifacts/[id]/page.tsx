import { Suspense } from "react";
import { ArtifactScreen } from "./artifact-screen";

async function Loader({ params }: { params: PageProps<"/artifacts/[id]">["params"] }) {
  const { id } = await params;
  return <ArtifactScreen id={id} />;
}

export default function ArtifactPage({ params }: PageProps<"/artifacts/[id]">) {
  return (
    <Suspense>
      <Loader params={params} />
    </Suspense>
  );
}
