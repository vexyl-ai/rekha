#!/usr/bin/env python3
"""Build the site gallery: trace every PNG or JPEG in a source directory
with the release settings, score each result, and write the page.

    python3 bench/gallery.py tests web/gallery

Needs bin/rekha (make build), rsvg-convert, Pillow and numpy. Sources
over 2048 pixels on the long side are scaled down first, as the
playground does. Writes <name>.png (source at 640 px), <name>.svg (the
trace), gallery.json (the figures) and index.html.
"""
import concurrent.futures as cf
import html
import json
import os
import re
import subprocess
import sys
import tempfile
import time

import numpy as np
from PIL import Image

SRC, OUT = sys.argv[1], sys.argv[2]
REKHA = os.path.join("bin", "rekha")
FLAGS = ["--preset", "photo", "--gradients"]
MAX_SIDE = 2048
THUMB = 640
NAMES = {"8_1sasa11": "Gradient bird"}


def title_of(stem):
    if stem in NAMES:
        return NAMES[stem]
    words = stem.replace("_", "-").split("-")
    words = [w for w in words if w and w != "logo"]
    out = []
    for i, w in enumerate(words):
        if w == "letter" and i + 1 < len(words) and len(words[i + 1]) == 1:
            out.append("letter")
            words[i + 1] = words[i + 1].upper()
        else:
            out.append(w)
    s = " ".join(out)
    return s[:1].upper() + s[1:]


def psnr(src_png, svg, w, h):
    with tempfile.NamedTemporaryFile(suffix=".png") as render:
        subprocess.run(["rsvg-convert", "-w", str(w), "-h", str(h), "-b", "white", svg, "-o", render.name], check=True)
        a = Image.open(src_png).convert("RGBA")
        alpha = np.asarray(a)[..., 3] > 0
        bg = Image.new("RGBA", a.size, "white")
        bg.alpha_composite(a)
        ref = np.asarray(bg.convert("RGB")).astype(float)
        o = np.asarray(Image.open(render.name).convert("RGB")).astype(float)
        mse = ((ref - o) ** 2)[alpha].mean()
        return 99.0 if mse == 0 else 10 * np.log10(255 ** 2 / mse)


def one(path):
    stem = os.path.splitext(os.path.basename(path))[0]
    im = Image.open(path).convert("RGBA")
    if max(im.size) > MAX_SIDE:
        scale = MAX_SIDE / max(im.size)
        im = im.resize((round(im.width * scale), round(im.height * scale)), Image.LANCZOS)
    with tempfile.TemporaryDirectory() as tmp:
        src = os.path.join(tmp, stem + ".png")
        im.save(src)
        svg = os.path.join(OUT, stem + ".svg")
        t0 = time.time()
        subprocess.run([REKHA, *FLAGS, "-o", svg, src], check=True)
        secs = time.time() - t0
        body = open(svg).read()
        score = psnr(src, svg, im.width, im.height)
    thumb = im.copy()
    thumb.thumbnail((THUMB, THUMB), Image.LANCZOS)
    thumb.save(os.path.join(OUT, stem + ".png"), optimize=True)
    return {
        "name": stem,
        "title": title_of(stem),
        "width": im.width,
        "height": im.height,
        "paths": body.count("<path"),
        "gradients": body.count("Gradient id="),
        "bytes": len(body.encode()),
        "psnr": round(score, 1),
        "seconds": round(secs, 1),
    }


def page(items):
    figs = []
    for it in items:
        cap = f"{it['paths']} paths, {it['bytes'] / 1000:.0f} kB"
        if it["gradients"] == 1:
            cap += ", 1 gradient"
        elif it["gradients"]:
            cap += f", {it['gradients']} gradients"
        cap += f", {it['psnr']} dB"
        figs.append(f'''      <figure class="wipe">
        <div class="frame" style="aspect-ratio: {it['width']} / {it['height']}">
          <img class="px" src="{it['name']}.png" width="{it['width']}" height="{it['height']}" alt="{html.escape(it['title'])}: the source image" loading="lazy">
          <div class="vec"><img src="{it['name']}.svg" width="{it['width']}" height="{it['height']}" alt="{html.escape(it['title'])}: traced by Rekha" loading="lazy"></div>
          <input type="range" min="0" max="100" value="50" aria-label="Move the divider between source and SVG for {html.escape(it['title'])}">
          <div class="handle" aria-hidden="true"></div>
          <span class="tag l">Source</span><span class="tag r">SVG</span>
        </div>
        <figcaption><strong>{html.escape(it['title'])}</strong> <a href="{it['name']}.svg">SVG</a><br>{cap}</figcaption>
      </figure>''')
    n = len(items)
    return f'''<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Rekha gallery: logos traced from PNG to SVG</title>
<meta name="description" content="{n} logos traced by Rekha with the photo preset and gradients on, each with a source-versus-SVG wipe, the path count, file size and PSNR. Download any SVG.">
<link rel="canonical" href="https://vexyl-ai.github.io/rekha/gallery/">
<meta property="og:type" content="website">
<meta property="og:site_name" content="Rekha">
<meta property="og:title" content="Rekha gallery: logos traced from PNG to SVG">
<meta property="og:description" content="{n} logos traced to SVG with gradient fills. Wipe between source and result, download the SVG.">
<meta property="og:url" content="https://vexyl-ai.github.io/rekha/gallery/">
<meta property="og:image" content="https://vexyl-ai.github.io/rekha/img/og.png">
<meta property="og:image:width" content="1200">
<meta property="og:image:height" content="630">
<meta name="twitter:card" content="summary_large_image">
<meta name="twitter:title" content="Rekha gallery: logos traced from PNG to SVG">
<meta name="twitter:description" content="{n} logos traced to SVG with gradient fills. Wipe between source and result, download the SVG.">
<meta name="twitter:image" content="https://vexyl-ai.github.io/rekha/img/og.png">
<meta name="theme-color" content="#014AED">
<link rel="icon" href="../img/logo-mark.svg" type="image/svg+xml">
<link rel="preconnect" href="https://fonts.googleapis.com">
<link rel="stylesheet" href="https://fonts.googleapis.com/css2?family=IBM+Plex+Sans:wght@400;500;600&family=IBM+Plex+Mono&display=swap">
<link rel="stylesheet" href="../site.css">
</head>
<body>
<header class="bar">
  <div class="wrap">
    <a class="brand" href="../"><img src="../img/logo-mark.svg" alt="" width="34" height="34">Rekha</a>
    <nav aria-label="Site">
      <a href="../demo/">Playground</a>
      <a href="./">Gallery</a>
      <a href="../#licence">Licence</a>
      <a href="https://github.com/vexyl-ai/rekha">GitHub</a>
    </nav>
  </div>
</header>

<main>
<section class="gallery" id="gallery">
  <div class="wrap">
    <h1>Gallery</h1>
    <p class="prose">{n} logos traced with <code>rekha --preset photo --gradients</code> and no other settings, at 2048 pixels. Drag each divider to wipe between the source and the SVG. PSNR is measured over the logo's own pixels after rendering the SVG back at source size; higher is closer. Every SVG links to the file Rekha wrote, so you can open it in your editor.</p>
    <p class="prose">Want to try your own? <a href="../demo/">Open the playground</a>; nothing is uploaded.</p>
    <div class="grid">
{chr(10).join(figs)}
    </div>
  </div>
</section>
</main>

<footer>
  <div class="wrap">
    <span>Rekha is written by <a href="https://www.linkedin.com/in/anilmathewm/">Anil Mathew</a>.</span>
    <span class="spacer"></span>
    <a href="https://github.com/vexyl-ai/rekha">GitHub</a>
    <a href="https://github.com/vexyl-ai/rekha/blob/main/LICENSE">AGPL-3.0</a>
    <a href="https://github.com/vexyl-ai/rekha/blob/main/COMMERCIAL.md">Commercial licence</a>
    <a href="../demo/">Playground</a>
  </div>
</footer>
<script src="../site.js"></script>
</body>
</html>
'''


def main():
    os.makedirs(OUT, exist_ok=True)
    srcs = sorted(p for p in os.listdir(SRC) if re.search(r"\.(png|jpe?g)$", p, re.I))
    with cf.ThreadPoolExecutor(max_workers=4) as ex:
        items = list(ex.map(one, [os.path.join(SRC, p) for p in srcs]))
    items.sort(key=lambda it: it["title"])
    with open(os.path.join(OUT, "gallery.json"), "w") as f:
        json.dump(items, f, indent=1)
    with open(os.path.join(OUT, "index.html"), "w") as f:
        f.write(page(items))
    for it in items:
        print(f"{it['title']:48} {it['paths']:4} paths {it['bytes']/1000:6.0f} kB {it['gradients']:3} grad {it['psnr']:5.1f} dB {it['seconds']:5.1f} s")


if __name__ == "__main__":
    main()
