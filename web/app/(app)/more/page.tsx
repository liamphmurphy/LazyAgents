import Link from "next/link";
import { Icon } from "@/components/ui/icon";
import { Content, TopBar } from "@/components/shell/topbar";
import { NAV_GROUPS } from "@/lib/project-scope";

/**
 * The compact shell's More tab: every section the tab bar has no tab for. A plain list
 * for now so each stays reachable on a phone; its designed contents come in a later ticket.
 */
export default function Page() {
  const groups = NAV_GROUPS.filter((g) => g.label !== "Work");
  return (
    <>
      <TopBar crumbs={[{ label: "More" }]} />
      <Content>
        <div className="flex flex-col gap-6">
          {groups.map((g) => (
            <section key={g.label} aria-labelledby={`more-${g.label}`}>
              <h2 id={`more-${g.label}`} className="mb-1.5 px-3 text-xs font-medium text-fg-tertiary">
                {g.label}
              </h2>
              <ul className="overflow-hidden rounded-lg border border-border bg-surface">
                {g.items.map((item) => (
                  <li key={item.href} className="border-b border-border-subtle last:border-b-0">
                    <Link href={item.href} className="flex min-h-11 items-center gap-3 px-3 text-sm text-fg hover:bg-hover">
                      <Icon name={item.icon} size={16} strokeWidth={1.6} />
                      {item.label}
                    </Link>
                  </li>
                ))}
              </ul>
            </section>
          ))}
        </div>
      </Content>
    </>
  );
}
