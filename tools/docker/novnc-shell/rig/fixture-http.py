#!/usr/bin/env python3
"""The rig's local web fixture for the browser pane (`rig http start`): links, target=_blank, a
form, a long page, a changing <title>, a redirect, a download, a slow endpoint, an image and a 404.

    fixture-http.py [port]      serves on 127.0.0.1:<port> (default 8088)
"""
import html
import os
import sys
import time
import urllib.parse
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

FIXTURES = os.path.expanduser("~/fixtures")


def page(title, body, head=""):
    return (f"<!doctype html><html><head><meta charset='utf-8'><title>{html.escape(title)}</title>{head}"
            "<style>body{font:15px/1.5 sans-serif;margin:2rem;max-width:46rem}a{display:block;margin:.3rem 0}</style>"
            f"</head><body>{body}</body></html>").encode()


INDEX = page("Rig fixture", """<h1>Rig fixture</h1>
<p>Every case the browser pane rows need.</p>
<a href="/long">A long page</a>
<a href="/long" target="_blank">The long page in a new tab (target=_blank)</a>
<a href="/form">A form</a>
<a href="/title">A page whose title keeps changing</a>
<a href="/redirect">A redirect back here</a>
<a href="/download">A download</a>
<a href="/slow?ms=8000">A slow page (8 s)</a>
<a href="/missing">A 404</a>
<a href="#bottom">An anchor on this page</a>
<img src="/logo.png" alt="logo" width="64" height="64">
<p id="bottom">Bottom of the index.</p>""")


class Handler(BaseHTTPRequestHandler):
    def send(self, status, body, ctype="text/html; charset=utf-8", headers=()):
        self.send_response(status)
        self.send_header("Content-Type", ctype)
        self.send_header("Content-Length", str(len(body)))
        for k, v in headers:
            self.send_header(k, v)
        self.end_headers()
        if self.command != "HEAD":
            self.wfile.write(body)

    def do_HEAD(self):
        self.do_GET()

    def do_GET(self):
        url = urllib.parse.urlparse(self.path)
        query = urllib.parse.parse_qs(url.query)
        if url.path == "/":
            return self.send(200, INDEX)
        if url.path == "/long":
            paras = "".join(f"<h2 id='s{i}'>Section {i}</h2><p>{'Long page text to scroll through. ' * 30}</p>" for i in range(1, 61))
            return self.send(200, page("A long page", "<h1>A long page</h1>" + paras + "<a href='/'>Back to the index</a>"))
        if url.path == "/form":
            return self.send(200, page("A form", """<h1>A form</h1>
<form method="post" action="/form"><label>Name <input name="name" autofocus></label>
<label>Notes <textarea name="notes"></textarea></label>
<label><input type="checkbox" name="agree"> Agree</label>
<select name="pick"><option>one</option><option>two</option></select>
<button type="submit">Send</button></form>"""))
        if url.path == "/title":
            script = "<script>let n=0;setInterval(()=>{document.title='Title tick '+(++n)},1000)</script>"
            return self.send(200, page("Title tick 0", "<h1>The tab title changes every second</h1>", script))
        if url.path == "/redirect":
            return self.send(302, b"", headers=[("Location", "/")])
        if url.path == "/download":
            body = "".join(f"row {i},value {i}\n" for i in range(100)).encode()
            return self.send(200, body, "text/csv", [("Content-Disposition", 'attachment; filename="rig-download.csv"')])
        if url.path == "/slow":
            time.sleep(min(int(query.get("ms", ["5000"])[0]), 60000) / 1000)
            return self.send(200, page("A slow page", "<h1>That took a while</h1>"))
        if url.path == "/logo.png":
            try:
                with open(os.path.join(FIXTURES, "import", "icon.png"), "rb") as f:
                    return self.send(200, f.read(), "image/png")
            except OSError:
                pass
        return self.send(404, page("Not found", f"<h1>404</h1><p>{html.escape(url.path)}</p>"))

    def do_POST(self):
        length = int(self.headers.get("Content-Length", "0"))
        fields = urllib.parse.parse_qs(self.rfile.read(length).decode())
        rows = "".join(f"<li><b>{html.escape(k)}</b>: {html.escape(', '.join(v))}</li>" for k, v in fields.items())
        self.send(200, page("Form received", f"<h1>Form received</h1><ul>{rows}</ul><a href='/form'>Again</a>"))

    def log_message(self, fmt, *args):
        sys.stderr.write("%s %s\n" % (self.log_date_time_string(), fmt % args))


if __name__ == "__main__":
    port = int(sys.argv[1]) if len(sys.argv) > 1 else 8088
    ThreadingHTTPServer(("127.0.0.1", port), Handler).serve_forever()
