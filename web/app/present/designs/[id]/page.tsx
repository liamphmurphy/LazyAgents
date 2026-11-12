import { Suspense } from "react";
import { PresentDesign } from "./present-design";

async function Loader({ params }: { params: PageProps<"/present/designs/[id]">["params"] }) {
  const { id } = await params;
  return <PresentDesign id={id} />;
}

/** A design's prototype played full screen, outside the app shell (ADR-designs §4, the Slides Present shell). */
export default function PresentPage({ params }: PageProps<"/present/designs/[id]">) {
  return (
    <Suspense>
      <Loader params={params} />
    </Suspense>
  );
}
