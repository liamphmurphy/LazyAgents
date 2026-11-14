import { redirect } from "next/navigation";

// Docs are Knowledge pages now; the Documents start page lists them.
export default function Page() {
  redirect("/knowledge?kind=document");
}
