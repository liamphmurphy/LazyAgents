import type { JSONContent } from "@tiptap/core";

/** The docx module, passed in so the page can load it only when exporting. */
type Docx = typeof import("docx");
type Inline = InstanceType<Docx["TextRun"]> | InstanceType<Docx["ExternalHyperlink"]>;
type Block = InstanceType<Docx["Paragraph"]> | InstanceType<Docx["Table"]>;

const MONO = "Menlo";
const ORDERED = "ordered";

/**
 * A Word document from the editor's document. It keeps structure and inline
 * formatting (headings, lists, tasks, quotes, code, tables, links); Word's
 * default styles do the rest, so it reads as the person's own document.
 */
export function toDocx(d: Docx, doc: JSONContent, title: string) {
  // Each ordered list restarts at 1: its own numbering instance.
  let lists = 0;

  const runs = (nodes: JSONContent[] = [], extra: { code?: boolean } = {}): Inline[] =>
    nodes.flatMap((n): Inline[] => {
      if (n.type === "hardBreak") return [new d.TextRun({ break: 1 })];
      if (n.type !== "text" || !n.text) return [];
      const marks = new Set((n.marks ?? []).map((m) => m.type));
      const run = new d.TextRun({
        text: n.text,
        bold: marks.has("bold"),
        italics: marks.has("italic"),
        strike: marks.has("strike"),
        font: marks.has("code") || extra.code ? MONO : undefined,
        style: marks.has("link") ? "Hyperlink" : undefined,
      });
      const href = n.marks?.find((m) => m.type === "link")?.attrs?.href;
      return typeof href === "string" && href ? [new d.ExternalHyperlink({ link: href, children: [run] })] : [run];
    });

  type Ctx = { level: number; list?: { ordered: boolean; instance: number }; quote?: boolean; check?: boolean | null };

  const blocks = (nodes: JSONContent[] = [], ctx: Ctx = { level: 0 }): Block[] =>
    nodes.flatMap((n): Block[] => {
      switch (n.type) {
        case "heading": {
          const level = Math.min(6, Math.max(1, Number(n.attrs?.level) || 1));
          return [new d.Paragraph({ heading: d.HeadingLevel[`HEADING_${level}` as "HEADING_1"], children: runs(n.content) })];
        }
        case "paragraph": {
          const children = runs(n.content);
          if (ctx.check != null) children.unshift(new d.TextRun({ text: ctx.check ? "☑ " : "☐ " }));
          const list = ctx.list;
          return [
            new d.Paragraph({
              children,
              ...(list?.ordered ? { numbering: { reference: ORDERED, level: ctx.level, instance: list.instance } } : list ? { bullet: { level: ctx.level } } : {}),
              ...(ctx.quote ? { indent: { left: 720 }, border: { left: { style: d.BorderStyle.SINGLE, size: 12, color: "BBBBBB", space: 8 } } } : {}),
            }),
          ];
        }
        case "bulletList":
        case "orderedList":
        case "taskList": {
          const ordered = n.type === "orderedList";
          const list = { ordered, instance: ordered ? ++lists : 0 };
          const level = ctx.list ? ctx.level + 1 : ctx.level;
          return (n.content ?? []).flatMap((item) => {
            const check = item.type === "taskItem" ? !!item.attrs?.checked : null;
            const [first, ...rest] = item.content ?? [];
            // The item's first paragraph carries its bullet; nested lists indent under it.
            return [...blocks(first ? [first] : [], { level, list: n.type === "taskList" ? undefined : list, quote: ctx.quote, check }), ...blocks(rest, { level, list, quote: ctx.quote })];
          });
        }
        case "blockquote":
          return blocks(n.content, { ...ctx, quote: true });
        case "codeBlock": {
          const lines = (n.content ?? []).map((t) => t.text ?? "").join("").split("\n");
          return lines.map((line) => new d.Paragraph({ children: [new d.TextRun({ text: line, font: MONO })], shading: { type: d.ShadingType.CLEAR, fill: "F3F3F3", color: "auto" } }));
        }
        case "horizontalRule":
          return [new d.Paragraph({ children: [], border: { bottom: { style: d.BorderStyle.SINGLE, size: 6, color: "BBBBBB", space: 1 } } })];
        case "table":
          return [
            new d.Table({
              width: { size: 100, type: d.WidthType.PERCENTAGE },
              rows: (n.content ?? []).map(
                (row) =>
                  new d.TableRow({
                    children: (row.content ?? []).map((cell) => {
                      const inner = blocks(cell.content, { level: 0 }).filter((b): b is InstanceType<Docx["Paragraph"]> => b instanceof d.Paragraph);
                      return new d.TableCell({ children: inner.length ? inner : [new d.Paragraph({})], shading: cell.type === "tableHeader" ? { type: d.ShadingType.CLEAR, fill: "F3F3F3", color: "auto" } : undefined });
                    }),
                  }),
              ),
            }),
          ];
        default:
          return blocks(n.content, ctx);
      }
    });

  const body = blocks(doc.content);
  return new d.Document({
    title,
    creator: "Vesper",
    numbering: {
      config: [
        {
          reference: ORDERED,
          levels: Array.from({ length: 6 }, (_, level) => ({
            level,
            format: d.LevelFormat.DECIMAL,
            text: `%${level + 1}.`,
            alignment: d.AlignmentType.START,
            style: { paragraph: { indent: { left: 720 * (level + 1), hanging: 360 } } },
          })),
        },
      ],
    },
    sections: [{ children: body.length ? body : [new d.Paragraph({})] }],
  });
}
