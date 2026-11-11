import sys, subprocess, numpy as np
from PIL import Image
src, svg, out = sys.argv[1], sys.argv[2], sys.argv[3]
a = Image.open(src).convert('RGBA'); alpha = np.asarray(a)[..., 3] > 0
bg = Image.new('RGBA', a.size, 'white'); bg.alpha_composite(a); ref = np.asarray(bg.convert('RGB')).astype(float)
subprocess.run(['rsvg-convert', '-b', 'white', '-w', str(a.width), '-h', str(a.height), svg, '-o', out], check=True)
o = np.asarray(Image.open(out).convert('RGB')).astype(float)
mse = ((ref - o) ** 2)[alpha].mean()
print(f"{10*np.log10(255**2/mse):.2f}")
