"use client";

import { useContext, useState } from "react";
import type { Schemas } from "@/lib/api/client";
import { useLinkPreviews } from "@/lib/api/hooks";
import { parseRecommend, videoEmbedUrl } from "@/lib/chat-recommend";
import { hostOf } from "@/lib/web-search";
import { Icon } from "@/components/ui/icon";
import { CodeFence } from "./code-fence";
import { SvgFenceMessage } from "./svg-fence";

const FRAME = "overflow-hidden rounded-xl border border-border bg-surface";
const WIDTH = "w-[380px] max-w-full";

function Note({ note }: { note: string }) {
  return note ? <p className="mt-2 text-[12.5px] italic text-fg-secondary">“{note}”</p> : null;
}

function Badge({ children }: { children: string }) {
  return <span className="absolute left-2.5 top-2.5 rounded-full px-2.5 py-0.5 text-[11.5px] font-semibold" style={{ background: "rgb(0 0 0 / 0.55)", color: "#fff" }}>{children}</span>;
}

function OpenMark() {
  return <Icon name="link" size={13} className="flex-none text-fg-tertiary" />;
}

/** No title or image to show: a compact link, with the persona's reason. */
function CompactCard({ url, title, note }: { url: string; title?: string; note: string }) {
  const host = hostOf(url);
  return (
    <a href={url} target="_blank" rel="noopener noreferrer" data-testid="recommend-card" data-kind="link" className={`${WIDTH} ${FRAME} flex items-center gap-2.5 p-2.5 no-underline transition-colors hover:border-border-strong`}>
      <span aria-hidden className="grid size-8 flex-none place-items-center rounded-lg bg-surface-sunken text-sm font-bold text-fg-secondary">{host.slice(0, 1).toUpperCase()}</span>
      <span className="flex min-w-0 flex-1 flex-col">
        <span className="truncate text-[13px] font-semibold text-fg">{title || `${host}${new URL(url).pathname.replace(/\/$/, "")}`}</span>
        {note ? <span className="truncate text-[12px] italic text-fg-secondary">“{note}”</span> : title && <span className="truncate text-[12px] text-fg-tertiary">{host}</span>}
      </span>
      <OpenMark />
    </a>
  );
}

function VideoCard({ url, preview, embed, note }: { url: string; preview: Schemas["LinkPreview"]; embed: string; note: string }) {
  const [playing, setPlaying] = useState(false);
  return (
    <div data-testid="recommend-card" data-kind="video" className={`${WIDTH} ${FRAME}`}>
      <div className="relative aspect-video bg-surface-sunken">
        {playing ? (
          <iframe src={embed} title={preview.title} allow="autoplay; encrypted-media; picture-in-picture; fullscreen" referrerPolicy="strict-origin-when-cross-origin" className="absolute inset-0 size-full border-0" />
        ) : (
          // The player, and any contact with YouTube, starts only here.
          <button type="button" aria-label={`Play ${preview.title}`} onClick={() => setPlaying(true)} className="group absolute inset-0 grid place-items-center">
            {preview.image_url && <img src={preview.image_url} alt="" className="absolute inset-0 size-full object-cover" />}
            <Badge>Video</Badge>
            <span aria-hidden className="relative grid size-14 place-items-center rounded-full shadow-md transition-transform group-hover:scale-105" style={{ background: "rgb(255 255 255 / 0.95)", color: "#17171c" }}>
              <svg viewBox="0 0 24 24" width="24" height="24" fill="currentColor"><path d="M8 5.5v13l11-6.500z" /></svg>
            </span>
          </button>
        )}
      </div>
      <div className="p-3">
        <a href={url} target="_blank" rel="noopener noreferrer" className="flex items-start gap-2 no-underline hover:underline">
          <span className="min-w-0 flex-1 text-[14px] font-semibold text-fg">{preview.title}</span>
          <OpenMark />
        </a>
        <p className="mt-0.5 truncate text-xs text-fg-tertiary">{[preview.site, hostOf(url)].filter(Boolean).join(" · ")}</p>
        <Note note={note} />
      </div>
    </div>
  );
}

function SiteCard({ url, preview, note }: { url: string; preview: Schemas["LinkPreview"]; note: string }) {
  const [broken, setBroken] = useState(false);
  if (!preview.image_url || broken) return <CompactCard url={url} title={preview.title} note={note} />;
  return (
    <a href={url} target="_blank" rel="noopener noreferrer" data-testid="recommend-card" data-kind="site" className={`${WIDTH} ${FRAME} block no-underline transition-colors hover:border-border-strong`}>
      <span className="relative block aspect-video bg-surface-sunken">
        <img src={preview.image_url} alt="" onError={() => setBroken(true)} className="absolute inset-0 size-full object-cover" />
        <Badge>Website</Badge>
      </span>
      <span className="block p-3">
        <span className="flex items-start gap-2">
          <span className="min-w-0 flex-1 text-[14px] font-semibold text-fg">{preview.title}</span>
          <OpenMark />
        </span>
        <span className="mt-0.5 block truncate text-xs text-fg-tertiary">{preview.site || hostOf(url)}</span>
        {note ? <Note note={note} /> : preview.description && <span className="mt-2 line-clamp-2 block text-[12.5px] text-fg-secondary">{preview.description}</span>}
      </span>
    </a>
  );
}

/** A ```recommend fence: a card for the page or video a persona says to go look at. */
export function RecommendFence({ code, info }: { code: string; info?: string }) {
  const parsed = parseRecommend(code);
  const messageId = useContext(SvgFenceMessage);
  const q = useLinkPreviews(parsed ? messageId : undefined);
  if (!parsed) return <CodeFence code={code} info={info} />;
  const { url, note } = parsed;
  if (messageId && q.isPending) return <div data-testid="recommend-loading" aria-label="Loading preview" className={`${WIDTH} ${FRAME} h-16 animate-pulse`} />;
  const preview = q.data?.previews.find((p) => p.url === url);
  if (!preview?.title) return <CompactCard url={url} note={note} />;
  const embed = preview.kind === "video" ? videoEmbedUrl(preview.video_id) : null;
  return embed ? <VideoCard url={url} preview={preview} embed={embed} note={note} /> : <SiteCard url={url} preview={preview} note={note} />;
}
