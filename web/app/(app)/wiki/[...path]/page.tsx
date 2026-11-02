import { Suspense } from "react";
import { WikiScreen } from "../_components/wiki-screen";

// Segments arrive percent-encoded ("Rollback%20runbook").
function decode(s: string) {
  try {
    return decodeURIComponent(s);
  } catch {
    return s;
  }
}

async function Loader({ params }: { params: PageProps<"/wiki/[...path]">["params"] }) {
  const { path } = await params;
  return <WikiScreen path={path.map(decode).join("/")} />;
}

export default function WikiPathPage({ params }: PageProps<"/wiki/[...path]">) {
  return (
    <Suspense>
      <Loader params={params} />
    </Suspense>
  );
}
