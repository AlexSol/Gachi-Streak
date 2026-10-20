#!/usr/bin/env bash
# Rebuilds assets/widget/<state>/ (embedded widget backgrounds) from the
# originals in this folder: videos → WebM VP9, GIFs → animated WebP.
# Needs ffmpeg with libvpx-vp9 and libwebp; FFMPEG=/path/to/ffmpeg overrides.
set -euo pipefail
cd "$(dirname "$0")"
ff=${FFMPEG:-ffmpeg}
out=../../assets/widget
mkdir -p "$out"/{safe,pending,warning,failed}

# ≤640 px wide (the 320 px widget on a 200% display), 24 fps, no audio
vp9() {
	"$ff" -v error -y -i "$1" -vf "scale='min(640,iw)':-2,fps=24" -an \
		-c:v libvpx-vp9 -crf 38 -b:v 0 -row-mt 1 -deadline good -cpu-used 2 -pix_fmt yuv420p "$out/$2"
}
webp() {
	"$ff" -v error -y -i "$1" -c:v libwebp_anim -loop 0 -q:v 72 -compression_level 6 "$out/$2"
}

vp9  doc_2026-10-20_17-18-39.mp4 safe/01_pool_chill.webm
vp9  doc_2026-10-20_17-18-45.mp4 pending/01_smoke_break.webm
vp9  Gachi_Billy.gif.mp4         pending/02_billy_sleeps.webm
vp9  doc_2026-10-20_17-18-52.mp4 warning/01_mustache_stare.webm
webp billy-herrington.gif        failed/01_billy_red_room.webp

du -ch "$out"/*/*
