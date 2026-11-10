"""Render a 5s loop of comets orbiting a glowing core over a twinkling starfield."""
import json, math, os, subprocess, sys
import numpy as np
from PIL import Image, ImageDraw, ImageFilter

W, H, FPS, SECS = 1280, 720, 30, 5
N = FPS * SECS
OUT = sys.argv[1]
MODE = sys.argv[2] if len(sys.argv) > 2 else "full"

rng = np.random.default_rng(7)
stars = [(rng.uniform(0, W), rng.uniform(0, H), rng.uniform(0.5, 2.0), rng.uniform(0, 2 * math.pi)) for _ in range(260)]
comets = [  # radius x, radius y, phase, colour, tilt
    (420, 150, 0.0, (120, 200, 255), 0.25),
    (300, 220, 2.1, (255, 140, 200), -0.4),
    (520, 110, 4.2, (255, 210, 120), 0.1),
]
CX, CY = W / 2, H / 2

def pos(rx, ry, ph, tilt, t):
    a = ph + 2 * math.pi * t  # one full orbit per loop
    x, y = rx * math.cos(a), ry * math.sin(a)
    return CX + x * math.cos(tilt) - y * math.sin(tilt), CY + x * math.sin(tilt) + y * math.cos(tilt)

yy, xx = np.mgrid[0:H, 0:W]
bg = np.zeros((H, W, 3), np.float32)
d = np.hypot(xx - CX, yy - CY) / W
bg[..., 0], bg[..., 1], bg[..., 2] = 8 + 20 * np.exp(-d * 4), 6 + 10 * np.exp(-d * 4), 22 + 40 * np.exp(-d * 3)

def frame(i):
    t = i / N
    img = Image.fromarray(bg.astype(np.uint8))
    dr = ImageDraw.Draw(img)
    for x, y, s, p in stars:
        b = int(120 + 135 * (0.5 + 0.5 * math.sin(p + 2 * math.pi * 2 * t)))
        dr.ellipse([x - s, y - s, x + s, y + s], fill=(b, b, b))
    glow = Image.new("RGB", (W, H))
    gd = ImageDraw.Draw(glow)
    pulse = 1 + 0.08 * math.sin(2 * math.pi * 2 * t)
    for r, c in [(90, (60, 40, 120)), (55, (160, 120, 255)), (28, (255, 240, 255))]:
        r *= pulse
        gd.ellipse([CX - r, CY - r, CX + r, CY + r], fill=c)
    for rx, ry, ph, col, tilt in comets:
        for k in range(40):  # tail: fading points trailing behind
            x, y = pos(rx, ry, ph, tilt, t - k * 0.004)
            f = (1 - k / 40) ** 2
            s = 9 * f + 1
            gd.ellipse([x - s, y - s, x + s, y + s], fill=tuple(int(c * f) for c in col))
    glow = glow.filter(ImageFilter.GaussianBlur(6))
    img = Image.fromarray(np.clip(np.asarray(img, np.int16) + np.asarray(glow, np.int16) * 1.6, 0, 255).astype(np.uint8))
    dr = ImageDraw.Draw(img)
    for rx, ry, ph, col, tilt in comets:
        x, y = pos(rx, ry, ph, tilt, t)
        dr.ellipse([x - 4, y - 4, x + 4, y + 4], fill=(255, 255, 255))
    return img

if MODE == "stills":
    os.makedirs(f"{OUT}/stills", exist_ok=True)
    for i in (0, N // 2, N - 1):
        frame(i).save(f"{OUT}/stills/{i:04d}.png")
    sys.exit(0)

ff = subprocess.Popen(["ffmpeg", "-y", "-loglevel", "error", "-f", "rawvideo", "-pix_fmt", "rgb24", "-s", f"{W}x{H}",
                       "-r", str(FPS), "-i", "-", "-c:v", "libx264", "-pix_fmt", "yuv420p", "-crf", "18",
                       "-movflags", "+faststart", f"{OUT}/video.mp4"], stdin=subprocess.PIPE)
for i in range(N):
    im = frame(i)
    ff.stdin.write(im.tobytes())
    if i == 0:
        im.convert("RGB").save(f"{OUT}/poster.jpg", quality=90)
    if i % 15 == 0:
        im.save(f"{OUT}/preview.jpg", quality=80)
        json.dump({"frame": i + 1, "total": N}, open(f"{OUT}/progress.json", "w"))
ff.stdin.close()
if ff.wait() != 0:
    sys.exit("ffmpeg failed")
json.dump({"frame": N, "total": N}, open(f"{OUT}/progress.json", "w"))
