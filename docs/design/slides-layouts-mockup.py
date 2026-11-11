"""Mockup for LAZYAGENTS-121: eight new slide layouts and the thumbnail layout picker.

python3 docs/design/slides-layouts-mockup.py > docs/design/slides-layouts.svg
"""

LIGHT = dict(background="#FFFFFF", text="#1F2328", heading="#111418", accent="#3B6FE0", accent2="#E8A33D", muted="#6B7280", surface="#F3F5F9")
DARK = dict(background="#14151A", text="#E6E7EB", heading="#FFFFFF", accent="#8B9CFF", accent2="#F2B35E", muted="#9AA0AE", surface="#1F2129")

# Placeholder frames in slide units (1920x1080), as they'll be in templates.go / schema.ts.
LAYOUTS = [
    ("title", "Title", [("title", 160, 300, 1600, 280), ("subtitle", 160, 600, 1600, 120)]),
    ("title-body", "Title and body", [("title", 120, 80, 1680, 160), ("body", 120, 280, 1680, 700)]),
    ("two-column", "Two columns", [("title", 120, 80, 1680, 160), ("body", 120, 280, 810, 700), ("body2", 990, 280, 810, 700)]),
    ("image-right", "Text and image", [("title", 120, 120, 800, 300), ("body", 120, 460, 800, 480), ("image", 1000, 120, 800, 840)]),
    ("section", "Section", [("title", 160, 380, 1600, 320)]),
    ("blank", "Blank", []),
    ("quote", "Quote", [("quote", 200, 220, 1520, 480), ("attribution", 200, 740, 1520, 100)]),
    ("big-number", "Big number", [("stat", 160, 220, 1600, 420), ("caption", 160, 680, 1600, 160)]),
    ("three-column", "Three columns", [("title", 120, 80, 1680, 160), ("body", 120, 280, 520, 700), ("body2", 700, 280, 520, 700), ("body3", 1280, 280, 520, 700)]),
    ("comparison", "Comparison", [("title", 120, 80, 1680, 160), ("label", 120, 280, 810, 90), ("label2", 990, 280, 810, 90), ("body", 120, 390, 810, 590), ("body2", 990, 390, 810, 590)]),
    ("image-full", "Full-bleed image", [("image", 0, 0, 1920, 1080), ("overlay", 120, 780, 1680, 200)]),
    ("image-left", "Image and text", [("image", 120, 120, 800, 840), ("title", 1000, 120, 800, 300), ("body", 1000, 460, 800, 480)]),
    ("agenda", "Agenda", [("title", 120, 80, 1680, 160), ("numbered", 120, 280, 1680, 700)]),
    ("closing", "Closing", [("closing", 160, 300, 1600, 280), ("contact", 160, 620, 1600, 140)]),
]
NEW = {"quote", "big-number", "three-column", "comparison", "image-full", "image-left", "agenda", "closing"}

out = []
w = out.append


def esc(s):
    return s.replace("&", "&amp;").replace("<", "&lt;").replace(">", "&gt;")


def thumb(x, y, tw, layout, th, ring=False):
    """A picker thumbnail: the placeholders drawn as bars in the theme's colours."""
    s = tw / 1920
    tht = 1080 * s
    w(f'<g transform="translate({x},{y})">')
    w(f'<rect width="{tw}" height="{tht:.0f}" rx="3" fill="{th["background"]}" stroke="{"#7C8CFF" if ring else "#3A3D48"}" stroke-width="{2.5 if ring else 1}"/>')
    for role, fx, fy, fw, fh in layout:
        X, Y, Wd, Ht = fx * s, fy * s, fw * s, fh * s
        if role == "image":
            w(f'<rect x="{X:.0f}" y="{Y:.0f}" width="{Wd:.0f}" height="{Ht:.0f}" fill="{th["muted"] if fw == 1920 else th["surface"]}" rx="{0 if fw == 1920 else 2}"/>')
            cx, cy = X + Wd / 2, Y + Ht / 2
            w(f'<path d="M{cx-8:.0f},{cy+5:.0f} l5,-7 l4,5 l3,-3 l4,5 z" fill="{th["background"] if fw == 1920 else th["muted"]}" opacity=".8"/>')
        elif role in ("title", "closing"):
            bw = Wd * (0.55 if fw > 1000 else 0.8)
            bx = X + (Wd - bw) / 2 if role == "closing" or (fw == 1600 and fy == 380) else X
            w(f'<rect x="{bx:.0f}" y="{Y + Ht/2 - 4:.0f}" width="{bw:.0f}" height="{max(5, Ht*0.28):.0f}" rx="1.5" fill="{th["heading"]}"/>')
        elif role == "overlay":
            w(f'<rect x="{X:.0f}" y="{Y + Ht - 14:.0f}" width="{Wd*0.5:.0f}" height="7" rx="1.5" fill="#FFFFFF"/>')
        elif role in ("subtitle", "attribution", "caption", "contact"):
            bw = Wd * 0.35
            bx = X + (Wd - bw) / 2 if role in ("caption", "contact") else X
            w(f'<rect x="{bx:.0f}" y="{Y + 4:.0f}" width="{bw:.0f}" height="4" rx="1" fill="{th["muted"]}"/>')
        elif role == "stat":
            w(f'<text x="{X + Wd/2:.0f}" y="{Y + Ht*0.8:.0f}" text-anchor="middle" font-size="{Ht*0.8:.0f}" font-weight="800" fill="{th["accent"]}">42%</text>')
        elif role == "quote":
            w(f'<text x="{X - 2:.0f}" y="{Y + 14:.0f}" font-size="22" font-family="Georgia,serif" fill="{th["accent"]}">“</text>')
            for i in range(3):
                w(f'<rect x="{X + 12:.0f}" y="{Y + 8 + i*9:.0f}" width="{(Wd-12)*(0.9 if i < 2 else 0.5):.0f}" height="5" rx="1" fill="{th["heading"]}"/>')
        elif role in ("label", "label2"):
            w(f'<rect x="{X:.0f}" y="{Y + 2:.0f}" width="{Wd*0.45:.0f}" height="5" rx="1" fill="{th["accent"]}"/>')
        else:  # body, body2, body3, numbered
            n = 4 if Ht > 30 else 2
            for i in range(n):
                ly = Y + 3 + i * 7
                if role == "numbered":
                    w(f'<text x="{X:.0f}" y="{ly + 4.5:.0f}" font-size="6" font-weight="700" fill="{th["accent"]}">{i+1}</text>')
                    w(f'<rect x="{X + 7:.0f}" y="{ly:.0f}" width="{Wd*0.45:.0f}" height="3.5" rx="1" fill="{th["text"]}" opacity=".7"/>')
                else:
                    w(f'<circle cx="{X + 1.8:.0f}" cy="{ly + 1.8:.0f}" r="1.3" fill="{th["text"]}"/>')
                    w(f'<rect x="{X + 6:.0f}" y="{ly:.0f}" width="{(Wd-6)*(0.85 - 0.12*(i % 3)):.0f}" height="3.5" rx="1" fill="{th["text"]}" opacity=".7"/>')
    w("</g>")


def picker(x, y, th, current, title):
    cols, tw, gx, gy = 4, 112, 12, 34
    ph = 44 + 4 * (63 + gy) + 6
    pw = cols * tw + (cols - 1) * gx + 28
    w(f'<g transform="translate({x},{y})">')
    w(f'<rect width="{pw}" height="{ph}" rx="10" fill="#17181E" stroke="#2E3140"/>')
    w(f'<text x="14" y="26" font-size="13" font-weight="600" fill="#E6E7EB">{esc(title)}</text>')
    w(f'<text x="{pw-14}" y="26" text-anchor="end" font-size="11.5" fill="#8A8F9C">Theme: {"Light" if th is LIGHT else "Dark"}</text>')
    for i, (lid, label, ph_) in enumerate(LAYOUTS):
        cx, cy = 14 + (i % cols) * (tw + gx), 40 + (i // cols) * (63 + gy)
        thumb(cx, cy, tw, ph_, th, ring=lid == current)
        w(f'<text x="{cx}" y="{cy + 63 + 15}" font-size="11.5" fill="{"#FFFFFF" if lid == current else "#B8BCC8"}" font-weight="{600 if lid == current else 400}">{esc(label)}</text>')
        if lid in NEW:
            w(f'<rect x="{cx + tw - 26}" y="{cy + 4}" width="22" height="12" rx="6" fill="#7C8CFF"/><text x="{cx + tw - 15}" y="{cy + 13}" text-anchor="middle" font-size="8" font-weight="700" fill="#0E0F14">NEW</text>')
    w("</g>")
    return pw, ph


def slide(x, y, sw, lid, th, sample, empty_image=False):
    """A full-size rendering of a layout filled with sample content."""
    s = sw / 1920
    w(f'<g transform="translate({x},{y}) scale({s})">')
    w(f'<rect width="1920" height="1080" fill="{th["background"]}"/>')
    w(sample(th, empty_image))
    w("</g>")
    w(f'<rect x="{x}" y="{y}" width="{sw}" height="{1080*s:.0f}" fill="none" stroke="#2E3140"/>')


def T(x, y, size, txt, fill, weight=400, anchor="start", family="Arial", style="normal"):
    return f'<text x="{x}" y="{y}" font-size="{size}" font-weight="{weight}" fill="{fill}" text-anchor="{anchor}" font-family="{family}" font-style="{style}">{esc(txt)}</text>'


def bullets(x, y, items, th, size=44, numbered=False, gap=78):
    r = []
    for i, it in enumerate(items):
        mark = f"{i+1}." if numbered else "•"
        r.append(T(x, y + i * gap, size, mark, th["accent"] if numbered else th["text"], 700 if numbered else 400))
        r.append(T(x + (70 if numbered else 50), y + i * gap, size, it, th["text"]))
    return "".join(r)


def photo(x, y, wd, ht):
    return (f'<rect x="{x}" y="{y}" width="{wd}" height="{ht}" fill="url(#sky)"/>'
            f'<path d="M{x},{y+ht} L{x+wd*0.3},{y+ht*0.55} L{x+wd*0.5},{y+ht*0.75} L{x+wd*0.75},{y+ht*0.4} L{x+wd},{y+ht*0.7} L{x+wd},{y+ht} Z" fill="#2B4A3A"/>')


def empty_img(x, y, wd, ht, dark_bg=False):
    c = "#9AA0AE"
    return (f'<rect x="{x}" y="{y}" width="{wd}" height="{ht}" fill="{"#2A2D36" if dark_bg else "#F3F5F9"}" stroke="{c}" stroke-width="6" stroke-dasharray="28 18"/>'
            + T(x + wd / 2, y + ht / 2, 52, "▨  Click to add image", c, 600, "middle"))


SAMPLES = {
    "quote": lambda th, e: T(170, 330, 220, "“", th["accent"], 700, family="Georgia")
    + T(260, 400, 72, "Simple things should be simple;", th["heading"], 400, family="Georgia", style="italic")
    + T(260, 500, 72, "complex things should be possible.", th["heading"], 400, family="Georgia", style="italic")
    + T(260, 800, 44, "— Alan Kay", th["muted"]),
    "big-number": lambda th, e: T(960, 590, 340, "42%", th["accent"], 800, "middle")
    + T(960, 760, 56, "fewer support tickets after launch", th["muted"], 400, "middle"),
    "three-column": lambda th, e: T(136, 200, 80, "Three ways to ship", th["heading"], 700)
    + bullets(136, 360, ["Plan", "Small PRs", "Review"], th)
    + bullets(716, 360, ["Build", "Feature flags", "Tests"], th)
    + bullets(1296, 360, ["Ship", "Stage first", "Monitor"], th),
    "comparison": lambda th, e: T(136, 200, 80, "Before and after", th["heading"], 700)
    + T(136, 350, 52, "Before", th["accent"], 700) + T(1006, 350, 52, "After", th["accent"], 700)
    + bullets(136, 470, ["Manual deploys", "Weekly releases", "Hand-run tests"], th)
    + bullets(1006, 470, ["One-click deploys", "Daily releases", "CI on every PR"], th),
    "image-full": lambda th, e: (empty_img(0, 0, 1920, 1080, True) if e else photo(0, 0, 1920, 1080))
    + T(136, 960, 96, "Field trip, spring 2026", "#FFFFFF", 700),
    "image-left": lambda th, e: (empty_img(120, 120, 800, 840) if e else photo(120, 120, 800, 840))
    + T(1016, 220, 72, "The new office", th["heading"], 700)
    + T(1016, 520, 40, "Opens in March with room", th["text"]) + T(1016, 580, 40, "for forty people.", th["text"]),
    "agenda": lambda th, e: T(136, 200, 80, "Agenda", th["heading"], 700)
    + bullets(136, 370, ["Where we are", "What we learned", "What's next", "Questions"], th, 52, True, 100),
    "closing": lambda th, e: T(960, 540, 128, "Thank you", th["heading"], 700, "middle")
    + T(960, 680, 44, "ada@example.com · vesper.dev", th["muted"], 400, "middle"),
}

W_, H_ = 1800, 1540
w(f'<svg xmlns="http://www.w3.org/2000/svg" font-family="Inter, \'Helvetica Neue\', Arial, sans-serif" width="{W_}" height="{H_}" viewBox="0 0 {W_} {H_}" role="img" aria-label="Slides: eight new layouts and the thumbnail layout picker">')
w("<title>Slides: eight new layouts and the thumbnail layout picker</title>")
w('<defs><linearGradient id="sky" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="#1F5A9F"/><stop offset="1" stop-color="#559FA0"/></linearGradient></defs>')
w(f'<rect width="{W_}" height="{H_}" fill="#07060B"/>')
w(T(40, 52, 26, "Slides · new layouts and the layout picker (LAZYAGENTS-121)", "#FFFFFF", 700))
w(T(40, 80, 14, "Left: the toolbar's layout menu becomes a thumbnail grid drawn from each layout's placeholders in the deck theme. Right: the eight new layouts with sample content.", "#9AA0AE"))

# Toolbar strip with the open menu
w('<g transform="translate(40,104)"><rect width="590" height="44" rx="8" fill="#111218" stroke="#2E3140"/>')
for i, (lbl, xx) in enumerate([("↶", 10), ("↷", 40), ("T Text", 78), ("◻ Shape ▾", 142), ("▨ Image", 232), ("╱ Line", 310)]):
    w(T(xx + 8, 28, 13, lbl, "#C9CCD6"))
w('<rect x="380" y="7" width="120" height="30" rx="6" fill="#262A3A" stroke="#7C8CFF"/>' + T(392, 28, 13, "▤ Quote ▾", "#FFFFFF", 600) + "</g>")
pw, ph = picker(40, 158, LIGHT, "quote", "Slide layout")
w(T(40, 158 + ph + 22, 12.5, "Click a layout to switch this slide; content moves to matching placeholders (title→title, body→body…).", "#9AA0AE"))
w(T(40, 158 + ph + 40, 12.5, "Keyboard: arrows move between tiles, Enter applies, Esc closes. Current layout is ringed.", "#9AA0AE"))

# Same picker, dark theme, opened from "+ New slide ▾"
y2 = 158 + ph + 70
w(f'<g transform="translate(40,{y2})"><rect width="160" height="32" rx="6" fill="none" stroke="#5A5F70" stroke-dasharray="4 3"/>' + T(14, 21, 13, "+ New slide", "#C9CCD6") + '<line x1="118" y1="6" x2="118" y2="26" stroke="#5A5F70"/>' + T(130, 21, 13, "▾", "#C9CCD6") + "</g>")
w(T(214, y2 + 34, 12.5, "the button alone repeats the current layout, as today.", "#9AA0AE"))
w(T(214, y2 + 16, 12.5, "▾ on “+ New slide” opens the same grid (dark deck shown);", "#9AA0AE"))
_, ph2 = picker(40, y2 + 48, DARK, "agenda", "New slide")

# Right: the eight layouts
rx, ry, sw, gx, gy = 680, 112, 532, 26, 52
order = ["quote", "big-number", "three-column", "comparison", "image-full", "image-left", "agenda", "closing"]
labels = {lid: lbl for lid, lbl, _ in LAYOUTS}
for i, lid in enumerate(order):
    cx, cy = rx + (i % 2) * (sw + gx), ry + (i // 2) * (sw * 1080 / 1920 + gy)
    w(T(cx, cy - 8, 13, f"{labels[lid]}  ·  {lid}", "#E6E7EB", 600))
    slide(cx, cy, sw, lid, LIGHT, SAMPLES[lid], empty_image=False)

# Empty image placeholder callout
ey, rx = y2 + 48 + ph2 + 40, 40
w(T(rx, ey, 13, "Empty image placeholders (editor only)", "#E6E7EB", 600))
sm = 300
slide(rx, ey + 10, sm, "image-left", LIGHT, SAMPLES["image-left"], empty_image=True)
slide(rx + sm + 20, ey + 10, sm, "image-full", LIGHT, SAMPLES["image-full"], empty_image=True)
for k, line in enumerate([
    "Click the dashed box to pick a PNG/JPEG; it fills the frame (cropped to cover)",
    "and keeps the image role, so layout switches move it with the rest.",
    "Empty, the box is never shown in presentation, PDF or PPTX.",
    "Text and image (image-right) gains the same image placeholder.",
    "New roles: body3 (third column), label / label2 (comparison headings).",
]):
    w(T(rx, ey + 10 + sm * 9 / 16 + 26 + k * 20, 12.5, line, "#9AA0AE"))
w("</svg>")
print("".join(out))
