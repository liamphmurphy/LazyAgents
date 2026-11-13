import { redirect } from "next/navigation";

// The wiki is part of Knowledge; its agent pages are the Agent knowledge view.
export default function WikiPage() {
  redirect("/knowledge");
}
