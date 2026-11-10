"use client";

import { Dialog } from "radix-ui";
import type { ReactNode } from "react";
import { cx } from "@/lib/cx";
import { Icon, type IconName } from "@/components/ui/icon";

/**
 * The chat drawer an app opens beside its full page: a header with the app's
 * icon and title, the conversation, and an optional footer note. Modal over a
 * dashboard; non-modal beside a document, so the page stays usable while the
 * assistant edits it.
 */
export function AppDrawer({ open, onOpenChange, icon, title, subtitle, actions, footer, modal = true, children }: {
  open: boolean;
  onOpenChange: (o: boolean) => void;
  icon: IconName;
  title: string;
  subtitle?: string;
  actions?: ReactNode;
  footer?: ReactNode;
  modal?: boolean;
  children: ReactNode;
}) {
  return (
    <Dialog.Root open={open} onOpenChange={onOpenChange} modal={modal}>
      <Dialog.Portal>
        {modal && <Dialog.Overlay className="motion-scrim fixed inset-0 z-40 bg-scrim/60" />}
        <Dialog.Content
          aria-describedby={undefined}
          onInteractOutside={modal ? undefined : (e) => e.preventDefault()}
          className={cx("app-drawer fixed inset-y-0 right-0 z-50 flex w-[min(540px,100%)] flex-col border-l border-border bg-bg outline-none", modal ? "shadow-[var(--shadow-float)]" : "md:w-[440px]")}
        >
          <div className="flex items-center gap-[10px] border-b border-border-subtle px-4 py-3">
            <span aria-hidden className="grid size-[26px] place-items-center rounded-lg bg-accent-soft text-accent">
              <Icon name={icon} size={14} />
            </span>
            <div className="min-w-0 flex-1">
              <Dialog.Title className="truncate text-[14px] font-semibold text-fg">{title}</Dialog.Title>
              {subtitle && <p className="text-[11px] text-fg-tertiary">{subtitle}</p>}
            </div>
            {actions}
            <Dialog.Close className="grid size-[28px] place-items-center rounded-md text-fg-secondary hover:bg-hover hover:text-fg" aria-label="Close">
              <Icon name="x" size={14} strokeWidth={1.6} />
            </Dialog.Close>
          </div>
          {children}
          {footer && <p className="flex items-center gap-[6px] border-t border-border-subtle px-4 py-2 text-[11px] text-fg-tertiary">{footer}</p>}
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}
