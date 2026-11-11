import re
names = ["editor", "review", "present", "export", "states"]
titles = ["2 · Editor: thumbnails, canvas, formatting, persona chat", "3 · Suggestion review", "4 · Presentation", "5 · PDF / PPTX export", "6 · States: saving, conflict, errors, overflow, focus, narrow"]
parts, y = [], 0
for n, t in zip(names, titles):
    s = open(f"/workspace/docs/design/slides-{n}.svg").read()
    h = int(re.search(r'height="(\d+)"', s).group(1))
    for ident in ("screen", "sky", "sh"):
        s = s.replace(f'id="{ident}"', f'id="{n}-{ident}"').replace(f"url(#{ident})", f"url(#{n}-{ident})")
    s = s.replace('<svg xmlns="http://www.w3.org/2000/svg" ', f'<svg x="0" y="{y + 40}" ', 1)
    parts.append(f'<text x="20" y="{y + 28}" font-size="18" font-weight="600" fill="#F2A97E">{t}</text>' + s)
    y += h + 60
out = (f'<svg xmlns="http://www.w3.org/2000/svg" font-family="Inter, \'Helvetica Neue\', Arial, sans-serif" width="1800" height="{y}" viewBox="0 0 1800 {y}" role="img" aria-label="Slides mockups 2 to 6">'
       f'<title>Slides mockups 2-6: editor, review, presentation, export, states</title><rect width="1800" height="{y}" fill="#07060B"/>' + "".join(parts) + "</svg>")
open("/tmp/mockout/slides-2-6-combined.svg", "w").write(out)
print(len(out), y)
