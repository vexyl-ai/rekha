#!/usr/bin/env python3
"""Vectorize every image in a folder with the Rekha API.

    REKHA_KEY=rk_... python3 vectorize.py photos/ out/ svg eps pdf

One request per image and format, four at a time. Files already in the
output folder are skipped, so a stopped run picks up where it left off.
"""
import mimetypes, os, sys, time, uuid, urllib.request, urllib.error
from concurrent.futures import ThreadPoolExecutor
from pathlib import Path

API = os.environ.get("REKHA_API", "https://app.rekhasvg.com/v1/trace")
KEY = os.environ["REKHA_KEY"]
OPTIONS = {"auto": "true"}          # or {"preset": "poster"}, {"preset": "bw", "threshold": "auto"}, ...

src, dst = Path(sys.argv[1]), Path(sys.argv[2])
formats = sys.argv[3:] or ["svg"]
dst.mkdir(parents=True, exist_ok=True)
images = sorted(p for p in src.iterdir() if p.suffix.lower() in (".png", ".jpg", ".jpeg", ".gif"))

def multipart(path, fields):
    b = uuid.uuid4().hex
    body = b""
    for k, v in fields.items():
        body += f"--{b}\r\nContent-Disposition: form-data; name=\"{k}\"\r\n\r\n{v}\r\n".encode()
    ctype = mimetypes.guess_type(path.name)[0] or "application/octet-stream"
    body += f"--{b}\r\nContent-Disposition: form-data; name=\"image\"; filename=\"{path.name}\"\r\nContent-Type: {ctype}\r\n\r\n".encode()
    body += path.read_bytes() + f"\r\n--{b}--\r\n".encode()
    return body, f"multipart/form-data; boundary={b}"

def trace(path, fmt):
    out = dst / f"{path.stem}.{fmt}"
    if out.exists():
        return f"skip  {out.name}"
    body, ctype = multipart(path, {**OPTIONS, "format": fmt})
    for attempt in range(5):
        req = urllib.request.Request(API, body, {"Authorization": f"Bearer {KEY}", "Content-Type": ctype})
        try:
            with urllib.request.urlopen(req, timeout=120) as r:
                out.write_bytes(r.read())
                return f"ok    {out.name}  {r.headers.get('X-Rekha-Paths')} paths in {r.headers.get('X-Rekha-Millis')} ms"
        except urllib.error.HTTPError as e:
            if e.code in (429, 503) and attempt < 4:
                time.sleep(int(e.headers.get("Retry-After", "2")))
                continue
            return f"fail  {out.name}  {e.code} {e.read().decode(errors='replace').strip()}"

with ThreadPoolExecutor(max_workers=4) as pool:
    jobs = [(p, f) for p in images for f in formats]
    for line in pool.map(lambda j: trace(*j), jobs):
        print(line)
