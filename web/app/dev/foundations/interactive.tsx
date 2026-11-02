"use client";

import { useState } from "react";
import { Segmented, Tabs, Toggle } from "@/components/ui/controls";

export function RangeDemo() {
  const [v, setV] = useState("7d");
  return (
    <Segmented
      label="Range"
      value={v}
      onChange={setV}
      options={[
        { value: "24h", label: "24h" },
        { value: "7d", label: "7 days" },
        { value: "30d", label: "30 days" },
      ]}
    />
  );
}

export function ScopeModeDemo() {
  const [v, setV] = useState("approval");
  return (
    <Segmented
      small
      label="Permission mode"
      value={v}
      onChange={setV}
      options={[
        { value: "allowed", label: "Allowed" },
        { value: "approval", label: "Approval" },
        { value: "denied", label: "Denied" },
      ]}
    />
  );
}

export function ToggleDemo() {
  const [a, setA] = useState(true);
  const [b, setB] = useState(false);
  return (
    <div className="flex flex-col gap-2">
      <Toggle checked={a} onChange={setA} label="Sync PR state" />
      <Toggle checked={b} onChange={setB} label="Auto-dispatch" />
    </div>
  );
}

export function TabsDemo() {
  const [v, setV] = useState("active");
  return (
    <Tabs
      label="Views"
      value={v}
      onChange={setV}
      items={[
        { key: "active", label: "Active", count: 42 },
        { key: "backlog", label: "Backlog", count: 18 },
        { key: "agent", label: "Agent-owned", count: 27 },
        { key: "human", label: "Needs human", count: 4 },
      ]}
    />
  );
}
