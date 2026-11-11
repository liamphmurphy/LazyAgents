"""Compares the app's PDF of the preset-shapes slide with LibreOffice's render
of the app's PPTX. Runs inside the container compare.sh starts:
  python3 compare.py <dir>   (dir holds what web/scripts/slides-shapes-evidence.mjs wrote)
Writes report.json and PNGs next to the inputs; exits 1 if a shape differs.
"""
import json
import math
import subprocess
import sys

from PIL import Image, ImageChops, ImageOps

d = sys.argv[1]
MIN_IOU = 0.95  # outlines agree to within anti-aliasing and stroke joins


def run(*cmd):
    p = subprocess.run(cmd, capture_output=True, text=True, cwd=d)
    if p.returncode:
        sys.exit(f"{cmd[0]} failed: {p.stderr}")
    return p.stdout + p.stderr


report = {"libreoffice": run("soffice", "--version").strip(), "shapes": [], "failures": []}
for name in ("shapes", "shapes-bare"):
    log = run("soffice", "--headless", "--convert-to", "pdf:impress_pdf_Export", "--outdir", "lo", f"{name}.pptx")
    report[f"{name}.pptx"] = log.strip()
    if "Error" in log:
        report["failures"].append(f"LibreOffice reported an error opening {name}.pptx")
    run("pdftoppm", "-r", "144", "-png", "-singlefile", f"{name}.pdf", f"{name}-app")
    run("pdftoppm", "-r", "144", "-png", "-singlefile", f"lo/{name}.pdf", f"{name}-lo")


def mask(img, box):
    g = ImageOps.invert(img.crop(box).convert("L"))  # white background → 0
    return [v > 40 for v in g.getdata()]


def page(path):
    # LibreOffice's 13.333 in page rounds to 1921 px at 144 dpi; trim to the slide.
    img = Image.open(path).convert("RGB")
    assert abs(img.size[0] - 1920) <= 1 and abs(img.size[1] - 1080) <= 1, img.size
    return img.crop((0, 0, 1920, 1080))


app, lo = page(f"{d}/shapes-bare-app.png"), page(f"{d}/shapes-bare-lo.png")
for e in json.load(open(f"{d}/frames.json")):
    f, r = e["frame"], math.radians(e["rotation"])
    # The rotated frame's bounds, plus room for a callout's tail and the stroke.
    cx, cy = f["x"] + f["w"] / 2, f["y"] + f["h"] / 2
    hw = (abs(f["w"] * math.cos(r)) + abs(f["h"] * math.sin(r))) / 2 + 8
    hh = (abs(f["w"] * math.sin(r)) + abs(f["h"] * math.cos(r))) / 2 + 8 + 0.15 * f["h"]
    box = tuple(round(v) for v in (cx - hw, cy - hh, cx + hw, cy + hh))
    a, b = mask(app, box), mask(lo, box)
    inter = sum(x and y for x, y in zip(a, b))
    union = sum(x or y for x, y in zip(a, b))
    iou = inter / union if union else 0
    report["shapes"].append({"shape": e["shape"], "iou": round(iou, 4)})
    if iou < MIN_IOU:
        report["failures"].append(f"{e['shape']}: IoU {iou:.3f} < {MIN_IOU}")

# Evidence images at half size: app over LibreOffice, and their difference.
for name in ("shapes", "shapes-bare"):
    a, b = page(f"{d}/{name}-app.png"), page(f"{d}/{name}-lo.png")
    pair = Image.new("RGB", (1920, 2160 + 20), "#888888")
    pair.paste(a, (0, 0))
    pair.paste(b, (0, 1080 + 20))
    pair.resize((960, 1090)).save(f"{d}/{name}-app-vs-libreoffice.png", optimize=True)
    ImageOps.invert(ImageChops.difference(a, b)).resize((960, 540)).save(f"{d}/{name}-diff.png", optimize=True)

json.dump(report, open(f"{d}/report.json", "w"), indent=2)
print(json.dumps(report, indent=2))
sys.exit(1 if report["failures"] else 0)
