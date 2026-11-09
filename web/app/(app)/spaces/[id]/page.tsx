import { Suspense } from "react";
import { SpaceScreen } from "../_components/space-screen";

async function Loader({ params }: { params: PageProps<"/spaces/[id]">["params"] }) {
  const { id } = await params;
  return <SpaceScreen id={decodeURIComponent(id)} />;
}

export default function Page({ params }: PageProps<"/spaces/[id]">) {
  // params are request-time data under Cache Components, so they stream in Suspense.
  return (
    <Suspense>
      <Loader params={params} />
    </Suspense>
  );
}
