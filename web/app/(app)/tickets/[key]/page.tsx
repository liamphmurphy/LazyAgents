import { Suspense } from "react";
import { TicketScreen } from "./ticket-screen";

async function Loader({ params }: { params: PageProps<"/tickets/[key]">["params"] }) {
  const { key } = await params;
  return <TicketScreen ticketKey={decodeURIComponent(key).toUpperCase()} />;
}

export default function TicketPage({ params }: PageProps<"/tickets/[key]">) {
  // params are request-time data under Cache Components, so they stream in Suspense.
  return (
    <Suspense>
      <Loader params={params} />
    </Suspense>
  );
}
