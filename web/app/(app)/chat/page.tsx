import { Suspense } from "react";
import { ChatScreen } from "./_components/chat-screen";

export default function Page() {
  return (
    <Suspense>
      <ChatScreen />
    </Suspense>
  );
}
