import { Suspense } from "react";
import { DocScreen } from "../_components/doc-screen";

async function Loader({ params }: { params: PageProps<"/apps/docs/[id]">["params"] }) {
  const { id } = await params;
  return <DocScreen id={id} />;
}

export default function Page({ params }: PageProps<"/apps/docs/[id]">) {
  // The drawer's conversation reads search params, which stream in Suspense.
  return (
    <Suspense>
      <Loader params={params} />
    </Suspense>
  );
}
