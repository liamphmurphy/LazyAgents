"use client";

import { cx } from "@/lib/cx";
import { Icon } from "@/components/ui/icon";
import { CopyButton } from "@/components/ui/copy-button";

/**
 * The folder on the person's machine holding a run's checkout (host-folder
 * workspaces), with a button that copies its path.
 */
export function WorkspaceFolderChip({ path, className }: { path: string; className?: string }) {
  return (
    <span
      data-testid="workspace-folder"
      title={`Checkout on this machine:\n${path}`}
      className={cx("flex min-w-0 flex-none items-center gap-1 rounded-full border border-border bg-surface py-px pl-2.5 pr-0.5 text-xs text-fg-secondary", className)}
    >
      <Icon name="folder" size={13} className="flex-none" />
      <span className="max-w-[220px] truncate font-mono text-[11.5px]" dir="rtl">
        {/* rtl keeps the run's own folder visible when the path truncates */}
        <bdi>{path}</bdi>
      </span>
      <CopyButton text={path} label="Copy folder path" size={18} />
    </span>
  );
}

