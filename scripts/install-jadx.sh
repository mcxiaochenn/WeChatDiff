#!/usr/bin/env bash
set -euo pipefail
mkdir -p .work/tools
curl --fail --location --retry 3 --max-time 300 \
  https://github.com/skylot/jadx/releases/download/v1.5.6/jadx-1.5.6.zip \
  -o .work/tools/jadx.zip
echo '545ea2be9c242511bc145755cf4bda2485ade42966e096f8b4d3da2a230e8974  .work/tools/jadx.zip' | sha256sum --check
unzip -q .work/tools/jadx.zip -d .work/tools/jadx
chmod +x .work/tools/jadx/bin/jadx
