"""Downloads Inter 4.1 (SIL OFL 1.1) and extracts the faces the PoC uses into fonts/.
Run with: python3 -I fetch-fonts.py   (from this directory)"""
import hashlib, io, sys, urllib.request, zipfile, pathlib

URL = "https://github.com/rsms/inter/releases/download/v4.1/Inter-4.1.zip"
ZIP_SHA256 = "9883fdd4a49d4fb66bd8177ba6625ef9a64aa45899767dde3d36aa425756b11e"
FACES = ["Regular", "Medium", "SemiBold", "Bold", "Italic"]

out = pathlib.Path(__file__).resolve().parent / "fonts"
out.mkdir(exist_ok=True)
data = urllib.request.urlopen(URL).read()
if hashlib.sha256(data).hexdigest() != ZIP_SHA256:
    sys.exit("Inter-4.1.zip checksum mismatch")
z = zipfile.ZipFile(io.BytesIO(data))
for f in FACES:
    (out / f"Inter-{f}.ttf").write_bytes(z.read(f"extras/ttf/Inter-{f}.ttf"))
(out / "LICENSE.txt").write_bytes(z.read("LICENSE.txt"))
print("wrote", ", ".join(f"Inter-{f}.ttf" for f in FACES), "to", out)
