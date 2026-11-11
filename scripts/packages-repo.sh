#!/usr/bin/env bash
# Rebuild the apt and yum repositories served from vexyl-ai/packages.
#
#   scripts/packages-repo.sh <dir with .deb and .rpm> <checkout of the packages repo> <gpg key id>
#
# Copies the packages in, keeps the newest KEEP versions of each, signs
# the rpms, regenerates the apt (dists/stable, pool) and rpm (repodata)
# metadata, signs both, and writes the public key and an index page.
# Needs apt-ftparchive, createrepo_c, rpmsign and gpg with the key loaded.
set -euo pipefail

pkgs=$1
repo=$2
key=$3
keep=${KEEP:-3}
base=${BASE_URL:-https://vexyl-ai.github.io/packages}

mkdir -p "$repo/apt/pool/main/r/rekha" "$repo/rpm"
cp "$pkgs"/*.deb "$repo/apt/pool/main/r/rekha/"
cp "$pkgs"/*.rpm "$repo/rpm/"

# Keep the newest $keep versions of each package name and architecture.
python3 - "$repo" "$keep" <<'PY'
import os, re, sys
from collections import defaultdict
repo, keep = sys.argv[1], int(sys.argv[2])
pat = re.compile(r"^(?P<name>[a-z0-9-]+)[_-](?P<ver>\d+(?:\.\d+)*(?:[.~-][0-9A-Za-z.~-]+)?)[_.-](?P<rest>.+)\.(?P<ext>deb|rpm)$")
for d in (os.path.join(repo, "apt/pool/main/r/rekha"), os.path.join(repo, "rpm")):
    groups = defaultdict(list)
    for f in os.listdir(d):
        m = pat.match(f)
        if m:
            groups[(m["name"], m["rest"], m["ext"])].append((m["ver"], f))
    for files in groups.values():
        files.sort(key=lambda t: [int(x) if x.isdigit() else x for x in re.split(r"[.~-]", t[0])], reverse=True)
        for _, f in files[keep:]:
            os.remove(os.path.join(d, f))
            print("pruned", f)
PY

# rpm: each package carries a signature, and the repository metadata is signed.
rpmsign --define "_gpg_name $key" --addsign "$repo"/rpm/*.rpm
createrepo_c --update "$repo/rpm" > /dev/null
rm -f "$repo/rpm/repodata/repomd.xml.asc"
gpg --batch --yes -u "$key" --detach-sign --armor "$repo/rpm/repodata/repomd.xml"
cat > "$repo/rpm/vexyl-ai.repo" <<REPO
[vexyl-ai]
name=Rekha packages
baseurl=$base/rpm
enabled=1
gpgcheck=1
repo_gpgcheck=1
gpgkey=$base/gpg.key
REPO

# apt: one suite, stable; one component, main; signed Release.
(
  cd "$repo/apt"
  for arch in amd64 arm64; do
    mkdir -p "dists/stable/main/binary-$arch"
    apt-ftparchive --arch "$arch" packages pool > "dists/stable/main/binary-$arch/Packages"
    gzip -9 -f -k "dists/stable/main/binary-$arch/Packages"
  done
  apt-ftparchive \
    -o APT::FTPArchive::Release::Origin=Rekha \
    -o APT::FTPArchive::Release::Label=Rekha \
    -o APT::FTPArchive::Release::Suite=stable \
    -o APT::FTPArchive::Release::Codename=stable \
    -o APT::FTPArchive::Release::Architectures="amd64 arm64" \
    -o APT::FTPArchive::Release::Components=main \
    -o APT::FTPArchive::Release::Description="Rekha, raster to SVG tracer" \
    release dists/stable > dists/stable/Release
  gpg --batch --yes -u "$key" --clearsign -o dists/stable/InRelease dists/stable/Release
  gpg --batch --yes -u "$key" -abs -o dists/stable/Release.gpg dists/stable/Release
)

gpg --armor --export "$key" > "$repo/gpg.key"
touch "$repo/.nojekyll"
cat > "$repo/index.html" <<HTML
<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Rekha package repositories</title>
<meta name="description" content="apt and dnf repositories for Rekha, the raster to SVG tracer. Add once, install and upgrade with the system package manager.">
<link rel="canonical" href="$base/">
<style>body{font:16px/1.5 system-ui,sans-serif;max-width:72ch;margin:40px auto;padding:0 20px;color:#1C2230}pre{background:#F2F3F5;padding:12px 16px;overflow-x:auto;border-radius:6px}a{color:#014AED}</style>
</head>
<body>
<h1>Rekha package repositories</h1>
<p>Signed apt and dnf repositories for <a href="https://vexyl-ai.github.io/rekha/">Rekha</a>, the raster to SVG tracer. Each holds the <code>rekha</code> command and the <code>rekha-serve</code> HTTP service for amd64 and arm64. Key fingerprint: <code>$key</code>.</p>
<h2>Debian, Ubuntu</h2>
<pre>sudo install -m 0755 -d /etc/apt/keyrings
curl -fsSL $base/gpg.key | sudo tee /etc/apt/keyrings/vexyl-ai.asc &gt; /dev/null
echo "deb [signed-by=/etc/apt/keyrings/vexyl-ai.asc] $base/apt stable main" | sudo tee /etc/apt/sources.list.d/vexyl-ai.list
sudo apt update &amp;&amp; sudo apt install rekha</pre>
<h2>Fedora, RHEL, openSUSE</h2>
<pre>sudo curl -fsSL $base/rpm/vexyl-ai.repo -o /etc/yum.repos.d/vexyl-ai.repo
sudo dnf install rekha</pre>
<p>Alpine and Arch packages, and plain archives, are on the <a href="https://github.com/vexyl-ai/rekha/releases/latest">releases page</a>. Rebuilt on every release by <a href="https://github.com/vexyl-ai/rekha/blob/main/.github/workflows/packages.yml">packages.yml</a>.</p>
</body>
</html>
HTML
echo "repository rebuilt: $(ls "$repo"/apt/pool/main/r/rekha | wc -l) deb, $(ls "$repo"/rpm/*.rpm | wc -l) rpm"
