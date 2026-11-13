import { KnowledgeShell } from "./_components/shell";

export default function KnowledgeLayout({ children }: LayoutProps<"/knowledge">) {
  return <KnowledgeShell>{children}</KnowledgeShell>;
}
