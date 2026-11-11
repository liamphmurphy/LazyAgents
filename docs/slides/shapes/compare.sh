#!/usr/bin/env bash
# Renders the acceptance deck's preset-shapes slide with the app's export code,
# opens the PPTX in LibreOffice (in a container) and compares it with the app's
# PDF shape by shape. Writes docs/slides/shapes/evidence/. Needs docker.
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
work=$(mktemp -d)
(cd "$here/../../../web" && node scripts/slides-shapes-evidence.mjs "$work")
cp "$here/compare.py" "$work/"
image=vesper-slides-libreoffice
docker image inspect "$image" >/dev/null 2>&1 || docker build -q -t "$image" - <<'DOCKERFILE'
FROM debian:bookworm-slim
RUN apt-get update -qq && DEBIAN_FRONTEND=noninteractive apt-get install -y -qq --no-install-recommends libreoffice-impress poppler-utils python3-pil fonts-liberation2 >/dev/null && rm -rf /var/lib/apt/lists/*
DOCKERFILE
c=$(docker create "$image" python3 /work/compare.py /work)
trap 'docker rm -f "$c" >/dev/null' EXIT
docker cp "$work/." "$c:/work"
status=0
docker start -a "$c" || status=$?
mkdir -p "$here/evidence"
for f in report.json shapes-app-vs-libreoffice.png shapes-diff.png shapes-bare-app-vs-libreoffice.png shapes-bare-diff.png; do
  docker cp "$c:/work/$f" "$here/evidence/$f"
done
cp "$work/shapes.pptx" "$work/shapes.pdf" "$work/shapes.svg" "$here/evidence/"
exit $status
