"""Checks the PoC outputs and writes evidence. Run with a venv that has
pymupdf and python-pptx:  python -I verify.py out evidence
"""
import json
import re
import sys
import zipfile

import pymupdf
from pptx import Presentation
from pptx.util import Emu

out, ev = sys.argv[1], sys.argv[2]
deck = json.load(open(f"{out}/../../fixtures/acceptance-deck.json"))
report = json.load(open(f"{out}/layout-report.json"))
res = {"pdf": {}, "pptx": {}, "failures": []}


def check(ok, msg):
    if not ok:
        res["failures"].append(msg)


# ---- PDF ----
doc = pymupdf.open(f"{out}/deck.pdf")
res["pdf"]["pages"] = doc.page_count
check(doc.page_count == len(deck["slides"]), "one PDF page per slide")
fonts = set()
for i, page in enumerate(doc):
    check(round(page.rect.width) == 960 and round(page.rect.height) == 540, f"page {i+1} is 960x540pt")
    # jsPDF lists the standard 14 fonts and every image in a shared resource
    # dictionary, so check what the page actually draws, not what it lists.
    embedded = {f[3].split("+")[-1]: len(doc.extract_font(f[0])[3] or b"") > 0 for f in page.get_fonts(full=True)}
    for b in page.get_text("dict")["blocks"]:
        for l in b.get("lines", []):
            for sp in l["spans"]:
                fonts.add((sp["font"], embedded.get(sp["font"], False)))
    text = page.get_text()
    res["pdf"].setdefault("first_words", []).append(" ".join(text.split()[:6]))
    pix = page.get_pixmap(dpi=48)
    pix.save(f"{ev}/pdf-page-{i+1}.png")
    res["pdf"].setdefault("images_on_page", []).append(len(page.get_image_info()))
res["pdf"]["fonts"] = sorted(f"{n} (embedded)" if e else f"{n} (NOT embedded)" for n, e in fonts)
check(all(e for _, e in fonts), "every PDF font is embedded")
check("Quarterly review" in doc[0].get_text(), "slide 1 title is real text in the PDF")
check("deliberately long bullet" in doc[1].get_text(), "wrapped bullet is real text in the PDF")
check(all(n == 0 for i, n in enumerate(res["pdf"]["images_on_page"]) if i != 2), "only the image slide has a raster")

# ---- PPTX ----
prs = Presentation(f"{out}/deck.pptx")
res["pptx"]["slide_size_in"] = [round(Emu(prs.slide_width).inches, 3), round(Emu(prs.slide_height).inches, 3)]
check(prs.slide_width == 12192000 and prs.slide_height == 6858000, "PPTX is 16:9 widescreen")
check(len(prs.slides) == len(deck["slides"]), "one PPTX slide per deck slide, same order")
for i, (slide, src) in enumerate(zip(prs.slides, deck["slides"])):
    shapes = []
    for sh in slide.shapes:
        kind = sh.shape_type and str(sh.shape_type).split(".")[-1].split(" ")[0]
        entry = {"name": sh.name, "kind": kind, "editable_text": sh.has_text_frame and bool(sh.text_frame.text.strip())}
        if entry["editable_text"]:
            entry["text"] = sh.text_frame.text[:60]
            runs = [r for p in sh.text_frame.paragraphs for r in p.runs]
            entry["fonts"] = sorted({r.font.name for r in runs if r.font.name})
        if sh.shape_type == 13:  # picture
            entry["crop"] = {k: round(getattr(sh, f"crop_{k}"), 4) for k in ("left", "top", "right", "bottom")}
            covers = sh.width >= prs.slide_width * 0.95 and sh.height >= prs.slide_height * 0.95
            check(not covers, f"slide {i+1}: no whole-slide picture")
        shapes.append(entry)
    res["pptx"].setdefault("slides", []).append({"id": src["id"], "shapes": shapes})
    names = [s["name"] for s in shapes]
    check(names == [e["id"] for e in src["elements"]], f"slide {i+1}: element ids/order survive as shape names {names}")
    for e in src["elements"]:
        if e["type"] == "text" or e.get("text"):
            sh = next(s for s in shapes if s["name"] == e["id"])
            check(sh["editable_text"], f"slide {i+1}: {e['id']} is an editable text frame")

pic = next(s for sl in res["pptx"]["slides"] for s in sl["shapes"] if "crop" in s)
exp = report["expectedCrops"]["e_i3"]
for k, short in (("left", "l"), ("top", "t"), ("right", "r"), ("bottom", "b")):
    check(abs(pic["crop"][k] - exp[short]) < 0.002, f"image crop {k} {pic['crop'][k]} == {exp[short]:.4f}")

with zipfile.ZipFile(f"{out}/deck.pptx") as z:
    xml = z.read("ppt/slides/slide2.xml").decode()
    res["pptx"]["slide2_line_spacing_pts"] = sorted(set(re.findall(r'<a:spcPts val="(\d+)"', xml)))
    res["pptx"]["slide2_paragraphs"] = [f"{m.group(1)} {m.group(2)}" for m in re.finditer(r"<a:p>(<a:pPr[^>]*>).*?<a:t>([^<]{0,20})", xml)]
    check('lvl="1" marL="762000"' in xml, "level-1 bullet is indented 120 units (762000 EMU), like the shared layout")
    res["pptx"]["autofit"] = "normAutofit" in xml
    check("normAutofit" not in xml, "PPTX text does not auto-shrink")
    res["pptx"]["notes_parts"] = [n for n in z.namelist() if n.startswith("ppt/notesSlides/") and n.endswith(".xml")]

res["layout_warnings"] = report["warnings"]
json.dump(res, open(f"{ev}/verify-report.json", "w"), indent=2)
print(json.dumps({"failures": res["failures"], "pdf_fonts": res["pdf"]["fonts"], "crop": pic["crop"]}, indent=1))
sys.exit(1 if res["failures"] else 0)
