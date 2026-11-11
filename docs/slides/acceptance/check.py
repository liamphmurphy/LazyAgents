"""Checks a deck's exports inside the LibreOffice container (run.sh).

/work holds what web/scripts/slides-deck-evidence.mjs wrote (slide-NN.png,
deck.pdf, deck.pptx, slides.json) plus the deck itself as deck.json. Writes
report.json and compare-NN.png (editor | app PDF | LibreOffice's render of the
PPTX) into /work/evidence and exits 1 if any check fails.
"""
import json
import re
import subprocess
import sys
import zipfile
import xml.etree.ElementTree as ET
from pathlib import Path

from PIL import Image, ImageChops, ImageDraw, ImageStat

work = Path(sys.argv[1] if len(sys.argv) > 1 else "/work")
ev = work / "evidence"
ev.mkdir(exist_ok=True)
deck = json.loads((work / "deck.json").read_text())
shown = json.loads((work / "slides.json").read_text())
slides = {s["id"]: s for s in deck["slides"]}
failures, report = [], {"slides": []}


def fail(msg):
    failures.append(msg)


def sh(*args):
    return subprocess.run(args, capture_output=True, text=True, timeout=300)


def pages(pdf):
    m = re.search(r"Pages:\s+(\d+)", sh("pdfinfo", str(pdf)).stdout)
    return int(m.group(1)) if m else 0


report["libreoffice"] = sh("soffice", "--version").stdout.strip()

# 1. LibreOffice opens the PPTX: converts it to PDF, and round-trips it.
lo = work / "lo"
r = sh("soffice", "--headless", "--convert-to", "pdf", "--outdir", str(lo), str(work / "deck.pptx"))
lo_pdf = lo / "deck.pdf"
report["lo_convert_pdf"] = {"exit": r.returncode, "pages": pages(lo_pdf) if lo_pdf.exists() else 0}
if r.returncode or report["lo_convert_pdf"]["pages"] != len(shown):
    fail(f"LibreOffice PPTX→PDF: exit {r.returncode}, {report['lo_convert_pdf']['pages']} pages, want {len(shown)}: {r.stderr[-400:]}")
rt = work / "rt"
r = sh("soffice", "--headless", "--convert-to", "pptx", "--outdir", str(rt), str(work / "deck.pptx"))
r2 = sh("soffice", "--headless", "--convert-to", "pdf", "--outdir", str(rt), str(rt / "deck.pptx"))
report["lo_roundtrip"] = {"exit": r.returncode, "reopen_exit": r2.returncode, "pages": pages(rt / "deck.pdf") if (rt / "deck.pdf").exists() else 0}
if r.returncode or r2.returncode or report["lo_roundtrip"]["pages"] != len(shown):
    fail(f"LibreOffice PPTX round trip: {report['lo_roundtrip']}")
report["app_pdf_pages"] = pages(work / "deck.pdf")
if report["app_pdf_pages"] != len(shown):
    fail(f"app PDF has {report['app_pdf_pages']} pages, want {len(shown)}")

# 2. The PPTX's structure: native, named, editable elements carrying the styles.
NS = {"p": "http://schemas.openxmlformats.org/presentationml/2006/main", "a": "http://schemas.openxmlformats.org/drawingml/2006/main"}
P, A = "{%s}" % NS["p"], "{%s}" % NS["a"]
z = zipfile.ZipFile(work / "deck.pptx")
pres = z.read("ppt/presentation.xml").decode()
if 'cx="12192000"' not in pres or 'cy="6858000"' not in pres:
    fail("slide size isn't 12192000 × 6858000 EMU")
if "normAutofit" in "".join(z.read(n).decode() for n in z.namelist() if n.startswith("ppt/slides/slide")):
    fail("a text frame uses normAutofit")
nslides = len([n for n in z.namelist() if re.fullmatch(r"ppt/slides/slide\d+\.xml", n)])
if nslides != len(shown):
    fail(f"PPTX has {nslides} slides, want {len(shown)}")


def by_name(root):
    """Every shape-like node by its cNvPr name, with its parent group's name."""
    out = {}

    def walk(node, group):
        for ch in node:
            tag = ch.tag.replace(P, "p:")
            if tag in ("p:sp", "p:cxnSp", "p:pic", "p:grpSp"):
                nv = ch.find(".//" + P + "cNvPr")
                name = nv.get("name") if nv is not None else None
                out[name] = (tag, ch, group)
                if tag == "p:grpSp":
                    walk(ch, name)

    walk(root.find(".//" + P + "spTree"), None)
    return out


def check_text(where, node, box):
    paras = node.findall(".//" + A + "p")
    if node.find(".//" + P + "txBody") is None:
        return fail(f"{where}: no editable text frame")
    for i, p in enumerate(box["paragraphs"]):
        if i >= len(paras):
            return fail(f"{where}: paragraph {i + 1} missing")
        xp = paras[i]
        ppr = xp.find(A + "pPr")
        if p.get("align") == "justify" and (ppr is None or ppr.get("algn") != "just"):
            fail(f"{where} paragraph {i + 1}: not justified")
        if p.get("lineSpacing") and xp.find(f"{A}pPr/{A}lnSpc/{A}spcPts") is None:
            fail(f"{where} paragraph {i + 1}: line spacing isn't exact points")
        rprs = xp.findall(f"{A}r/{A}rPr")
        if any(r.get("strike") for r in p["runs"]) and not any(x.get("strike") == "sngStrike" for x in rprs):
            fail(f"{where} paragraph {i + 1}: strikethrough missing")
        if any(r.get("highlight") for r in p["runs"]) and not any(x.find(A + "highlight") is not None for x in rprs):
            fail(f"{where} paragraph {i + 1}: highlight missing")


counts = {"elements": 0, "groups": 0, "presets": 0, "gradients": 0, "transparent": 0, "dashed": 0}
for i, sid in enumerate(shown, 1):
    root = ET.fromstring(z.read(f"ppt/slides/slide{i}.xml"))
    nodes = by_name(root)
    s = slides[sid]
    bg = (s.get("background") or {}).get("color")
    if isinstance(bg, dict) and root.find(f".//{P}bg//{A}gradFill") is None:
        fail(f"{sid}: gradient background missing")

    def check(e, group=None):
        where = f"{sid}/{e['id']}"
        if e["type"] == "text" and not any(r["text"] for p in e["paragraphs"] for r in p["runs"]):
            return  # empty placeholders aren't exported
        if e["id"] not in nodes:
            return fail(f"{where}: not in the PPTX by name")
        tag, node, parent = nodes[e["id"]]
        counts["elements"] += 1
        if parent != group:
            fail(f"{where}: in group {parent}, want {group}")
        if e["type"] == "group":
            counts["groups"] += 1
            if tag != "p:grpSp":
                fail(f"{where}: a group exported as {tag}, not p:grpSp")
            for c in e["elements"]:
                check(c, e["id"])
            return
        # Every element sits in its frame (EMU = slide units × 6350; lines may flip).
        xf = node.find(f"{P}spPr/{A}xfrm")
        if xf is not None:
            fr = e["frame"]
            want = [min(fr["x"], fr["x"] + fr["w"]), min(fr["y"], fr["y"] + fr["h"]), abs(fr["w"]), abs(fr["h"])]
            off, ext = xf.find(A + "off"), xf.find(A + "ext")
            got = [int(off.get("x")), int(off.get("y")), int(ext.get("cx")), int(ext.get("cy"))]
            if any(abs(g - w * 6350) > 6350 for g, w in zip(got, want)):
                fail(f"{where}: placed at {[round(g / 6350, 1) for g in got]} units, want {want}")
        if e["type"] == "image":
            if tag != "p:pic":
                fail(f"{where}: image exported as {tag}")
            if e.get("opacity") is not None and node.find(f".//{A}alphaModFix") is None:
                fail(f"{where}: image transparency missing")
            counts["transparent"] += e.get("opacity") is not None
            return
        if tag == "p:pic":
            return fail(f"{where}: drawn as a picture, not an editable shape")
        geom = node.find(f".//{A}prstGeom")
        if e["type"] == "shape":
            want = e["shape"]
            if geom is None or geom.get("prst") != want:
                fail(f"{where}: prstGeom {geom.get('prst') if geom is not None else None}, want {want}")
            counts["presets"] += want not in ("rect", "roundRect", "ellipse")
            if isinstance(e.get("fill"), dict):
                counts["gradients"] += 1
                if node.find(f"{P}spPr/{A}gradFill") is None:
                    fail(f"{where}: gradient fill missing")
            if e.get("fillOpacity") is not None:
                counts["transparent"] += 1
                if node.find(f"{P}spPr/{A}gradFill//{A}alpha") is None and node.find(f"{P}spPr/{A}solidFill//{A}alpha") is None:
                    fail(f"{where}: fill transparency missing")
            if e.get("text"):
                check_text(where, node, e["text"])
        if e["type"] in ("shape", "line"):
            if e.get("dash"):
                counts["dashed"] += 1
                if node.find(f".//{A}ln/{A}prstDash") is None:
                    fail(f"{where}: dash missing")
            if e.get("strokeOpacity") is not None and node.find(f".//{A}ln//{A}alpha") is None:
                fail(f"{where}: outline transparency missing")
            if e["type"] == "line" and (e.get("endArrow") or e.get("startArrow")):
                ln = node.find(f".//{A}ln")
                if (e.get("endArrow") and ln.find(A + "tailEnd") is None) or (e.get("startArrow") and ln.find(A + "headEnd") is None):
                    fail(f"{where}: arrowhead missing")
        if e["type"] == "text":
            check_text(where, node, e)

    for e in s["elements"]:
        check(e)
report["pptx_counts"] = counts

# 3. Pixels: editor (Chromium SVG) vs app PDF vs LibreOffice's PDF of the PPTX.
for name, pdf in (("app", work / "deck.pdf"), ("lo", lo_pdf)):
    if pdf.exists():
        sh("pdftoppm", "-r", "144", "-png", str(pdf), str(work / name))


def page_png(prefix, i):
    hits = sorted(work.glob(f"{prefix}-*.png"), key=lambda p: int(p.stem.split("-")[-1]))
    return Image.open(hits[i - 1]).convert("RGB").resize((1920, 1080)) if len(hits) >= i else None


def diff(a, b):
    d = ImageChops.difference(a, b).convert("L")
    hist = d.histogram()
    return {"mean": round(ImageStat.Stat(d).mean[0], 2), "over64": round(sum(hist[65:]) / (1920 * 1080), 4)}


for i, sid in enumerate(shown, 1):
    ed = Image.open(work / f"slide-{i:02d}.png").convert("RGB")
    app, lop = page_png("app", i), page_png("lo", i)
    row = {"n": i, "id": sid}
    if app:
        row["editor_vs_pdf"] = diff(ed, app)
        # The PDF draws what the editor draws: under 2% of pixels differ by more than a quarter.
        if row["editor_vs_pdf"]["over64"] > 0.02:
            fail(f"{sid}: editor and PDF differ on {row['editor_vs_pdf']['over64']:.1%} of pixels")
    if app and lop:
        row["pdf_vs_libreoffice"] = diff(app, lop)
    report["slides"].append(row)
    sheet = Image.new("RGB", (3 * 640 + 40, 360 + 44), "white")
    draw = ImageDraw.Draw(sheet)
    for k, (label, im) in enumerate((("Editor / presentation (SVG)", ed), ("PDF export", app), ("PPTX in LibreOffice", lop))):
        x = 10 + k * 650
        draw.text((x, 8), f"{i}. {sid}: {label}", fill="black")
        if im:
            sheet.paste(im.resize((640, 360)), (x, 30))
            draw.rectangle((x - 1, 29, x + 640, 390), outline="#999999")
    sheet.save(ev / f"compare-{i:02d}.png", optimize=True)

report["failures"] = failures
(ev / "report.json").write_text(json.dumps(report, indent=2) + "\n")
print(json.dumps({k: report[k] for k in ("libreoffice", "lo_convert_pdf", "lo_roundtrip", "app_pdf_pages", "pptx_counts")}, indent=1))
for row in report["slides"]:
    print(row)
print("FAILURES:" if failures else "0 failures", *failures, sep="\n  ")
sys.exit(1 if failures else 0)
