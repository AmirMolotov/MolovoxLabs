from pathlib import Path
from html.parser import HTMLParser
import re
from urllib.parse import urlsplit, unquote
root=Path(__file__).resolve().parent.parent
html=(root/"web/studio.html").read_text(encoding="utf-8-sig")
js="\n".join((root/"web"/src).read_text(encoding="utf-8-sig")
             for src in re.findall(r'<script\b[^>]*src="([^"]+)"',html))
ids=re.findall(r'id="([^"]+)"',html)
assert len(ids)==len(set(ids)), "Duplicate UI identifiers"
refs=set(re.findall(r"\$\('([^']+)'\)",js))
assert not refs-set(ids), "Missing controls: "+str(refs-set(ids))
assert 'id="apply"' not in html
native=set(re.findall(r'\b(native[A-Z]\w+)\s*\(',js))
bindings=set(re.findall(r'"(native[^"]+)"\s*:',(root/"cmd/molovoxlabs/main_windows.go").read_text(encoding="utf-8-sig")))
assert not native-bindings, "Missing native bridge methods: "+str(native-bindings)
class Structure(HTMLParser):
    def __init__(self):super().__init__();self.stack=[];self.signals=[]
    def handle_starttag(self,tag,attrs):
        attrs=dict(attrs)
        if tag=="video" and attrs.get("id") in ("arrPreview","arrMediaVideo"):
            assert "muted" in attrs, "Arrangement preview must use native audio"
        if tag=="canvas" and attrs.get("id") in ("micWave","outWave"):
            assert "section" not in self.stack
            self.signals.append(attrs.get("id"))
        if tag not in ("input","meta","link","br","hr","img"):self.stack.append(tag)
    def handle_endtag(self,tag):
        assert self.stack and self.stack[-1]==tag,(tag,self.stack)
        self.stack.pop()
p=Structure();p.feed(html)
assert not p.stack
assert set(p.signals)=={"micWave","outWave"}
# Check every page and stylesheet, including secondary browser entry points.
web=root/"web"
def check_resource(source, reference):
    url=urlsplit(reference)
    if url.scheme or url.netloc or not url.path:
        return
    target=(source.parent/unquote(url.path)).resolve()
    if url.path.endswith("/"):
        target=target/"index.html"
    assert target.is_relative_to(web.resolve()) and target.is_file(), f"Missing resource in {source.name}: {reference}"

class Resources(HTMLParser):
    def handle_starttag(self,tag,attrs):
        for name,value in attrs:
            if name in ("src","href") and value:
                check_resource(self.source,value)

for page in web.glob("*.html"):
    resources=Resources();resources.source=page
    resources.feed(page.read_text(encoding="utf-8-sig"))
for sheet in (web/"css").glob("*.css"):
    for reference in re.findall(r"url\(\s*([^)]*?)\s*\)",sheet.read_text(encoding="utf-8-sig")):
        check_resource(sheet,reference.strip("\"'"))
print("Studio checks passed: controls, native bridge, shared signals and automatic-routing UI.")
