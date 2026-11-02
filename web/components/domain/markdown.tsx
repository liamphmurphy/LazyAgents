import ReactMarkdown, { type Components } from "react-markdown";
import { cx } from "@/lib/cx";
import { headingAnchors } from "@/lib/slug";

/**
 * Renders user and agent markdown. react-markdown escapes raw HTML by
 * default, so agent-written content can't inject markup.
 */
export function Markdown({ children, className, anchors }: { children: string; className?: string; anchors?: boolean }) {
  // Heading ids come from source lines, so they match the API's outline
  // anchors and stay stable across re-renders.
  const ids = anchors ? headingAnchors(children) : undefined;
  const idOf = (node: { position?: { start: { line: number } } } | undefined) =>
    ids && node?.position ? ids.get(node.position.start.line) : undefined;
  const headings: Components = {
    h1: ({ children, node }) => <h2 id={idOf(node)} className="type-title mt-4 scroll-mt-6">{children}</h2>,
    h2: ({ children, node }) => <h3 id={idOf(node)} className="type-heading mt-4 scroll-mt-6">{children}</h3>,
    h3: ({ children, node }) => <h4 id={idOf(node)} className="mt-2 scroll-mt-6 font-semibold">{children}</h4>,
    h4: ({ children, node }) => <h5 id={idOf(node)} className="mt-2 scroll-mt-6 font-semibold">{children}</h5>,
  };
  return (
    <div className={cx("prose-code flex flex-col gap-2.5 [&_a]:text-accent [&_a]:underline-offset-2 hover:[&_a]:underline", className)}>
      <ReactMarkdown
        components={{
          p: ({ children }) => <p>{children}</p>,
          ul: ({ children }) => <ul className="ml-5 list-disc space-y-1">{children}</ul>,
          ol: ({ children }) => <ol className="ml-5 list-decimal space-y-1">{children}</ol>,
          ...(anchors
            ? headings
            : {
                h1: ({ children }) => <h3 className="type-heading mt-2">{children}</h3>,
                h2: ({ children }) => <h3 className="type-heading mt-2">{children}</h3>,
                h3: ({ children }) => <h4 className="mt-1 font-semibold">{children}</h4>,
              }),
          pre: ({ children }) => (
            <pre className="overflow-x-auto rounded-lg border border-border bg-surface-sunken p-3 font-mono text-xs leading-relaxed [&_code]:border-0 [&_code]:bg-transparent [&_code]:p-0">
              {children}
            </pre>
          ),
          a: ({ href, children }) => (
            <a href={href} target="_blank" rel="noreferrer noopener">
              {children}
            </a>
          ),
        }}
      >
        {children}
      </ReactMarkdown>
    </div>
  );
}
