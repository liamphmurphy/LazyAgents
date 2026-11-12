import { Suspense } from "react";
import { DesignEditor } from "../_components/design-editor";

async function Loader({ params }: { params: PageProps<"/apps/designs/[id]">["params"] }) {
  const { id } = await params;
  return <DesignEditor id={id} />;
}

export default function Page({ params }: PageProps<"/apps/designs/[id]">) {
  // The brief reads search params, which stream in Suspense.
  return (
    <Suspense>
      <Loader params={params} />
    </Suspense>
  );
}
