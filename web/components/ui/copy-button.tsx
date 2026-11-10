"use client";

import { useState } from "react";
import { IconButton } from "./button";

export function CopyButton({ text, label = "Copy", size }: { text: string; label?: string; size?: number }) {
  const [copied, setCopied] = useState(false);
  return (
    <IconButton
      icon={copied ? "check" : "link"}
      label={copied ? "Copied" : label}
      size={size}
      className={copied ? "motion-pop-child" : undefined}
      onClick={() => {
        void navigator.clipboard.writeText(text);
        setCopied(true);
        setTimeout(() => setCopied(false), 1500);
      }}
    />
  );
}
