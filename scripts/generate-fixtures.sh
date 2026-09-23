#!/bin/sh
set -eu

export LC_ALL=C
fixture_dir="${DATA_DIR:-/app/data}/fixtures"
mkdir -p "$fixture_dir"

for tool in ffmpeg ffprobe; do
  command -v "$tool" >/dev/null 2>&1 || {
    printf 'Missing required tool: %s\n' "$tool" >&2
    exit 1
  }
done

ffmpeg -hide_banner -loglevel error -y \
  -f lavfi -i 'testsrc2=size=640x360:rate=30:duration=4.2' \
  -an -c:v libx264 -pix_fmt yuv420p -movflags +faststart \
  "$fixture_dir/landscape.mp4"

ffmpeg -hide_banner -loglevel error -y \
  -f lavfi -i 'color=c=yellow:s=360x640:r=30:d=3.6' \
  -an -c:v libx264 -pix_fmt yuv420p -movflags +faststart \
  "$fixture_dir/portrait.mp4"

ffmpeg -hide_banner -loglevel error -y \
  -f lavfi -i 'testsrc2=size=640x360:rate=30:duration=4.5' \
  -f lavfi -i 'sine=frequency=1200:sample_rate=48000:duration=4.5' \
  -map 0:v:0 -map 1:a:0 -c:v libx264 -pix_fmt yuv420p -c:a aac \
  -movflags +faststart "$fixture_dir/with-source-audio.mp4"

ffmpeg -hide_banner -loglevel error -y \
  -f lavfi -i 'color=c=red:s=320x360:r=30:d=3.7' \
  -f lavfi -i 'color=c=blue:s=320x360:r=30:d=3.7' \
  -filter_complex '[0:v][1:v]hstack=inputs=2[v]' -map '[v]' \
  -an -c:v libx264 -pix_fmt yuv420p \
  "$fixture_dir/rotation-base.mp4"
ffmpeg -hide_banner -loglevel error -y \
  -display_rotation:v:0 90 -i "$fixture_dir/rotation-base.mp4" \
  -map 0:v:0 -c:v copy -movflags +faststart \
  "$fixture_dir/rotated.mp4"
rm "$fixture_dir/rotation-base.mp4"

ffmpeg -hide_banner -loglevel error -y \
  -f lavfi -i 'sine=frequency=440:sample_rate=48000:duration=9.7' \
  -vn -c:a pcm_s16le "$fixture_dir/narration.wav"
ffmpeg -hide_banner -loglevel error -y \
  -f lavfi -i 'sine=frequency=440:sample_rate=48000:duration=2.3' \
  -vn -c:a pcm_s16le "$fixture_dir/short-narration.wav"
printf 'invalid video fixture\n' > "$fixture_dir/invalid.mp4"

for file in landscape.mp4 portrait.mp4 with-source-audio.mp4 rotated.mp4 narration.wav short-narration.wav; do
  ffprobe -v error "$fixture_dir/$file" >/dev/null || exit 1
done
printf 'Fixtures ready: %s\n' "$fixture_dir"
