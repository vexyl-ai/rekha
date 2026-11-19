import os, sys, uuid, urllib.request
api = os.environ.get("REKHA_API", "https://app.rekhasvg.com/v1/trace")
key = os.environ.get("REKHA_KEY", "")
src, fmt = sys.argv[1], (sys.argv[2] if len(sys.argv) > 2 else "svg")
b = uuid.uuid4().hex
body = (f"--{b}\r\nContent-Disposition: form-data; name=\"auto\"\r\n\r\ntrue\r\n"
        f"--{b}\r\nContent-Disposition: form-data; name=\"format\"\r\n\r\n{fmt}\r\n"
        f"--{b}\r\nContent-Disposition: form-data; name=\"image\"; filename=\"{src}\"\r\n"
        f"Content-Type: application/octet-stream\r\n\r\n").encode() + open(src, "rb").read() + f"\r\n--{b}--\r\n".encode()
req = urllib.request.Request(api, data=body, headers={"Authorization": f"Bearer {key}", "Content-Type": f"multipart/form-data; boundary={b}"})
with urllib.request.urlopen(req) as r:
    out = src.rsplit(".", 1)[0] + "." + fmt
    open(out, "wb").write(r.read())
    print(out, r.headers["X-Rekha-Paths"], "paths", r.headers["X-Rekha-Millis"], "ms")
