#!/usr/bin/env python3
"""Vectorize PNG/JPG files with the Rekha API: vectorize.py IN_DIR OUT_DIR [FORMAT ...]"""
import json, os, sys, time, uuid
from concurrent.futures import ThreadPoolExecutor
from pathlib import Path
from urllib import error, request

API = os.environ.get("REKHA_API", "https://app.rekhasvg.com/v1/trace")
KEY = os.environ.get("REKHA_KEY") or sys.exit("REKHA_KEY is not set")
MIME = {".png": "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg"}


def multipart(fields, name, data, mime):
    b = uuid.uuid4().hex
    body = "".join(f"--{b}\r\nContent-Disposition: form-data; name=\"{k}\"\r\n\r\n{v}\r\n" for k, v in fields.items())
    body += f"--{b}\r\nContent-Disposition: form-data; name=\"image\"; filename=\"{name}\"\r\nContent-Type: {mime}\r\n\r\n"
    return body.encode() + data + f"\r\n--{b}--\r\n".encode(), f"multipart/form-data; boundary={b}"


def trace(src, fmt):
    body, ctype = multipart({"auto": "true", "format": fmt}, src.name, src.read_bytes(), MIME[src.suffix.lower()])
    headers = {"Authorization": f"Bearer {KEY}", "Content-Type": ctype}
    for attempt in range(6):
        try:
            with request.urlopen(request.Request(API, data=body, headers=headers), timeout=120) as r:
                return r.read(), f"{r.headers.get('X-Rekha-Paths', '?')} paths, {r.headers.get('X-Rekha-Millis', '?')} ms"
        except error.HTTPError as e:
            if e.code not in (429, 503) or attempt == 5:
                try:
                    msg = json.loads(e.read()).get("error", "")
                except Exception:
                    msg = ""
                return None, f"HTTP {e.code} {msg}".strip()
            time.sleep(float(e.headers.get("Retry-After") or 1))
        except (error.URLError, TimeoutError, OSError) as e:
            return None, f"error {getattr(e, 'reason', e)}"


def convert(src, dst, fmt):
    out = dst / f"{src.stem}.{fmt}"
    if out.exists():
        return f"skip   {src.name} -> {out.name} (exists)"
    data, note = trace(src, fmt)
    if data is None:
        return f"FAIL   {src.name} -> {out.name}: {note}"
    out.write_bytes(data)
    return f"ok     {src.name} -> {out.name} ({len(data)} bytes, {note})"


if __name__ == "__main__":
    if len(sys.argv) < 3:
        sys.exit(__doc__)
    src_dir, dst = Path(sys.argv[1]), Path(sys.argv[2])
    dst.mkdir(parents=True, exist_ok=True)
    files = sorted(p for p in src_dir.iterdir() if p.suffix.lower() in MIME)
    jobs = [(f, dst, fmt) for f in files for fmt in (sys.argv[3:] or ["svg"])]
    with ThreadPoolExecutor(max_workers=4) as pool:
        for line in pool.map(lambda j: convert(*j), jobs):
            print(line, flush=True)
