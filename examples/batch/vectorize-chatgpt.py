import os, sys, time, json, uuid, urllib.request, urllib.error
from pathlib import Path
from concurrent.futures import ThreadPoolExecutor

API = os.getenv("REKHA_API", "https://app.rekhasvg.com/v1/trace")
KEY = os.environ["REKHA_KEY"]
PRESETS = {"auto": "true"}

def convert(src, dst, fmt):
    if dst.exists():
        return
    while True:
        boundary = uuid.uuid4().hex
        data = (f"--{boundary}\r\nContent-Disposition: form-data; name=\"auto\"\r\n\r\ntrue\r\n"
                f"--{boundary}\r\nContent-Disposition: form-data; name=\"format\"\r\n\r\n{fmt}\r\n"
                f"--{boundary}\r\nContent-Disposition: form-data; name=\"image\"; filename=\"{src.name}\"\r\n"
                f"Content-Type: application/octet-stream\r\n\r\n").encode()
        data += src.read_bytes() + f"\r\n--{boundary}--\r\n".encode()
        req = urllib.request.Request(API, data=data, headers={
            "Authorization": f"Bearer {KEY}",
            "Content-Type": f"multipart/form-data; boundary={boundary}"})
        try:
            with urllib.request.urlopen(req) as r:
                paths, millis = r.headers.get("X-Rekha-Paths", "?"), r.headers.get("X-Rekha-Millis", "?")
                dst.parent.mkdir(parents=True, exist_ok=True)
                dst.write_bytes(r.read())
                print(f"{src} -> {dst}: {paths} paths, {millis} ms", flush=True)
                return
        except urllib.error.HTTPError as e:
            if e.code in (429, 503):
                time.sleep(max(0, float(e.headers.get("Retry-After", 1))))
                continue
            try:
                msg = json.loads(e.read()).get("error", str(e))
            except Exception:
                msg = str(e)
            print(f"{src} ({fmt}): ERROR {msg}", flush=True)
            return

def main():
    if len(sys.argv) < 3:
        sys.exit("Usage: vectorize.py INPUT_DIR OUTPUT_DIR [svg pdf eps dxf ...]")
    inp, out = Path(sys.argv[1]), Path(sys.argv[2])
    formats = sys.argv[3:] or ["svg"]
    files = [p for p in inp.iterdir() if p.is_file()
             and p.suffix.lower() in (".png", ".jpg", ".jpeg")]
    with ThreadPoolExecutor(max_workers=4) as pool:
        list(pool.map(lambda job: convert(*job), [
            (p, out / f"{p.stem}.{fmt}", fmt)
            for p in files for fmt in formats]))

if __name__ == "__main__":
    main()
