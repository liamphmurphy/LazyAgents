import { redirect } from "next/navigation";

// Decks live in Knowledge now (LAZYAGENTS-212): their start page lists them and makes new ones.
export default function Page() {
  redirect("/knowledge?kind=deck");
}
