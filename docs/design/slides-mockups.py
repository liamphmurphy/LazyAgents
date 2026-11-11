# Generator for the Slides mockups (LAZYAGENTS-103): python3 docs/design/slides-mockups.py docs/design
"""Generates the Slides mockups (LAZYAGENTS-103) as self-contained SVGs."""
import sys

OUT = sys.argv[1]
import re as _re
_open = open


def open(path, mode="r"):  # noqa: A001 - round coordinates on write
    f = _open(path, mode)
    if "w" not in mode:
        return f
    class W:
        def write(self, data):
            f.write(_re.sub(r"\d+\.\d+", lambda m: (f"{float(m.group()):.4f}" if float(m.group()) < 1 else f"{float(m.group()):.1f}").rstrip("0").rstrip("."), data)); f.close()
    return W()
T = dict(bg="#0C0B12", side="#100F18", panel="#0E0D15", surf="#15141F", raised="#1B1A27", sunk="#110F19", hover="#1A1925", sel="#201E2C",
         border="#24222F", sub="#1E1C29", strong="#2E2B3B", text="#EEEDF3", t2="#CBC9D6", sec="#A4A1B3", ter="#8C899C", acc="#9B8CFF",
         onacc="#120F24", human="#F59E5B", humfg="#F8B98A", block="#EF6461", blkfg="#F49390", done="#4CB782", donefg="#7FD3A8", scrim="rgba(5,5,7,0.62)")
F = "Inter, 'Helvetica Neue', Arial, sans-serif"
M = "JetBrains Mono, monospace"


def esc(s):
    return s.replace("&", "&amp;").replace("<", "&lt;").replace(">", "&gt;")


def paint(attr, c):
    if c and c.startswith("rgba("):
        r, g, b, a = [v.strip() for v in c[5:-1].split(",")]
        return f' {attr}="#{int(r):02X}{int(g):02X}{int(b):02X}" {attr}-opacity="{a}"'
    return f' {attr}="{c}"'


def rect(x, y, w, h, fill="none", stroke=None, r=0, sw=1, extra=""):
    s = (paint("stroke", stroke) + f' stroke-width="{sw}"') if stroke else ""
    return f'<rect x="{x}" y="{y}" width="{w}" height="{h}" rx="{r}"{paint("fill", fill)}{s} {extra}/>'
    return f'<rect x="{x}" y="{y}" width="{w}" height="{h}" rx="{r}" fill="{fill}"{s} {extra}/>'


def text(x, y, s, size=13, fill=T["text"], weight=400, anchor="start", font=F, extra=""):
    return f'<text x="{x}" y="{y}" font-family="{font}" font-size="{size}" font-weight="{weight}" fill="{fill}" text-anchor="{anchor}" {extra}>{esc(s)}</text>'


def line(x1, y1, x2, y2, stroke=T["sub"], sw=1, extra=""):
    return f'<line x1="{x1}" y1="{y1}" x2="{x2}" y2="{y2}" stroke="{stroke}" stroke-width="{sw}" {extra}/>'


def button(x, y, label, kind="secondary", w=None, icon=None, h=30):
    w = w or (len(label) * 7 + 26 + (16 if icon else 0))
    fill, st, fg, wt = {"primary": (T["acc"], None, T["onacc"], 600), "secondary": ("#1E1E24", "#3A3A44", T["text"], 500),
                        "outline": ("none", T["strong"], T["t2"], 500), "ghost": ("none", None, T["sec"], 500),
                        "approve": (T["human"], None, "#1A1206", 600), "danger": ("none", T["strong"], T["blkfg"], 500)}[kind]
    o = rect(x, y, w, h, fill, st, 7)
    tx = x + 13
    if icon:
        o += text(tx, y + h / 2 + 4.5, icon, 13, fg, 600)
        tx += 16
    return o + text(tx, y + h / 2 + 4.5, label, 13, fg, wt), w


def note(x, y, n, s, w=260):
    """A numbered annotation callout (amber on the margin): design notes, not UI."""
    o = f'<circle cx="{x}" cy="{y}" r="10" fill="#F2A97E"/>' + text(x, y + 4, str(n), 11, "#1A1206", 700, "middle")
    return o


def ann_list(x, y, items, w=300, title="Notes"):
    o = rect(x, y, w, 24 + 18 * sum(len(wrap(i, w - 44)) for i in items) + 8 * len(items), "#1A1520", "rgba(242,169,126,0.35)", 10)
    o += text(x + 12, y + 18, title, 11.5, "#F2A97E", 600)
    yy = y + 40
    for n, it in enumerate(items, 1):
        o += f'<circle cx="{x + 20}" cy="{yy - 4}" r="8" fill="#F2A97E"/>' + text(x + 20, yy, str(n), 10, "#1A1206", 700, "middle")
        for ln in wrap(it, w - 44):
            o += text(x + 34, yy, ln, 11.5, T["t2"])
            yy += 18
        yy += 8
    return o


def wrap(s, w, cw=6.2):
    words, lines, cur = s.split(), [], ""
    for wd in words:
        if len(cur + " " + wd) * cw > w and cur:
            lines.append(cur)
            cur = wd
        else:
            cur = (cur + " " + wd).strip()
    lines.append(cur)
    return lines


# ---- miniature slides of the acceptance deck (1920x1080 units scaled into w) ----
USED = set()


def slide(n, x, y, w, sel=False, extra=""):
    k = w / 1920
    h = 1080 * k
    body = slide_symbol(n).split(">", 1)[1].rsplit("</symbol>", 1)[0]
    o = f'<g transform="translate({x},{y}) scale({k})">{body}{extra}</g>'
    return o + rect(x, y, w, h, "none", T["acc"] if sel else T["border"], 4, 2 if sel else 1)


def slide_symbol(n):
    g = f'<symbol id="slide{n}" viewBox="0 0 1920 1080" width="1920" height="1080">'
    if n == 1:
        g += rect(0, 0, 1920, 1080, "#F3F5F9") + rect(160, 320, 200, 12, "#3B6FE0")
        g += text(176, 540, "Quarterly review", 112, "#111418", 700, font="Arial") + text(176, 660, "Slides acceptance deck · v1 schema", 44, "#6B7280", font="Arial")
    elif n == 2:
        g += rect(0, 0, 1920, 1080, "#FFFFFF") + text(136, 200, "What changed this quarter", 80, "#111418", 700, font="Arial")
        for i, (lv, t, yy) in enumerate([(0, "Revenue grew 18% against a plan of 12%, driven by…", 340), (1, "Churn held steady; expansion did the work.", 400),
                                          (0, "This is a deliberately long bullet so the shared", 460), (0, "layout has to wrap it across more than one line", 518), (0, "Underlined and coloured runs survive export.", 640)]):
            xx = 136 + 60 * lv
            if not (i == 3):
                g += text(xx, yy, "1." if i == 4 else "•", 48, "#1F2328", font="Arial")
            g += text(xx + 60, yy, t, 48 if lv == 0 else 40, "#1F2328", font="Arial")
    elif n == 3:
        g += rect(0, 0, 1920, 1080, "#FFFFFF") + text(136, 230, "Images are cropped,", 72, "#111418", 700, font="Arial") + text(136, 318, "not squashed", 72, "#111418", 700, font="Arial")
        for i, t in enumerate(["The photo is 3:2; its frame is nearly square.", "A crop rectangle in source fractions keeps", "the sun and trims the red border on the left."]):
            g += text(136, 520 + i * 48, t, 40, "#1F2328", font="Arial")
        g += rect(1000, 120, 800, 840, "url(#sky)") + '<circle cx="1590" cy="350" r="150" fill="#F9C752"/>' + '<polygon points="1000,950 1030,620 1260,960" fill="#2B4A3E"/><polygon points="1000,960 1440,435 1800,860 1800,960" fill="#3E6652"/>'
    elif n == 4:
        g += rect(0, 0, 1920, 1080, "#FFFFFF") + rect(120, 160, 520, 300, "#3B6FE0", r=32) + text(380, 300, "Editable text in a", 48, "#FFFFFF", 700, "middle", font="Arial") + text(380, 356, "shape", 48, "#FFFFFF", 700, "middle", font="Arial")
        g += '<ellipse cx="960" cy="310" rx="200" ry="150" fill="#E8A33D" stroke="#7C4A03" stroke-width="6"/>'
        g += '<g transform="rotate(8 1540 310)">' + rect(1280, 160, 520, 300, "#F3F5F9", "#6B7280", 0, 4) + text(1300, 220, "Rotated 8°", 40, "#1F2328", font="Arial") + "</g>"
        g += line(120, 600, 1780, 600, "#111418", 8) + '<polygon points="1772,580 1800,600 1772,620" fill="#111418"/>'
        for i, t in enumerate(["This box is too small for the amount", "of text that has been typed into it, so", "the layout reports an overflow instead", "of shrinking it."]):
            g += text(136, 760 + i * 48, t, 40, "#1F2328", font="Arial")
    elif n == 5:
        g += rect(0, 0, 1920, 1080, "#111418") + text(960, 560, "Questions?", 128, "#FFFFFF", 700, "middle", font="Arial") + text(960, 640, "mono: deck.schema = 1", 40, "#E8A33D", anchor="middle", font="Courier New")
    elif n == 6:  # persona's new slide
        g += rect(0, 0, 1920, 1080, "#FFFFFF") + text(136, 200, "Next quarter", 80, "#111418", 700, font="Arial")
        for i, t in enumerate(["Launch the team plan in March", "Hire two support engineers", "Cut onboarding time in half"]):
            g += text(136, 360 + i * 80, "•", 48, "#1F2328", font="Arial") + text(196, 360 + i * 80, t, 48, "#1F2328", font="Arial")
    return g + "</symbol>"


def sidebar(h, active="Apps"):
    o = rect(0, 0, 232, h, T["side"]) + line(232, 0, 232, h, T["sub"])
    o += rect(12, 14, 22, 22, T["acc"], r=6) + text(23, 30, "V", 12, T["onacc"], 700, "middle") + text(44, 30, "Vesper", 13.5, T["text"], 600)
    o += rect(12, 50, 208, 32, T["surf"], T["border"], 7) + text(24, 70, "Search…", 12.5, T["ter"]) + text(206, 70, "⌘K", 11, T["ter"], anchor="end", font=M)
    y = 104
    for item in ["Home", "Chat", "Inbox", "Tickets", "Epics", "Agents", "Library", "Apps"]:
        if item == active:
            o += rect(10, y - 19, 212, 30, T["sel"], r=6)
        o += rect(22, y - 10, 14, 14, "none", T["sec"] if item != active else T["text"], 3, 1.5) + text(46, y + 1, item, 13, T["text"] if item == active else T["sec"], 500 if item == active else 400)
        y += 32
    o += text(22, y + 14, "Apps", 12, T["ter"], 500)
    for i, (a, act) in enumerate([("Docs", False), ("Slides", active == "Apps"), ("Finance", False)]):
        yy = y + 42 + 30 * i
        if act:
            o += rect(10, yy - 19, 212, 30, T["hover"], r=6)
        o += text(46, yy + 1, a, 13, T["text"] if act else T["sec"])
    o += line(0, h - 56, 232, h - 56, T["sub"]) + '<circle cx="30" cy="%d" r="12" fill="#2A2838"/>' % (h - 28) + text(30, h - 24, "LM", 9.5, T["t2"], 600, "middle") + text(50, h - 24, "Liam Murphy", 12.5, T["t2"])
    return o


def topbar(x, w, crumbs, right=""):
    o = rect(x, 0, w, 52, T["bg"]) + line(x, 52, x + w, 52, T["sub"])
    xx = x + 20
    for i, c in enumerate(crumbs):
        last = i == len(crumbs) - 1
        o += text(xx, 31, c, 13, T["text"] if last else T["sec"], 500 if last else 400)
        xx += len(c) * 7.2 + 8
        if not last:
            o += text(xx, 31, "/", 13, T["ter"])
            xx += 14
    return o + right


NOTES_W = 340


def svg(w, h, body, title):
    tw = w + NOTES_W + 20
    syms = "".join(slide_symbol(n) for n in sorted(USED))
    USED.clear()
    return (f'<svg xmlns="http://www.w3.org/2000/svg" width="{tw}" height="{h}" viewBox="0 0 {tw} {h}" role="img" aria-label="{esc(title)}">'
            f'<title>{esc(title)}</title>{rect(0, 0, tw, h, "#07060B")}<defs><clipPath id="screen"><rect width="{w}" height="{h}"/></clipPath>'
            f'<linearGradient id="sky" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="#1F5A9F"/><stop offset="1" stop-color="#559FA0"/></linearGradient>{syms}<filter id="sh" x="-20%" y="-20%" width="140%" height="140%"><feDropShadow dx="0" dy="8" stdDeviation="12" flood-color="#000" flood-opacity="0.45"/></filter></defs>'
            f'<g clip-path="url(#screen)">{rect(0, 0, w, h, T["bg"])}{body[0]}</g>{body[1]}</svg>')


def notes_col(w, items, title):
    return "\x00" + ann_list(w + 20, 20, items, NOTES_W - 20, title)


def chat_panel(x, y, w, h, persona="Ada", msgs=None, busy=False):
    o = rect(x, y, w, h, T["side"]) + line(x + w, y, x + w, y + h, T["sub"])
    o += rect(x + 16, y + 14, 26, 26, "#2B2366", r=8) + text(x + 29, y + 32, persona[0], 12, "#C9BEFF", 700, "middle")
    o += text(x + 50, y + 26, persona, 13, T["text"], 600) + text(x + 50, y + 42, "Persona · this deck's chat", 11.5, T["ter"])
    o += line(x, y + 54, x + w, y + 54)
    yy = y + 80
    for who, body in (msgs or []):
        lines = wrap(body, w - 70 if who == "me" else w - 48, 6.6)
        bh = 22 * len(lines) + 16
        if who == "me":
            o += rect(x + 50, yy, w - 66, bh, T["surf"], T["border"], 12)
            for i, l in enumerate(lines):
                o += text(x + 64, yy + 26 + 22 * i, l, 13.5, T["t2"])
        elif who == "tool":
            o += rect(x + 16, yy, w - 32, 30, T["sunk"], T["sub"], 8) + text(x + 28, yy + 19, body, 11.5, T["sec"], font=M)
            bh = 30
        else:
            for i, l in enumerate(lines):
                o += text(x + 16, yy + 18 + 22 * i, l, 14, T["t2"])
            bh = 22 * len(lines)
        yy += bh + 14
    if busy:
        o += text(x + 16, yy + 10, "● ● ●  Ada is editing the deck…", 12.5, T["ter"])
    o += rect(x + 12, y + h - 104, w - 24, 90, T["surf"], T["strong"], 12) + text(x + 26, y + h - 76, "Ask Ada to change this deck…", 13.5, T["ter"])
    o += rect(x + w - 50, y + h - 50, 28, 28, T["acc"], r=7) + text(x + w - 36, y + h - 31, "↑", 14, T["onacc"], 700, "middle")
    return o


# ======================= 1. Deck list + new deck =======================
def deck_list():
    W, H = 1440, 900
    o = sidebar(H) + topbar(232, W - 232, ["Apps", "Slides"])
    cx = 232 + (W - 232 - 980) / 2
    o += text(cx, 104, "Slides", 26, T["text"], 600, extra='letter-spacing="-0.5"') + text(cx, 128, "Decks you build and present with a persona. Each deck keeps its own chat.", 13, T["sec"])
    # new deck form (mirrors Docs) + theme choice
    o += rect(cx, 150, 980, 40, T["surf"], T["acc"], 12, 1.5) + text(cx + 14, 175, "Start a deck: give it a title, or describe it and Ada drafts the slides…", 13.5, T["ter"])
    o += rect(cx + 724, 156, 140, 28, T["sunk"], T["strong"], 7) + rect(cx + 732, 163, 22, 14, "#FFFFFF", T["strong"], 2) + text(cx + 762, 175, "Theme: Light", 12, T["t2"]) + text(cx + 852, 175, "▾", 11, T["ter"])
    b, _ = button(cx + 872, 155, "New deck", "primary", 102, "+")
    o += b
    o += note(cx + 992, 170, 1, "")
    # grid of deck cards
    o += text(cx, 226, "Recent", 12.5, T["sec"], 500) + text(cx + 980, 226, "Sorted by last edited", 12, T["ter"], anchor="end")
    decks = [("Quarterly review", "Edited 4 min ago · 5 slides", 1), ("Board update: November", "Edited yesterday · 12 slides", 2), ("Hiring plan 2027", "Edited 3 days ago · 8 slides", 4),
             ("Onboarding for new agents", "Edited last week · 15 slides", 5), ("Untitled deck", "Edited 2 weeks ago · 1 slide", 0), ("Design review", "Edited Oct 2 · 6 slides", 3)]
    for i, (t, m, n) in enumerate(decks):
        x = cx + (i % 3) * 332
        y = 240 + (i // 3) * 262
        focus = i == 1
        o += rect(x, y, 316, 246, T["surf"], T["acc"] if focus else T["border"], 12, 2 if focus else 1)
        if focus:
            o += rect(x - 3, y - 3, 322, 252, "none", "rgba(155,140,255,0.35)", 14, 2)
        if n:
            o += slide(n, x + 12, y + 12, 292)
        else:
            o += rect(x + 12, y + 12, 292, 164, "#FFFFFF", T["border"], 4) + text(x + 158, y + 98, "Click to add a title", 13, "#9CA3AF", anchor="middle")
        o += text(x + 14, y + 202, t, 13.5, T["text"], 500) + text(x + 14, y + 222, m, 12, T["ter"])
        o += rect(x + 280, y + 192, 26, 26, T["hover"] if i == 1 else "none", r=6) + text(x + 293, y + 210, "⋯", 14, T["sec"], 700, "middle")
    # open card menu on deck 2
    mx, my = cx + 332 + 196, 240 + 222
    o += f'<g filter="url(#sh)">' + rect(mx, my, 176, 150, T["raised"], "#2A2A31", 10) + "</g>"
    for i, (lbl, danger) in enumerate([("Open", False), ("Rename", False), ("Duplicate", False), ("Present", False), ("Delete…", True)]):
        if i == 2:
            o += rect(mx + 4, my + 6 + i * 28, 168, 26, T["hover"], r=6)
        o += text(mx + 14, my + 24 + i * 28, lbl, 13, T["blkfg"] if danger else T["text"])
    o += note(mx + 190, my + 14, 2, "")
    # states strip
    sy = 790
    o += line(232, sy - 22, W, sy - 22, T["sub"]) + text(cx, sy - 4, "States", 12, "#F2A97E", 600)
    # empty
    ex = cx
    o += rect(ex, sy + 6, 316, 92, T["surf"], T["border"], 12) + rect(ex + 14, sy + 20, 32, 32, T["sel"], T["strong"], 8) + text(ex + 30, sy + 41, "▭", 14, T["sec"], 600, "middle")
    o += text(ex + 58, sy + 33, "No decks yet", 13, T["text"], 600) + text(ex + 58, sy + 51, "Start one above, or ask for a deck in", 12, T["sec"]) + text(ex + 58, sy + 67, "any chat: it shows up here too.", 12, T["sec"])
    o += text(ex + 14, sy + 88, "Empty", 11, T["ter"], font=M)
    # loading
    lx = cx + 332
    o += rect(lx, sy + 6, 316, 92, T["surf"], T["border"], 12, extra='aria-busy="true"') + rect(lx + 14, sy + 18, 120, 66, "#1F1D2B", r=4) + rect(lx + 146, sy + 26, 140, 10, "#1F1D2B", r=4) + rect(lx + 146, sy + 46, 90, 10, "#1F1D2B", r=4)
    o += text(lx + 146, sy + 88, "Loading: card skeletons", 11, T["ter"], font=M)
    # error
    rx = cx + 664
    o += rect(rx, sy + 6, 316, 92, "rgba(239,100,97,0.06)", "rgba(239,100,97,0.25)", 12) + text(rx + 14, sy + 30, "Couldn’t load your decks", 13, T["blkfg"], 600) + text(rx + 14, sy + 50, "The server didn’t answer (502). Your decks", 12, T["sec"]) + text(rx + 14, sy + 66, "are safe.", 12, T["sec"])
    b, _ = button(rx + 236, sy + 58, "Retry", "secondary", 66, None, 26)
    o += b + text(rx + 14, sy + 88, "Error", 11, T["ter"], font=M)
    notes = ["The title field mirrors Docs. Enter or New deck creates a deck with a title slide in the chosen theme and opens the editor. If the text reads as a description rather than a title, it becomes Ada's first chat message and she drafts the slides.",
             "The card menu is a Radix dropdown and keyboard reachable: Tab to the card, then ⋯ or Shift+F10. Rename edits the card title inline. Duplicate makes “Copy of …” with new IDs and shared images. Delete… asks for confirmation and names the deck.",
             "Focus ring: 2px accent plus a 2px accent halo at 35%. Arrow keys move between cards; Enter opens a deck.",
             "Narrow (<800px): the sidebar stacks above, the grid becomes one column, and the theme picker moves under the title field."]
    o += notes_col(W, notes, "Design notes · deck list")
    return svg(W, H, o.split("\x00"), "Slides: deck list, new deck and list states")


open(f"{OUT}/slides-deck-list.svg", "w").write(deck_list())


# ======================= shared editor chrome =======================
def rail(h):
    o = rect(0, 0, 56, h, T["side"]) + line(56, 0, 56, h, T["sub"]) + rect(17, 14, 22, 22, T["acc"], r=6) + text(28, 30, "V", 12, T["onacc"], 700, "middle")
    for i in range(7):
        o += rect(19, 66 + i * 40, 18, 18, T["sel"] if i == 6 else "none", T["sec"], 4, 1.5)
    return o


def ed_top(W, state="Saved · v12", state_color=None, presence="Ada can edit", live=False, chat_open=True, extra_right=None):
    o = topbar(56, W - 56, ["Apps", "Slides", "Quarterly review"])
    x = W - 20
    b, w = button(0, 0, "Hide chat" if chat_open else "Chat with Ada", "secondary" if chat_open else "primary", None, "✦")
    x -= w
    o += f'<g transform="translate({x},11)">{b}</g>'
    b, w = button(0, 0, "Export", "secondary", None, "↓")
    x -= w + 8
    o += f'<g transform="translate({x},11)">{b}</g>'
    b, w = button(0, 0, "Present", "primary", None, "▶")
    x -= w + 8
    o += f'<g transform="translate({x},11)">{b}</g>'
    x -= 12
    o += text(x, 31, state, 12, state_color or T["ter"], anchor="end")
    x -= len(state) * 6.8 + 16
    o += text(x, 31, presence, 12, T["ter"], anchor="end")
    x -= len(presence) * 6.8 + 8
    op = 'opacity="0.6"' if live else ""
    o += f'<circle cx="{x - 9}" cy="27" r="9" fill="#2B2366" {op}/>' + text(x - 9, 31, "A", 10, "#C9BEFF", 700, "middle")
    return o, extra_right


def thumbs(x, y, h, sel=2, warn=(4,), changed=(), new_after=None):
    o = rect(x, y, 172, h, T["panel"]) + line(x + 172, y, x + 172, y + h, T["sub"])
    o += text(x + 14, y + 24, "Slides", 12, T["sec"], 500) + text(x + 158, y + 24, "5", 11.5, T["ter"], anchor="end", font=M)
    yy = y + 38
    order = [1, 2, 3, 4, 5]
    for i, n in enumerate(order, 1):
        o += text(x + 14, yy + 14, str(i), 11, T["acc"] if n == sel else T["ter"], 600 if n == sel else 400, font=M)
        o += slide(n, x + 30, yy, 128, n == sel)
        if n in warn:
            o += f'<circle cx="{x + 152}" cy="{yy + 10}" r="7" fill="{T["human"]}"/>' + text(x + 152, yy + 14, "!", 10, "#1A1206", 800, "middle")
        if n in changed:
            o += rect(x + 30, yy + 54, 40, 16, T["human"], r=4) + text(x + 50, yy + 66, "Ada", 10, "#1A1206", 700, "middle")
        yy += 84
    o += rect(x + 30, yy + 2, 128, 30, "none", T["strong"], 7, 1, 'stroke-dasharray="4 3"') + text(x + 94, yy + 21, "+ New slide", 12.5, T["sec"], anchor="middle")
    return o


def toolbar(x, y, w, disabled=False, mode="text"):
    o = rect(x, y, w, 46, T["bg"]) + line(x, y + 46, x + w, y + 46, T["sub"])
    op = ' opacity="0.45"' if disabled else ""
    g = ""
    xx = x + 14
    def chip(label, ww, active=False, mono=False):
        nonlocal xx, g
        g += rect(xx, y + 9, ww, 28, T["sel"] if active else "none", T["strong"] if not active else T["acc"], 7) + text(xx + ww / 2, y + 28, label, 12.5, T["text"] if active else T["t2"], 600 if label in "BIU" else 400, "middle", M if mono else F)
        xx += ww + 6
    def sep():
        nonlocal xx, g
        g += line(xx + 2, y + 12, xx + 2, y + 34, T["strong"])
        xx += 10
    for lbl, ww in [("↶", 28), ("↷", 28)]:
        chip(lbl, ww)
    sep()
    for lbl, ww in [("T Text", 52), ("◻ Shape ▾", 74), ("▨ Image", 64), ("╱ Line", 54)]:
        chip(lbl, ww)
    sep()
    chip("▤ Layout ▾", 84)
    sep()
    if mode == "slide":
        for lbl, ww in [("Theme: Light ▾", 112), ("Background ▾", 100)]:
            chip(lbl, ww)
    else:
        for lbl, ww, act, mono in [("Sans ▾", 60, False, False), ("24", 36, False, True), ("B", 28, True, False), ("I", 28, False, False), ("U", 28, False, False), ("A▾", 34, False, False), ("≡▾", 34, False, False), ("•≡", 30, True, False)]:
            chip(lbl, ww, act, mono)
    return o + f"<g{op}>{g}</g>"


def handles(x, y, w, h):
    o = rect(x, y, w, h, "none", T["acc"], 0, 1.5)
    for hx in (x, x + w / 2, x + w):
        for hy in (y, y + h / 2, y + h):
            if hx == x + w / 2 and hy == y + h / 2:
                continue
            o += rect(hx - 4, hy - 4, 8, 8, "#FFFFFF", T["acc"], 1.5, 1.5)
    o += line(x + w / 2, y, x + w / 2, y - 18, T["acc"], 1.5) + f'<circle cx="{x + w / 2}" cy="{y - 22}" r="5" fill="#FFFFFF" stroke="{T["acc"]}" stroke-width="1.5"/>'
    return o


# ======================= 2. Editor =======================
CHAT = [("me", "Tighten slide 2 and add a slide on next quarter's plan."),
        ("ada", "I shortened the long bullet on slide 2 and added “Next quarter” after it. Review the changes when you're ready."),
        ("tool", "canvas_patch · 2 ops · v12 → v13"),
        ("me", "Make the title slide subtitle say who presents.")]


def editor():
    W, H = 1440, 900
    o = rail(H)
    top, _ = ed_top(W, "Saving… · v12")
    o += top
    o += chat_panel(56, 53, 360, H - 53, msgs=CHAT)
    o += thumbs(416, 53, H - 53)
    mx, mw = 588, W - 588
    o += toolbar(mx, 53, mw)
    cw = mw - 64
    ch = cw * 1080 / 1920
    cx, cy = mx + 32, 53 + 46 + 28
    o += rect(mx, 99, mw, H - 99, "#0A0910")
    o += slide(2, cx, cy, cw)
    k = cw / 1920
    o += handles(cx + 120 * k, cy + 280 * k, 1680 * k, 700 * k)
    # text caret inside body
    o += line(cx + 760 * k, cy + 405 * k, cx + 760 * k, cy + 455 * k, "#111418", 2)
    o += rect(cx + 120 * k + 1680 * k - 132, cy + 280 * k - 28, 132, 22, T["raised"], T["border"], 6) + text(cx + 120 * k + 1680 * k - 66, cy + 280 * k - 13, "Text · body · e_b2", 10.5, T["sec"], anchor="middle", font=M)
    # notes pane
    ny = cy + ch + 20
    o += rect(cx, ny, cw, H - ny - 16, T["surf"], T["border"], 10) + text(cx + 14, ny + 22, "Speaker notes", 12, T["sec"], 500) + text(cx + 14, ny + 46, "Lead with the 18% number; churn detail only if asked.", 13.5, T["t2"])
    # status line under notes
    o += text(cx + cw, ny + 22, "Slide 2 of 5", 11.5, T["ter"], anchor="end", font=M)
    notes = ["Layout follows Docs: Ada's chat on the left (Hide chat toggles it), then slide thumbnails, then the canvas. In the editor the app sidebar is collapsed to its icon rail.",
             "Top bar: persona presence (“Ada can edit / is editing… / suggested changes”), save state with version (Saved · Unsaved changes · Saving… · Not saved), Present (primary), Export, chat toggle.",
             "Thumbnails: click to select, drag or Alt+↑/↓ to reorder, ⌘D duplicates, Delete removes (undoable), + New slide uses the current layout. An amber “!” marks a slide whose text doesn't fit.",
             "The toolbar is contextual. Undo/redo (⌘Z / ⇧⌘Z), Insert and Layout are always there. With text selected it shows font slot, size, B/I/U (⌘B/⌘I/⌘U), colour from the theme tokens, alignment and bullets. With a shape selected: fill, stroke and width. With nothing selected: deck Theme and slide Background.",
             "Selection: frame, 8 handles and a rotate handle. Arrow keys nudge 8 units (⇧ 40). Enter edits the text in place; Esc leaves text, then deselects. Tab cycles elements in paint order. The chip shows type · role · id for debugging.",
             "Canvas: the SVG renderer from the shared layout module, so it matches Present, PDF and PPTX. It scales to fit the available width, with a minimum of 480px before the narrow layout takes over.",
             "Speaker notes under the canvas: plain text, exported to PPTX notes."]
    return svg(W, H, [o, ann_list(W + 20, 20, notes, NOTES_W - 20, "Design notes · editor")], "Slides: editor with thumbnails, canvas, formatting and persona chat")


open(f"{OUT}/slides-editor.svg", "w").write(editor())


def flag(x, y, label, kind="human"):
    bg, fg = {"human": ("rgba(245,158,91,0.14)", T["humfg"]), "done": ("rgba(76,183,130,0.14)", T["donefg"]), "blocked": ("rgba(239,100,97,0.14)", T["blkfg"]), "neutral": ("#24222F", T["sec"])}[kind]
    w = len(label) * 6.4 + 14
    return rect(x, y, w, 19, bg, None, 5) + text(x + 7, y + 13.5, label, 11.5, fg, 500), w


# ======================= 3. Suggestion review =======================
def review():
    W, H = 1440, 900
    o = rail(H)
    top, _ = ed_top(W, "Saved · v14", presence="Ada suggested changes")
    o += top + chat_panel(56, 53, 360, H - 53, msgs=CHAT[:3] + [("tool", "canvas_patch · 1 op · v13 → v14"), ("ada", "Subtitle now says “Presented by Liam Murphy”.")])
    o += thumbs(416, 53, H - 53, sel=2, warn=(), changed=(1, 2))
    mx, mw = 588, W - 588
    o += toolbar(mx, 53, mw, disabled=True)
    o += rect(mx, 99, mw, H - 99, "#0A0910")
    # sticky review bar
    o += rect(mx, 99, mw, 48, "rgba(12,11,18,0.95)") + line(mx, 147, mx + mw, 147, T["sub"])
    tt, tr = T["text"], T["ter"]
    o += f'<text x="{mx + 20}" y="128" font-family="{F}" font-size="13"><tspan font-weight="600" fill="{tt}">Ada suggested 3 changes</tspan><tspan fill="{tr}">  2 left to decide. Editing is paused until you finish.</tspan></text>'
    b, w1 = button(0, 0, "Reject all", "secondary")
    b2, w2 = button(0, 0, "Accept all", "primary")
    o += f'<g transform="translate({mx + mw - 20 - w2 - 8 - w1},108)">{b}</g><g transform="translate({mx + mw - 20 - w2},108)">{b2}</g>'
    # change cards
    tw = 250
    y = 164
    cards = [
        ("Slide 2 · What changed this quarter", "Modified", [("Body text", "Bullet 3 shortened to “Long bullets wrap the same everywhere.”")], (2, 2), None),
        ("New slide after 2 · Next quarter", "Added", [("Layout", "Title + body, 3 bullets")], (None, 6), None),
        ("Slide 1 · Quarterly review", "Modified", [("Subtitle", "“Slides acceptance deck · v1 schema” → “Presented by Liam Murphy”")], (1, 1), "accepted"),
    ]
    for title, kind, items, (bn, an), decided in cards:
        hh = 216 if not decided else 64
        o += rect(mx + 20, y, mw - 40, hh, T["surf"], T["border"] if not decided else T["sub"], 12)
        f, fw = flag(mx + 36, y + 14, kind, "human" if not decided else "neutral")
        o += f + text(mx + 44 + fw, y + 28, title, 13.5, T["text"] if not decided else T["sec"], 600)
        if decided:
            f2, _ = flag(mx + 44 + fw + len(title) * 7.3 + 10, y + 14, "✓ Accepted", "done")
            o += f2 + text(mx + 36, y + 52, items[0][1], 12.5, T["ter"]) + text(mx + mw - 36, y + 28, "Undo", 12.5, T["acc"], anchor="end")
            y += hh + 12
            continue
        # before / after
        bx = mx + 36
        o += text(bx, y + 56, "Before", 11.5, T["ter"], 500)
        if bn:
            hl = '<rect x="120" y="440" width="1680" height="190" fill="none" stroke="#EF6461" stroke-width="10" stroke-dasharray="30 18"/>' if bn == 2 else ""
            o += slide(bn, bx, y + 64, tw, extra=hl)
        else:
            o += rect(bx, y + 64, tw, tw * 9 / 16, T["sunk"], T["strong"], 4, 1, 'stroke-dasharray="4 3"') + text(bx + tw / 2, y + 64 + tw * 9 / 32 + 4, "No slide here before", 12, T["ter"], anchor="middle")
        o += text(bx + tw + 14, y + 140, "→", 18, T["ter"])
        ax = bx + tw + 40
        o += text(ax, y + 56, "After", 11.5, T["ter"], 500)
        hl = '<rect x="120" y="440" width="1680" height="80" fill="none" stroke="#4CB782" stroke-width="10"/>' if an == 2 else '<rect x="6" y="6" width="1908" height="1068" fill="none" stroke="#4CB782" stroke-width="12"/>'
        o += slide(an, ax, y + 64, tw, extra=hl)
        # change list
        lx = ax + tw + 28
        o += text(lx, y + 56, "What changed", 11.5, T["ter"], 500)
        yy = y + 80
        for lbl, desc in items:
            o += text(lx, yy, lbl, 12.5, T["t2"], 600)
            for i, ln in enumerate(wrap(desc, mw - (lx - mx) - 60, 6.4)):
                o += text(lx, yy + 20 + 18 * i, ln, 12.5, T["sec"])
            yy += 22 + 18 * len(wrap(desc, mw - (lx - mx) - 60, 6.4))
        b, w1 = button(0, 0, "Reject", "outline")
        b2, w2 = button(0, 0, "Accept", "approve")
        o += f'<g transform="translate({mx + mw - 36 - w2 - 8 - w1},{y + hh - 46})">{b}</g><g transform="translate({mx + mw - 36 - w2},{y + hh - 46})">{b2}</g>'
        y += hh + 12
    notes = ["Review opens on its own when Ada's versions are newer than the person's last edit or last review (same rule as Docs: reviewBase). Pending local edits are saved first, and the toolbar and canvas are read-only until every change is decided.",
             "One decision per changed slide (added, removed, moved, modified), plus one for a theme change. Before and after use the real slide renderer. Changed elements are outlined: red dashed for before, green for after. The text list is what screen readers get.",
             "Accept and Reject use the approval colours (a person decides). A decided card collapses to a single line with Undo until the review is finished. Keyboard: J/K next and previous change, A accept, R reject, ⇧A accept all.",
             "Finishing saves one new version noted “Kept 2 of Ada's 3 changes, rejected 1” and marks it reviewed, so Ada reads the outcome on her next turn. Rejecting a removed slide puts it back after its old neighbour.",
             "Thumbnails carry an amber “Ada” badge on changed slides. Clicking a thumbnail scrolls to its change card."]
    return svg(W, H, [o, ann_list(W + 20, 20, notes, NOTES_W - 20, "Design notes · suggestion review")], "Slides: reviewing the persona's suggested changes")


open(f"{OUT}/slides-review.svg", "w").write(review())


# ======================= 4. Presentation =======================
def present():
    W, H = 1440, 900
    o = rect(0, 0, W, H, "#000000")
    sw = W
    sh = sw * 9 / 16
    o += slide(2, 0, (H - sh) / 2, sw)
    # control bar (appears on mouse move / any key, fades after 2s)
    bx, by = W / 2 - 170, H - 64
    o += f'<g filter="url(#sh)">' + rect(bx, by, 340, 44, "rgba(27,26,39,0.92)", "#2A2A31", 12) + "</g>"
    o += rect(bx + 8, by + 7, 30, 30, "none", None, 7) + text(bx + 23, by + 27, "‹", 18, T["text"], 600, "middle")
    o += text(bx + 82, by + 27, "2 / 5", 13, T["text"], 500, "middle", M)
    o += rect(bx + 112, by + 7, 30, 30, "none", None, 7) + text(bx + 127, by + 27, "›", 18, T["text"], 600, "middle")
    o += line(bx + 152, by + 10, bx + 152, by + 34, T["strong"])
    o += text(bx + 168, by + 27, "?", 13, T["sec"], 600)
    o += rect(bx + 244, by + 7, 88, 30, "#1E1E24", "#3A3A44", 7) + text(bx + 256, by + 27, "Exit", 13, T["text"], 500) + text(bx + 320, by + 27, "esc", 10.5, T["ter"], anchor="end", font=M)
    # focus ring on Exit
    o += rect(bx + 241, by + 4, 94, 36, "none", T["acc"], 9, 2)
    # keyboard help
    kx, ky = W - 340, 40
    o += f'<g filter="url(#sh)">' + rect(kx, ky, 300, 236, T["raised"], "#2A2A31", 14) + "</g>" + text(kx + 18, ky + 30, "Keyboard", 14, T["text"], 600)
    for i, (k, d) in enumerate([("→ ↓ Space PgDn", "Next slide"), ("← ↑ PgUp", "Previous slide"), ("Home / End", "First / last"), ("3 then Enter", "Go to slide 3"), ("F", "Toggle fullscreen"), ("Esc", "Exit to the editor"), ("?", "Show or hide this")]):
        o += text(kx + 18, ky + 60 + i * 24, k, 12, T["t2"], font=M) + text(kx + 282, ky + 60 + i * 24, d, 12.5, T["sec"], anchor="end")
    notes = ["Present starts from the selected slide (⌘↵, or ⇧⌘↵ from the start). It saves pending edits, then asks for fullscreen. If the browser refuses, it fills the window and a toast says “Press F for fullscreen”.",
             "The slide is the same SVG renderer as the editor, letterboxed on black and scaled to the viewport. Hidden slides are skipped. There are no transitions; slides change instantly, which also covers prefers-reduced-motion.",
             "The control bar appears on mouse move or Tab and hides after 2s idle. It never hides while it has focus. Exit is always reachable with Esc or the visible Exit button, and focus returns to the slide you were on in the editor.",
             "Accessibility: each slide is role=img with an aria-label made from its text and image alt text. A polite live region announces “Slide 2 of 5: What changed this quarter”.",
             "Narrow or touch screens: swipe left and right, tap the right or left third to go next or previous, and the control bar stays visible. A presenter view with notes is deferred."]
    return svg(W, H, [o, ann_list(W + 20, 20, notes, NOTES_W - 20, "Design notes · presentation")], "Slides: presenting full screen with keyboard navigation and exit")


open(f"{OUT}/slides-present.svg", "w").write(present())


# ======================= 5. Export =======================
def export():
    W, H = 1440, 900
    o = rail(H)
    top, _ = ed_top(W, "Saved · v14")
    o += top + chat_panel(56, 53, 360, H - 53, msgs=CHAT[:2])
    o += thumbs(416, 53, H - 53, sel=4)
    mx, mw = 588, W - 588
    o += toolbar(mx, 53, mw, mode="slide") + rect(mx, 99, mw, H - 99, "#0A0910")
    cw = mw - 64
    cx, cy = mx + 32, 127
    o += slide(4, cx, cy, cw)
    k = cw / 1920
    # overflow marker on e_s5
    o += rect(cx + 120 * k, cy + 700 * k, 700 * k, 120 * k, "none", T["human"], 0, 2, 'stroke-dasharray="6 4"')
    o += rect(cx + 120 * k, cy + 828 * k, 168, 20, T["human"], None, 5) + text(cx + 120 * k + 8, cy + 828 * k + 14, "Text doesn't fit · 2 lines", 11.5, "#1A1206", 600)
    ex = W - 20 - 113 - 8 - 84
    o += rect(ex, 11, 84, 30, "none", T["acc"], 7, 2)
    # preflight dialog
    dx, dy = 690, 300
    o += rect(588, 99, mw, H - 99, "rgba(5,5,7,0.62)")
    o += f'<g filter="url(#sh)">' + rect(dx, dy, 560, 268, T["raised"], "#2A2A31", 14) + "</g>"
    o += text(dx + 24, dy + 40, "Export PDF with text that doesn't fit?", 16, T["text"], 600)
    for i, ln in enumerate(["One slide has text that runs past its box. The PDF shows it", "exactly as the editor does: the extra lines spill out, and any", "past the page edge are cut off."]):
        o += text(dx + 24, dy + 70 + i * 20, ln, 13, T["sec"])
    o += rect(dx + 24, dy + 136, 512, 44, T["sunk"], T["sub"], 8) + slide(4, dx + 32, dy + 141, 60) + text(dx + 104, dy + 156, "Slide 4 · text box “This box is too small…”", 12.5, T["t2"]) + text(dx + 104, dy + 173, "overflows by 2 lines", 12, T["humfg"])
    o += rect(dx, dy + 204, 560, 64, T["sunk"], None, 0) + line(dx, dy + 204, dx + 560, dy + 204, "#24222F")
    o += text(dx + 24, dy + 241, "Esc cancels", 12, T["ter"])
    b1, w1 = button(0, 0, "Go to slide 4", "outline")
    b2, w2 = button(0, 0, "Export anyway", "primary")
    o += f'<g transform="translate({dx + 560 - 24 - w2 - 8 - w1},{dy + 221})">{b1}</g><g transform="translate({dx + 560 - 24 - w2},{dy + 221})">{b2}</g>'
    o += rect(dx + 560 - 24 - w2 - 3, dy + 218, w2 + 6, 36, "none", T["acc"], 9, 2)
    # notice row under the menu: the four outcomes
    ny = H - 150
    o += rect(588, ny - 12, mw, 162, T["bg"]) + line(588, ny - 12, W, ny - 12, T["sub"]) + text(608, ny + 8, "Export outcomes (shown next to the Export button, like Docs)", 12, "#F2A97E", 600)
    rows = [("busy", "Building PDF… slide 3 of 5", T["sec"]), ("ok", "Downloaded “Quarterly review.pdf”", T["sec"]), ("ok", "Downloaded “Quarterly review.pptx”", T["sec"]),
            ("err", "PowerPoint export failed: the image on slide 3 couldn't be loaded. Try again", T["blkfg"])]
    for i, (kind, t, c) in enumerate(rows):
        yy = ny + 34 + i * 28
        o += text(608, yy, {"busy": "◌", "ok": "✓", "err": "!"}[kind], 13, c, 700) + text(628, yy, t, 12.5, c)
        if kind == "busy":
            o += text(628 + len(t) * 6.6 + 12, yy, "Cancel", 12.5, T["acc"])
        if kind == "err":
            o += text(628 + len(t) * 6.4 + 12, yy, "Retry", 12.5, T["acc"])
    notes = ["Export is a menu, like Docs. Both formats are built in the browser from the saved version. Pending edits are saved first, so an export never misses the last keystrokes. Fonts and images are loaded before building.",
             "PDF downloads a file directly: one 960×540pt page per visible slide, fonts embedded, text selectable. It never opens the print dialog.",
             "PowerPoint (.pptx): text boxes, shapes and lines stay native and editable, images are cropped pictures, and speaker notes are included. Theme colours become literal colours.",
             "Preflight: if any slide has text that doesn't fit (the amber dashed box and badge on the canvas), a dialog lists those slides first. Export anyway is focused; Go to slide selects the box.",
             "Progress, success and failure appear inline next to the Export button. Success uses role=status, failure role=alert. Cancel stops a long build. A failure never changes the deck."]
    mxm, mym = W + 20, 520
    m = text(mxm, mym - 12, "Export menu, open (step 1, real size)", 12, "#F2A97E", 600)
    m += f'<g filter="url(#sh)">' + rect(mxm, mym, 268, 176, T["raised"], "#2A2A31", 10) + "</g>"
    for i, (a, b, hi) in enumerate([("PDF (.pdf)", "Downloads one page per slide", True), ("PowerPoint (.pptx)", "Editable in PowerPoint, Keynote, Google Slides", False)]):
        yy = mym + 6 + i * 48
        if hi:
            m += rect(mxm + 4, yy, 260, 44, T["hover"], r=6)
        m += text(mxm + 14, yy + 19, a, 13, T["text"]) + text(mxm + 14, yy + 36, b, 11.5, T["ter"])
    m += line(mxm + 6, mym + 106, mxm + 262, mym + 106, T["sub"])
    m += text(mxm + 14, mym + 128, "Present", 13, T["text"]) + text(mxm + 254, mym + 128, "⌘↵", 11, T["ter"], anchor="end", font=M)
    m += text(mxm + 14, mym + 158, "Exports use the saved version (v14)", 11.5, T["ter"])
    m += text(mxm, mym + 206, "Step 2 (on screen): the preflight dialog,", 11.5, T["sec"]) + text(mxm, mym + 224, "shown only when some text doesn't fit.", 11.5, T["sec"])
    return svg(W, H, [o, ann_list(W + 20, 20, notes, NOTES_W - 20, "Design notes · export") + m], "Slides: Export menu, preflight and outcomes for PDF and PPTX")


open(f"{OUT}/slides-export.svg", "w").write(export())


# ======================= 6. States sheet =======================
def banner(x, y, w, title, body, tone="blocked", actions=()):
    bg, bd, tc = {"blocked": ("rgba(239,100,97,0.06)", "rgba(239,100,97,0.25)", T["blkfg"]), "human": ("rgba(245,158,91,0.06)", "rgba(245,158,91,0.25)", T["humfg"])}[tone]
    o = rect(x, y, w, 78, bg, bd, 12) + text(x + 16, y + 26, title, 13, tc, 600) + text(x + 16, y + 46, body, 12.5, T["sec"])
    ax = x + 16
    for lbl, kind in actions:
        b, bw = button(0, 0, lbl, kind, None, None, 26)
        o += f'<g transform="translate({ax},{y + 54 - 4})">{b}</g>'
        ax += bw + 8
    return o


def states():
    W, H = 1440, 1000
    o = rect(0, 0, W, H, T["bg"]) + text(32, 44, "Slides · states", 22, T["text"], 600) + text(32, 68, "Every state reuses the Docs patterns: Banner, Skeleton, the save-state label, role=alert and role=status.", 13, T["sec"])
    # column 1: save states + conflict + persona live + save error
    x = 32
    o += text(x, 108, "Save state (top bar)", 12, "#F2A97E", 600)
    for i, (lbl, c) in enumerate([("Saved · v14", T["ter"]), ("Unsaved changes · v14", T["ter"]), ("Saving… · v14", T["ter"]), ("Not saved · v14", T["blkfg"])]):
        o += rect(x, 120 + i * 34, 300, 28, T["surf"], T["border"], 7) + text(x + 12, 139 + i * 34, lbl, 12, c)
    o += text(x + 160, 241, "← hover: why, and Retry", 11.5, T["ter"])
    o += text(x, 288, "Conflict (409, overlapping changes)", 12, "#F2A97E", 600)
    o += banner(x, 300, 640, "This deck changed while you were editing", "Ada changed slide 2's body text, which you also edited. Your other changes were merged.", "blocked",
                [("Compare", "outline"), ("Use the latest version", "secondary"), ("Keep mine", "primary")])
    o += text(x, 408, "Persona editing (read-only)", 12, "#F2A97E", 600)
    o += rect(x, 420, 640, 40, T["surf"], T["border"], 10) + '<circle cx="%d" cy="440" r="9" fill="#2B2366" opacity="0.6"/>' % (x + 20) + text(x + 20, 444, "A", 10, "#C9BEFF", 700, "middle") + text(x + 38, 445, "Ada is editing… The canvas is read-only until she finishes. Stop", 12.5, T["sec"])
    o += text(x, 492, "Save failed (network or 5xx)", 12, "#F2A97E", 600)
    o += banner(x, 504, 640, "Couldn’t save your changes", "Vesper will keep retrying. Your edits are kept in this tab; don't close it yet.", "blocked", [("Retry now", "secondary")])
    o += text(x, 612, "Open failed / not found", 12, "#F2A97E", 600)
    o += banner(x, 624, 640, "Couldn’t open this deck", "It may have been deleted, or it belongs to someone else.", "blocked", [("Back to Slides", "secondary")])
    o += text(x, 732, "Loading the editor", 12, "#F2A97E", 600)
    o += rect(x, 744, 640, 220, T["surf"], T["border"], 12, 1, 'aria-busy="true"')
    for i in range(4):
        o += rect(x + 16, 760 + i * 48, 96, 40, "#1F1D2B", r=4)
    o += rect(x + 128, 760, 300, 14, "#1F1D2B", r=4) + rect(x + 128, 786, 496, 162, "#1F1D2B", r=6)
    # column 2: overflow, image errors, focus, narrow
    x2 = 712
    o += text(x2, 108, "Text overflow (editor)", 12, "#F2A97E", 600)
    o += rect(x2, 120, 330, 190, "#FFFFFF", T["border"], 4)
    for i, t in enumerate(["This box is too small for the", "amount of text that has been", "typed into it, so the layout", "reports an overflow."]):
        o += text(x2 + 20, 160 + i * 22, t, 15, "#1F2328" if i < 2 else "#9CA3AF", font="Arial")
    o += rect(x2 + 12, 136, 300, 52, "none", T["human"], 0, 2, 'stroke-dasharray="6 4"')
    o += rect(x2 + 12, 238, 168, 20, T["human"], None, 5) + text(x2 + 20, 252, "Text doesn't fit · 2 lines", 11.5, "#1A1206", 600) + text(x2 + 12, 276, "Lines past the box are dimmed while editing.", 11.5, "#6B7280") + text(x2 + 12, 294, "The thumbnail gets an amber “!”.", 11.5, "#6B7280")
    o += text(x2 + 350, 108, "Image upload", 12, "#F2A97E", 600)
    o += rect(x2 + 350, 120, 340, 84, T["sunk"], T["strong"], 10, 1, 'stroke-dasharray="4 3"') + text(x2 + 520, 156, "Drop an image, paste, or browse", 13, T["sec"], anchor="middle") + text(x2 + 520, 178, "PNG or JPEG, up to 10 MB", 11.5, T["ter"], anchor="middle")
    o += rect(x2 + 350, 214, 340, 34, T["surf"], T["border"], 8) + rect(x2 + 360, 228, 200, 6, T["acc"], r=3) + text(x2 + 680, 236, "Uploading… 62%", 11.5, T["ter"], anchor="end")
    o += rect(x2 + 350, 256, 340, 50, "rgba(239,100,97,0.06)", "rgba(239,100,97,0.25)", 8) + text(x2 + 362, 276, "photo.heic is 14 MB: the limit is 10 MB.", 12, T["blkfg"]) + text(x2 + 362, 294, "Missing image on a slide shows a grey “image unavailable” tile.", 11.5, T["sec"])
    o += text(x2, 342, "Keyboard and focus", 12, "#F2A97E", 600)
    o += rect(x2, 354, 690, 150, T["surf"], T["border"], 12)
    o += slide(2, x2 + 16, 370, 112, True) + rect(x2 + 13, 367, 118, 69, "none", "rgba(155,140,255,0.35)", 6, 2) + text(x2 + 16, 456, "Focused thumbnail", 11.5, T["ter"])
    o += rect(x2 + 150, 370, 180, 64, "#FFFFFF", None, 2) + rect(x2 + 162, 382, 156, 40, "none", T["acc"], 0, 1.5) + rect(x2 + 158, 378, 8, 8, "#FFFFFF", T["acc"], 1.5) + rect(x2 + 314, 418, 8, 8, "#FFFFFF", T["acc"], 1.5) + text(x2 + 150, 456, "Selected element (Tab cycles)", 11.5, T["ter"])
    o += rect(x2 + 350, 376, 30, 30, T["sel"], T["acc"], 7) + text(x2 + 365, 396, "B", 13, T["text"], 600, "middle") + rect(x2 + 347, 373, 36, 36, "none", "rgba(155,140,255,0.5)", 9, 2) + text(x2 + 392, 396, "Toolbar: roving tabindex, ← → inside groups", 11.5, T["ter"])
    o += text(x2 + 16, 482, "Shortcuts: ⌘Z ⇧⌘Z undo/redo · ⌘D duplicate · ⌫ delete · ⌘↵ present · Alt+↑/↓ move slide · Enter edit text · Esc leave/deselect · ? help", 11.5, T["sec"])
    # narrow viewport
    o += text(x2, 540, "Narrow viewport (390 × 844, editor)", 12, "#F2A97E", 600)
    px, py, pw, ph = x2, 552, 300, 420
    o += rect(px, py, pw, ph, T["bg"], T["strong"], 18)
    o += rect(px, py, pw, 44, T["bg"], None, 18) + line(px, py + 44, px + pw, py + 44, T["sub"]) + text(px + 14, py + 27, "‹  Quarterly review", 12.5, T["text"], 500) + text(px + pw - 14, py + 27, "Saved · v14   ⋯", 11, T["ter"], anchor="end")
    o += slide(2, px + 10, py + 56, pw - 20)
    sy = py + 56 + (pw - 20) * 9 / 16 + 10
    for i, n in enumerate([1, 2, 3, 4]):
        o += slide(n, px + 10 + i * 72, sy, 64, n == 2)
    o += rect(px + 10, sy + 50, pw - 20, 34, T["surf"], T["border"], 8) + text(px + 22, sy + 72, "T   ◻   ▨   ▤   B  I  U   •≡   ⋯", 13, T["t2"])
    o += rect(px + 10, py + ph - 52, pw - 20, 40, T["acc"], None, 10) + text(px + pw / 2, py + ph - 27, "✦ Chat with Ada", 13, T["onacc"], 600, "middle")
    o += text(px + pw + 16, py + 20, "Below 800px:", 12, T["t2"], 600)
    for i, t in enumerate(["• the sidebar collapses; the top bar keeps", "  title, save state and a ⋯ menu", "  (Present, Export, Rename, Duplicate)", "• thumbnails become a horizontal strip", "  under the slide (swipe; long-press to move)",
                           "• the toolbar is one scrolling row; the", "  text group opens as a bottom sheet", "• the chat is a full-height drawer from the", "  Chat with Ada button, like Docs (max-lg)", "• the review shows before/after stacked", "• present: swipe, tap thirds, Exit stays visible"]):
        o += text(px + pw + 16, py + 44 + i * 20, t, 11.5, T["sec"])
    return svg(W, H, [o, ""], "Slides: empty, loading, error, saving, conflict, overflow, keyboard focus and narrow viewport states")


open(f"{OUT}/slides-states.svg", "w").write(states())
