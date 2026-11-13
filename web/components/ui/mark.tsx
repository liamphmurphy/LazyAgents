"use client";

import { useId } from "react";

/** The brand asset's geometry, with colors inherited from Appearance. */
export function Mark({ size = 22 }: { size?: number }) {
  // Multiple marks can share a page without sharing SVG paint servers.
  const id = useId();
  const dusk = `${id}-dusk`, glow = `${id}-glow`, tile = `${id}-tile`;
  return (
    <svg width={size} height={size} viewBox="0 0 64 64" fill="none" aria-hidden="true" className="flex-none">
      <defs>
        <linearGradient id={dusk} x1="32" y1="0" x2="32" y2="64" gradientUnits="userSpaceOnUse">
          <stop stopColor="var(--brand-deep)" />
          <stop offset=".62" stopColor="var(--accent)" />
          <stop offset="1" stopColor="var(--brand-star)" />
        </linearGradient>
        <radialGradient id={glow} cx="0" cy="0" r="1" gradientUnits="userSpaceOnUse" gradientTransform="translate(32 27) scale(16)">
          <stop stopColor="#FFF6DD" stopOpacity=".55" />
          <stop offset="1" stopColor="#FFF6DD" stopOpacity="0" />
        </radialGradient>
        <clipPath id={tile}><rect width="64" height="64" rx="18" /></clipPath>
      </defs>
      <g clipPath={`url(#${tile})`}>
        <rect width="64" height="64" fill={`url(#${dusk})`} />
        <circle cx="32" cy="27" r="16" fill={`url(#${glow})`} />
        {/* A deep outline keeps the cream star distinct on bright accents. */}
        <path d="M32 12c1.2 8.6 3.4 10.8 12 12-8.6 1.2-10.8 3.4-12 12-1.2-8.6-3.4-10.8-12-12 8.6-1.2 10.8-3.4 12-12Z" fill="#FFF6DD" stroke="var(--brand-deep)" strokeWidth="1" />
        <circle cx="48" cy="13" r="1.6" fill="#F7F5FF" fillOpacity=".8" />
        <circle cx="15" cy="17" r="1.2" fill="#F7F5FF" fillOpacity=".6" />
        <path d="M-4 52c12-6 24-8 36-8s24 2 36 8v16H-4Z" fill="color-mix(in srgb, var(--brand-deep) 65%, var(--bg))" />
      </g>
    </svg>
  );
}
