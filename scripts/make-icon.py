from pathlib import Path
from PIL import Image, ImageDraw

root = Path(__file__).resolve().parent.parent
assets = root / "assets"
assets.mkdir(exist_ok=True)
scale = 4
im = Image.new("RGBA", (256 * scale, 256 * scale))
d = ImageDraw.Draw(im)
def box(coords): return tuple(int(v * scale) for v in coords)
d.rounded_rectangle(box((5, 5, 251, 251)), radius=54*scale, fill="#14251d", outline="#3b5845", width=3*scale)
d.arc(box((45, 39, 211, 211)), 180, 360, fill="#b9e78d", width=15*scale)
d.rounded_rectangle(box((37, 119, 66, 196)), radius=12*scale, fill="#b9e78d")
d.rounded_rectangle(box((190, 119, 219, 196)), radius=12*scale, fill="#b9e78d")
for x, top, bottom in [(88, 119, 163), (112, 96, 184), (136, 110, 174), (160, 126, 159)]:
    d.rounded_rectangle(box((x, top, x+10, bottom)), radius=5*scale, fill="#b9e78d")
im = im.resize((256, 256), Image.Resampling.LANCZOS)
im.save(assets / "quiet-lab.png")
im.save(assets / "quiet-lab.ico", sizes=[(16,16),(24,24),(32,32),(48,48),(64,64),(128,128),(256,256)])
for name in ("quiet-lab.png", "quiet-lab.ico"):
    (root / "dist" / name).write_bytes((assets / name).read_bytes())
