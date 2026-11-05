from http.server import ThreadingHTTPServer, SimpleHTTPRequestHandler
from pathlib import Path

root = Path(__file__).resolve().parents[2]
output = root / "release" / "sampler-smoke"
output.mkdir(parents=True, exist_ok=True)
class Handler(SimpleHTTPRequestHandler):
    def __init__(self, *args, **kwargs): super().__init__(*args, directory=str(root), **kwargs)
    def do_POST(self):
        length = int(self.headers.get("Content-Length", "0"))
        if length > 10 * 1024 * 1024 or self.path not in ("/smoke-video", "/smoke-result"):
            self.send_error(400); return
        data = self.rfile.read(length)
        name = "media.webm" if self.path == "/smoke-video" else "result.json"
        (output / name).write_bytes(data)
        self.send_response(204); self.end_headers()
    def log_message(self, *args): pass
print("Sampler browser smoke test: http://127.0.0.1:8097/tests/browser/sampler-smoke.html", flush=True)
ThreadingHTTPServer(("127.0.0.1", 8097), Handler).serve_forever()
