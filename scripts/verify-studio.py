from pathlib import Path
from html.parser import HTMLParser
import re
root=Path(__file__).resolve().parent.parent
html=(root/"dist/studio.html").read_text(encoding="utf-8-sig")
js="\n".join((root/p).read_text(encoding="utf-8-sig") for p in ("dist/studio.js","dist/studio-controls.js","dist/studio-vst.js","dist/studio-patch.js","dist/studio-loading.js","dist/studio-sampler.js","dist/studio-trim.js","dist/trim-timeline.js"))
ids=re.findall(r'id="([^"]+)"',html)
assert len(ids)==len(set(ids)), "Duplicate UI identifiers"
refs=set(re.findall(r"\$\('([^']+)'\)",js))
assert not refs-set(ids), "Missing controls: "+str(refs-set(ids))
assert 'id="apply"' not in html
native=set(re.findall(r'\b(native[A-Z]\w+)\s*\(',js))
bindings=set(re.findall(r'"(native[^"]+)"\s*:',(root/"main_windows.go").read_text(encoding="utf-8-sig")))
assert not native-bindings, "Missing native bridge methods: "+str(native-bindings)
class Structure(HTMLParser):
    def __init__(self):super().__init__();self.stack=[];self.signals=[]
    def handle_starttag(self,tag,attrs):
        attrs=dict(attrs)
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
print("Studio checks passed: controls, native bridge, shared signals and automatic-routing UI.")
