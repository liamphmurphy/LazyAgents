import { Suspense } from "react";
import { BoardScreen } from "./board-screen";

export default function BoardPage() {
  return <Suspense><BoardScreen /></Suspense>;
}
