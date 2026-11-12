import { Suspense } from "react";
import { notFound } from "next/navigation";
import { tokenOk } from "@/lib/designs/worker";
import { Preview } from "./preview";

// A bare <DesignSvg> for the render-parity e2e spec (it compares the editor's
// drawing with the worker's PNG). Like the worker routes it needs the bearer
// token (?token=) and is a 404 when DESIGN_WORKER_TOKEN is unset.
async function Gate({ searchParams }: { searchParams: Promise<{ token?: string }> }) {
  const { token } = await searchParams;
  if (!tokenOk(token ?? "")) notFound();
  return <Preview />;
}

export default function Page({ searchParams }: { searchParams: Promise<{ token?: string }> }) {
  return (
    <Suspense>
      <Gate searchParams={searchParams} />
    </Suspense>
  );
}
