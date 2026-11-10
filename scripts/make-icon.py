from pathlib import Path
from PIL import Image

root = Path(__file__).resolve().parent.parent
assets = root / "assets"
with Image.open(assets / "molovox-labs-source.png") as source:
    icon = source.convert("RGBA")
    icon.thumbnail((256, 256), Image.Resampling.LANCZOS)
    icon.save(root / "web" / "images" / "molovox-labs.png")
    icon.save(assets / "molovox-labs.ico",
              sizes=[(16, 16), (24, 24), (32, 32), (48, 48),
                     (64, 64), (128, 128), (256, 256)])
(root / "web" / "images" / "molovox-labs.ico").write_bytes(
    (assets / "molovox-labs.ico").read_bytes())
