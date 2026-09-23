#!/bin/sh
set -eu

export LC_ALL=C
data_dir=${DATA_DIR:-/app/data}
fixture_dir="$data_dir/fixtures"
listen_addr=${APP_ADDR:-:8080}
base_url=${E2E_BASE_URL:-http://127.0.0.1:${listen_addr##*:}}

for tool in curl ffmpeg ffprobe awk sed grep mktemp; do
  command -v "$tool" >/dev/null 2>&1 || {
    printf 'Missing required tool: %s\n' "$tool" >&2
    exit 1
  }
done
for file in landscape.mp4 portrait.mp4 with-source-audio.mp4 rotated.mp4 narration.wav short-narration.wav invalid.mp4; do
  [ -f "$fixture_dir/$file" ] || {
    printf 'Missing fixture: %s\n' "$fixture_dir/$file" >&2
    exit 1
  }
done

mkdir -p "$data_dir/e2e"
run_dir=$(mktemp -d "$data_dir/e2e/run.XXXXXX")
printf 'E2E evidence: %s\n' "$run_dir"

json_string() {
  sed -n 's/.*"'"$2"'":"\([^"]*\)".*/\1/p' "$1"
}

json_number() {
  sed -n 's/.*"'"$2"'":\([0-9][0-9]*\).*/\1/p' "$1"
}

expect_http() {
  if [ "$1" != "$2" ]; then
    printf 'Expected HTTP %s, got %s; response: ' "$2" "$1" >&2
    cat "$3" >&2
    printf '\n' >&2
    exit 1
  fi
}

upload_video() {
  response="$run_dir/$2.upload.json"
  status=$(curl -sS -o "$response" -w '%{http_code}' \
    -F "files=@$1" "$base_url/api/assets/videos")
  expect_http "$status" 200 "$response"
  [ "$(json_string "$response" status)" = ready ] || {
    cat "$response" >&2
    exit 1
  }
  id=$(json_string "$response" id)
  [ -n "$id" ] || exit 1
  printf '%s\n' "$id"
}

upload_audio() {
  response="$run_dir/$2.upload.json"
  status=$(curl -sS -o "$response" -w '%{http_code}' \
    -F "file=@$1" "$base_url/api/assets/audio")
  expect_http "$status" 200 "$response"
  [ "$(json_string "$response" status)" = ready ] || {
    cat "$response" >&2
    exit 1
  }
  id=$(json_string "$response" id)
  [ -n "$id" ] || exit 1
  printf '%s\n' "$id"
}

create_mix() {
  request="$run_dir/$1.request.json"
  response="$run_dir/$1.mix.json"
  printf '%s\n' "$2" > "$request"
  status=$(curl -sS -o "$response" -w '%{http_code}' \
    -H 'Content-Type: application/json' --data-binary "@$request" \
    "$base_url/api/mixes")
  expect_http "$status" 200 "$response"
  [ "$(json_string "$response" status)" = completed ] || {
    cat "$response" >&2
    exit 1
  }
  id=$(json_string "$response" id)
  [ -n "$id" ] || exit 1
  printf '%s\n' "$id"
}

download_mix() {
  status=$(curl -sS -D "$run_dir/$1.headers" -o "$run_dir/$1.mp4" \
    -w '%{http_code}' "$base_url/api/mixes/$2/download")
  expect_http "$status" 200 "$run_dir/$1.mp4"
  grep -iq '^content-type: video/mp4' "$run_dir/$1.headers"
  grep -iq '^content-disposition: attachment;' "$run_dir/$1.headers"
  [ -s "$run_dir/$1.mp4" ]
}

probe_value() {
  if [ -n "$2" ]; then
    ffprobe -v error -select_streams "$2" -show_entries "$3" \
      -of default=noprint_wrappers=1:nokey=1 "$1" | awk 'NF { print; exit }'
  else
    ffprobe -v error -show_entries "$3" \
      -of default=noprint_wrappers=1:nokey=1 "$1" | awk 'NF { print; exit }'
  fi
}

duration_us() {
  awk -v seconds="$1" 'BEGIN { printf "%.0f", seconds * 1000000 }'
}

assert_delta() {
  delta=$(( $2 - $3 ))
  [ "$delta" -ge 0 ] || delta=$(( -delta ))
  printf '%s: expected=%s us actual=%s us error=%s us\n' "$1" "$3" "$2" "$delta"
  [ "$delta" -le 100000 ] || exit 1
}

sample_yavg() {
  ffmpeg -hide_banner -loglevel info -ss 0.5 -i "$1" \
    -vf "crop=100:100:490:$2,signalstats,metadata=print:key=lavfi.signalstats.YAVG" \
    -frames:v 1 -f null - >/dev/null 2>"$run_dir/$3.signal.log"
  awk -F= '/lavfi.signalstats.YAVG=/ { print $2; exit }' "$run_dir/$3.signal.log"
}

tone_level() {
  ffmpeg -hide_banner -loglevel info -i "$1" -vn \
    -af "bandpass=f=$2:width_type=h:width=80,volumedetect" \
    -f null - >/dev/null 2>"$run_dir/tone-$2.log"
  sed -n 's/.*mean_volume: \([-0-9.]*\) dB.*/\1/p' "$run_dir/tone-$2.log" | awk 'NF { value=$0 } END { print value }'
}

attempt=0
until curl -fsS "$base_url/api/health" -o "$run_dir/health.json" 2>/dev/null; do
  attempt=$((attempt + 1))
  [ "$attempt" -lt 30 ] || {
    printf 'Health did not become ready: %s\n' "$base_url" >&2
    exit 1
  }
  sleep 1
done
printf 'Health: HTTP 200\n'

landscape_id=$(upload_video "$fixture_dir/landscape.mp4" landscape)
portrait_id=$(upload_video "$fixture_dir/portrait.mp4" portrait)
sound_id=$(upload_video "$fixture_dir/with-source-audio.mp4" sound)
rotated_id=$(upload_video "$fixture_dir/rotated.mp4" rotated)
audio_id=$(upload_audio "$fixture_dir/narration.wav" narration)
short_audio_id=$(upload_audio "$fixture_dir/short-narration.wav" short-narration)
target_us=$(json_number "$run_dir/narration.upload.json" durationUs)
[ -n "$target_us" ] || exit 1
printf 'Uploaded: 4 videos, 2 audio; target=%s us\n' "$target_us"

mix_id=$(create_mix main "$(printf '{"videoIds":["%s","%s","%s","%s"],"audioId":"%s","seed":"42"}' "$landscape_id" "$portrait_id" "$sound_id" "$rotated_id" "$audio_id")")
[ "$(json_string "$run_dir/main.mix.json" seed)" = 42 ]
[ "$(json_number "$run_dir/main.mix.json" durationUs)" = "$target_us" ]
download_mix main "$mix_id"

preview_status=$(curl -sS -D "$run_dir/preview.headers" -o "$run_dir/preview.part" \
  -w '%{http_code}' -H 'Range: bytes=0-1023' "$base_url/api/mixes/$mix_id/file")
expect_http "$preview_status" 206 "$run_dir/preview.part"
grep -iq '^content-type: video/mp4' "$run_dir/preview.headers"
[ -s "$run_dir/preview.part" ]

output="$run_dir/main.mp4"
format_name=$(probe_value "$output" '' format=format_name)
case "$format_name" in *mp4*) ;; *) printf 'Unexpected format: %s\n' "$format_name" >&2; exit 1 ;; esac
video_count=$(ffprobe -v error -select_streams v -show_entries stream=index -of csv=p=0 "$output" | awk 'END { print NR }')
audio_count=$(ffprobe -v error -select_streams a -show_entries stream=index -of csv=p=0 "$output" | awk 'END { print NR }')
subtitle_count=$(ffprobe -v error -select_streams s -show_entries stream=index -of csv=p=0 "$output" | awk 'END { print NR }')
[ "$video_count" -eq 1 ] && [ "$audio_count" -eq 1 ] && [ "$subtitle_count" -eq 0 ]
[ "$(probe_value "$output" v:0 stream=codec_name)" = h264 ]
[ "$(probe_value "$output" v:0 stream=pix_fmt)" = yuv420p ]
[ "$(probe_value "$output" v:0 stream=width)" = 1080 ]
[ "$(probe_value "$output" v:0 stream=height)" = 1920 ]
[ "$(probe_value "$output" a:0 stream=codec_name)" = aac ]
fps=$(probe_value "$output" v:0 stream=avg_frame_rate)
awk -v fps="$fps" 'BEGIN { split(fps, parts, "/"); exit !(parts[2] > 0 && parts[1] / parts[2] >= 29.9 && parts[1] / parts[2] <= 30.1) }'
video_raw=$(probe_value "$output" v:0 stream=duration)
audio_raw=$(probe_value "$output" a:0 stream=duration)
format_raw=$(probe_value "$output" '' format=duration)
video_us=$(duration_us "$video_raw")
audio_us=$(duration_us "$audio_raw")
format_us=$(duration_us "$format_raw")
printf 'Main mix: id=%s format=%s video=h264/1080x1920/yuv420p/%s audio=aac\n' "$mix_id" "$format_name" "$fps"
assert_delta Video "$video_us" "$target_us"
assert_delta Audio "$audio_us" "$target_us"
assert_delta Format "$format_us" "$target_us"
assert_delta Video-Audio "$video_us" "$audio_us"

rotation=$(probe_value "$fixture_dir/rotated.mp4" v:0 stream_side_data=rotation)
awk -v rotation="$rotation" 'BEGIN { exit !(rotation == 90 || rotation == -90) }'
rotation_id=$(create_mix rotation "$(printf '{"videoIds":["%s"],"audioId":"%s","seed":"42"}' "$rotated_id" "$short_audio_id")")
download_mix rotation "$rotation_id"
[ "$(probe_value "$run_dir/rotation.mp4" v:0 stream=width)" = 1080 ]
[ "$(probe_value "$run_dir/rotation.mp4" v:0 stream=height)" = 1920 ]
top_y=$(sample_yavg "$run_dir/rotation.mp4" 60 rotation-top)
bottom_y=$(sample_yavg "$run_dir/rotation.mp4" 1760 rotation-bottom)
# The source is red on the left and blue on the right; rotation=90 puts blue above red.
awk -v top="$top_y" -v bottom="$bottom_y" 'BEGIN { exit !(top > 30 && bottom > 60 && bottom - top > 20) }'
printf 'Rotation: metadata=%s degrees top YAVG=%s bottom YAVG=%s (portrait without letterbox)\n' "$rotation" "$top_y" "$bottom_y"

sound_mix_id=$(create_mix sound "$(printf '{"videoIds":["%s"],"audioId":"%s","seed":"42"}' "$sound_id" "$short_audio_id")")
download_mix sound "$sound_mix_id"
sound_audio_count=$(ffprobe -v error -select_streams a -show_entries stream=index -of csv=p=0 "$run_dir/sound.mp4" | awk 'END { print NR }')
[ "$sound_audio_count" -eq 1 ]
[ "$(probe_value "$run_dir/sound.mp4" a:0 stream=codec_name)" = aac ]
voice_level=$(tone_level "$run_dir/sound.mp4" 440)
source_level=$(tone_level "$run_dir/sound.mp4" 1200)
awk -v voice="$voice_level" -v source="$source_level" 'BEGIN { exit !(voice > source + 10) }'
printf 'Source audio replacement: one AAC track, 440 Hz level=%s dB, 1200 Hz level=%s dB\n' "$voice_level" "$source_level"

invalid_response="$run_dir/invalid.upload.json"
invalid_status=$(curl -sS -o "$invalid_response" -w '%{http_code}' \
  -F "files=@$fixture_dir/invalid.mp4" "$base_url/api/assets/videos")
expect_http "$invalid_status" 200 "$invalid_response"
[ "$(json_string "$invalid_response" status)" = failed ]
[ "$(json_string "$invalid_response" code)" = FFPROBE_FAILED ]
printf 'Invalid media: HTTP 200, item failed / FFPROBE_FAILED\n'

shortage_request="$run_dir/shortage.request.json"
shortage_response="$run_dir/shortage.mix.json"
printf '{"videoIds":["%s"],"audioId":"%s","seed":"42"}\n' "$portrait_id" "$audio_id" > "$shortage_request"
shortage_status=$(curl -sS -o "$shortage_response" -w '%{http_code}' \
  -H 'Content-Type: application/json' --data-binary "@$shortage_request" "$base_url/api/mixes")
expect_http "$shortage_status" 422 "$shortage_response"
[ "$(json_string "$shortage_response" code)" = INSUFFICIENT_VIDEO_DURATION ]
missing_us=$(json_number "$shortage_response" missingDurationUs)
portrait_us=$(json_number "$run_dir/portrait.upload.json" durationUs)
[ -n "$missing_us" ] && [ "$missing_us" -eq "$((target_us - portrait_us))" ]
if grep -q '"status":"completed"' "$shortage_response"; then
  printf 'Shortage produced a false completed result\n' >&2
  exit 1
fi
printf 'Insufficient duration: HTTP 422, missingDurationUs=%s\n' "$missing_us"
printf 'PASS: %s\n' "$run_dir"
