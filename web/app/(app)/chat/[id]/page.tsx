import { Suspense } from "react";
import { ChatScreen } from "../_components/chat-screen";

async function Loader({ params }: { params: PageProps<"/chat/[id]">["params"] }) {
  const { id } = await params;
  return <ChatScreen id={decodeURIComponent(id)} />;
}

export default function Page({ params }: PageProps<"/chat/[id]">) {
  // params are request-time data under Cache Components, so they stream in Suspense.
  return (
    <Suspense>
      <Loader params={params} />
    </Suspense>
  );
}
