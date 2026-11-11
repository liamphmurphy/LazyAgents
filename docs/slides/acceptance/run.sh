#!/usr/bin/env bash
# Renders a deck (default: the acceptance deck) with the app's own code in the
# editor's SVG, the PDF and the PPTX, opens the PPTX in LibreOffice (in a
# container) and checks all of them (check.py). Writes evidence/ beside this
# script, or into the directory given second. Needs node (web/ installed, with
# Playwright's Chromium) and docker.
#   docs/slides/acceptance/run.sh [deck.json] [evidence-dir]
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
deck=$(realpath "${1:-$here/../fixtures/acceptance-deck.json}")
dest=$(mkdir -p "${2:-$here/evidence}" && cd "${2:-$here/evidence}" && pwd)
work=$(mktemp -d)
(cd "$here/../../../web" && node scripts/slides-deck-evidence.mjs "$deck" "$work")
rm -f "$work/page.html"
cp "$deck" "$work/deck.json"
cp "$here/check.py" "$work/"
image=vesper-slides-libreoffice
docker image inspect "$image" >/dev/null 2>&1 || docker build -q -t "$image" - <<'DOCKERFILE'
FROM debian:bookworm-slim
RUN apt-get update -qq && DEBIAN_FRONTEND=noninteractive apt-get install -y -qq --no-install-recommends libreoffice-impress poppler-utils python3-pil fonts-liberation2 >/dev/null && rm -rf /var/lib/apt/lists/*
DOCKERFILE
c=$(docker create "$image" python3 /work/check.py /work)
trap 'docker rm -f "$c" >/dev/null' EXIT
docker cp "$work/." "$c:/work"
status=0
docker start -a "$c" || status=$?
docker cp "$c:/work/evidence/." "$dest/"
docker cp "$c:/work/lo/deck.pdf" "$dest/deck-libreoffice.pdf" 2>/dev/null || true
cp "$work/deck.pdf" "$work/deck.pptx" "$dest/"
exit $status
