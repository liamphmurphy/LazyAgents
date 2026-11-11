import { Suspense } from "react";
import { DeckScreen } from "../_components/deck-screen";

async function Loader({ params }: { params: PageProps<"/apps/slides/[id]">["params"] }) {
  const { id } = await params;
  return <DeckScreen id={id} />;
}

export default function Page({ params }: PageProps<"/apps/slides/[id]">) {
  // The chat and the brief read search params, which stream in Suspense.
  return (
    <Suspense>
      <Loader params={params} />
    </Suspense>
  );
}
