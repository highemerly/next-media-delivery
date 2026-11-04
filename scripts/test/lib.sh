#!/usr/bin/env bash
# lib.sh — shared helpers for integration tests
# Source this file from each test script: source "$(dirname "$0")/lib.sh"

set -euo pipefail

PROXY_BASE="${PROXY_BASE:-http://localhost:3000}"
# Origin serving ./assets with normal Content-Types (the `origin` service in
# docker-compose.yml). Tests never fetch from the internet: an external origin
# can rate-limit CI (raw.githubusercontent.com returned 429) and only sees
# assets already merged to main.
ORIGIN_BASE="${ORIGIN_BASE:-http://origin}"
PASS=0
FAIL=0

# encode_url <raw_url>
encode_url() {
  python3 -c "import urllib.parse, sys; print(urllib.parse.quote(sys.argv[1], safe=''))" "$1"
}

# proxy_url <filename> <encoded_url> [flag ...]
# e.g. proxy_url "img.png" "$enc" "avatar"
#      proxy_url "img.png" "$enc" "avatar" "fallback"
proxy_url() {
  local filename="$1" encoded="$2"
  shift 2
  local qs="${PROXY_BASE}/proxy/${filename}?url=${encoded}"
  for flag in "$@"; do
    qs="${qs}&${flag}"
  done
  echo "$qs"
}

# get_response <url>
# Fetches headers and status code in a single request (HEAD).
# Stores results in: RESP_STATUS, RESP_HEADERS
get_response() {
  local url="$1" tmpfile
  tmpfile=$(mktemp)
  RESP_STATUS=$(curl -sI -o "$tmpfile" -w "%{http_code}" "$url")
  RESP_HEADERS=$(tr -d '\r' < "$tmpfile")
  rm -f "$tmpfile"
}

# get_http_status <url>  (single request; use get_response when headers are also needed)
get_http_status() {
  curl -o /dev/null -s -w "%{http_code}" "$1"
}

# get_all_headers <url>  (full header block, \r stripped)
get_all_headers() {
  curl -sI "$1" | tr -d '\r'
}

# extract_header <header_name> <header_block>  (case-insensitive)
extract_header() {
  local name="$1" block="$2"
  echo "$block" | grep -i "^${name}:" | head -1 | cut -d' ' -f2-
}

# assert_http_status <expected> <actual> <label>
assert_http_status() {
  local expected="$1" actual="$2" label="$3"
  if [ "$actual" = "$expected" ]; then
    echo "  [OK] ${label}: HTTP ${actual}"
    return 0
  else
    echo "  [FAIL] ${label}: expected HTTP ${expected}, got ${actual}"
    return 1
  fi
}

# assert_eq <expected> <actual> <label>
assert_eq() {
  local expected="$1" actual="$2" label="$3"
  if [ "$actual" = "$expected" ]; then
    echo "  [OK] ${label}: '${actual}'"
    return 0
  else
    echo "  [FAIL] ${label}: expected '${expected}', got '${actual}'"
    return 1
  fi
}

# assert_header_absent <header_name> <header_block> <label>
# Checks that a header does NOT appear in the response.
assert_header_absent() {
  local name="$1" block="$2" label="$3"
  if echo "$block" | grep -qi "^${name}:"; then
    local value
    value=$(echo "$block" | grep -i "^${name}:" | head -1 | cut -d' ' -f2-)
    echo "  [FAIL] ${label}: header '${name}' must be absent, got '${value}'"
    return 1
  else
    echo "  [OK] ${label}: header '${name}' is absent"
    return 0
  fi
}

# assert_match <pattern> <actual> <label>  (grep -qE extended regex)
assert_match() {
  local pattern="$1" actual="$2" label="$3"
  if echo "$actual" | grep -qE "$pattern"; then
    echo "  [OK] ${label}: '${actual}' matches '${pattern}'"
    return 0
  else
    echo "  [FAIL] ${label}: '${actual}' does not match '${pattern}'"
    return 1
  fi
}

# server_timing_fetch_dur <server_timing_value>
# Prints the nmdFetch duration (milliseconds, may be fractional: "0.423").
server_timing_fetch_dur() {
  echo "$1" | sed -nE 's/.*nmdFetch;dur=([0-9]+(\.[0-9]+)?).*/\1/p'
}

# assert_server_timing_fetch_positive <server_timing_value> <label>
# Checks nmdFetch;dur > 0 (origin was actually contacted; sub-millisecond
# fetches are reported as fractions, so only "not fetched" is exactly 0)
assert_server_timing_fetch_positive() {
  local st="$1" label="$2"
  local fetch_dur
  fetch_dur=$(server_timing_fetch_dur "$st")
  if [ -n "$fetch_dur" ] && awk -v d="$fetch_dur" 'BEGIN { exit !(d > 0) }'; then
    echo "  [OK] ${label}: nmdFetch;dur=${fetch_dur} (> 0)"
    return 0
  else
    echo "  [FAIL] ${label}: expected nmdFetch;dur > 0, got '${st}'"
    return 1
  fi
}

# assert_server_timing_fetch_zero <server_timing_value> <label>
# Checks nmdFetch;dur = 0 exactly (cache hit, no origin contact)
assert_server_timing_fetch_zero() {
  local st="$1" label="$2"
  local fetch_dur
  fetch_dur=$(server_timing_fetch_dur "$st")
  if [ "$fetch_dur" = "0" ]; then
    echo "  [OK] ${label}: nmdFetch;dur=0 (cache hit)"
    return 0
  else
    echo "  [FAIL] ${label}: expected nmdFetch;dur=0, got '${st}'"
    return 1
  fi
}

# assert_last_modified_not_after <before_epoch> <http_date_value> <label>
# Checks Last-Modified <= before_epoch (reflects cache-write time, not current time)
assert_last_modified_not_after() {
  local before_epoch="$1" lm_value="$2" label="$3"
  if [ -z "$lm_value" ]; then
    echo "  [FAIL] ${label}: Last-Modified header is absent"
    return 1
  fi
  local lm_epoch
  lm_epoch=$(python3 -c "
from email.utils import parsedate_to_datetime
import sys
print(int(parsedate_to_datetime(sys.argv[1]).timestamp()))
" "$lm_value" 2>/dev/null || echo 0)

  if [ "$lm_epoch" -le "$before_epoch" ]; then
    echo "  [OK] ${label}: Last-Modified(${lm_value}) <= cache-write time"
    return 0
  else
    echo "  [FAIL] ${label}: Last-Modified(epoch=${lm_epoch}) is AFTER cache-write time(${before_epoch})"
    return 1
  fi
}

# assert_weak_etag <etag_value> <label>
# Checks that the value has the weak ETag format: W/"<unix>-<size>"
assert_weak_etag() {
  local value="$1" label="$2"
  if echo "$value" | grep -qE '^W/"[0-9]+-[0-9]+"$'; then
    echo "  [OK] ${label}: weak ETag '${value}'"
    return 0
  else
    echo "  [FAIL] ${label}: expected weak ETag W/\"<unix>-<size>\", got '${value}'"
    return 1
  fi
}

# get_response_with_header <url> <header_name> <header_value>
# Like get_response but sends an additional request header.
# Stores results in: RESP_STATUS, RESP_HEADERS
get_response_with_header() {
  local url="$1" header_name="$2" header_value="$3" tmpfile
  tmpfile=$(mktemp)
  RESP_STATUS=$(curl -sI -H "${header_name}: ${header_value}" -o "$tmpfile" -w "%{http_code}" "$url")
  RESP_HEADERS=$(tr -d '\r' < "$tmpfile")
  rm -f "$tmpfile"
}

# get_response_with_two_headers <url> <name1> <value1> <name2> <value2>
# Like get_response but sends two additional request headers.
# Stores results in: RESP_STATUS, RESP_HEADERS
get_response_with_two_headers() {
  local url="$1" name1="$2" value1="$3" name2="$4" value2="$5" tmpfile
  tmpfile=$(mktemp)
  RESP_STATUS=$(curl -sI -H "${name1}: ${value1}" -H "${name2}: ${value2}" -o "$tmpfile" -w "%{http_code}" "$url")
  RESP_HEADERS=$(tr -d '\r' < "$tmpfile")
  rm -f "$tmpfile"
}

# get_image_body <url>
# Downloads the response body to a temp file.
# Sets RESP_BODY_FILE (caller must rm after use).
get_image_body() {
  RESP_BODY_FILE=$(mktemp).bin
  curl -sf -o "$RESP_BODY_FILE" "$1"
}

# get_image_dimensions <file>
# Returns "WxH" via python3 + struct (PNG) or identifies via file magic.
# Works for WebP (RIFF) and PNG without external tools.
get_image_dimensions() {
  local file="$1"
  python3 - "$file" <<'EOF'
import sys, struct

path = sys.argv[1]
with open(path, 'rb') as f:
    hdr = f.read(32)

# PNG: 8-byte sig + 4-byte len + "IHDR" + 4W + 4H
if hdr[:8] == b'\x89PNG\r\n\x1a\n':
    w, h = struct.unpack('>II', hdr[16:24])
    print(f"{w}x{h}")
# WebP: RIFF????WEBPVP8 or VP8L or VP8X
elif hdr[:4] == b'RIFF' and hdr[8:12] == b'WEBP':
    chunk = hdr[12:16]
    if chunk == b'VP8 ':
        # Lossy: 3-byte frame tag + 3-byte start code (0x9d012a), then
        # 14 bits W, 14 bits H (each in a little-endian uint16)
        with open(path, 'rb') as f:
            f.seek(20)
            data = f.read(10)
        w = (struct.unpack('<H', data[6:8])[0] & 0x3fff)
        h = (struct.unpack('<H', data[8:10])[0] & 0x3fff)
        print(f"{w}x{h}")
    elif chunk == b'VP8L':
        with open(path, 'rb') as f:
            f.seek(21)
            data = f.read(4)
        bits = struct.unpack('<I', data)[0]
        w = (bits & 0x3fff) + 1
        h = ((bits >> 14) & 0x3fff) + 1
        print(f"{w}x{h}")
    elif chunk == b'VP8X':
        with open(path, 'rb') as f:
            f.seek(24)
            data = f.read(6)
        w = struct.unpack('<I', data[:3] + b'\x00')[0] + 1
        h = struct.unpack('<I', data[3:] + b'\x00')[0] + 1
        print(f"{w}x{h}")
    else:
        print("unknown_webp")
# GIF: 6-byte signature + logical screen descriptor (W, H as LE uint16)
elif hdr[:6] in (b'GIF87a', b'GIF89a'):
    w, h = struct.unpack('<HH', hdr[6:10])
    print(f"{w}x{h}")
# JPEG
elif hdr[:2] == b'\xff\xd8':
    with open(path, 'rb') as f:
        f.seek(2)
        while True:
            marker = f.read(2)
            if len(marker) < 2: break
            length = struct.unpack('>H', f.read(2))[0]
            if marker[1] in (0xC0, 0xC2):
                f.read(1)  # precision
                h, w = struct.unpack('>HH', f.read(4))
                print(f"{w}x{h}")
                break
            f.seek(length - 2, 1)
else:
    print("unknown")
EOF
}

# assert_image_dimensions <expected_WxH> <file> <label>
assert_image_dimensions() {
  local expected="$1" file="$2" label="$3"
  local actual
  actual=$(get_image_dimensions "$file")
  if [ "$actual" = "$expected" ]; then
    echo "  [OK] ${label}: dimensions ${actual}"
    return 0
  else
    echo "  [FAIL] ${label}: expected ${expected}, got ${actual}"
    return 1
  fi
}

# assert_image_dimensions_le <max_W> <max_H> <file> <label>
# Checks width <= max_W AND height <= max_H (for fit-within variants)
assert_image_dimensions_le() {
  local max_w="$1" max_h="$2" file="$3" label="$4"
  local dims actual_w actual_h
  dims=$(get_image_dimensions "$file")
  actual_w=$(echo "$dims" | cut -dx -f1)
  actual_h=$(echo "$dims" | cut -dx -f2)
  if [ "$actual_w" -le "$max_w" ] && [ "$actual_h" -le "$max_h" ]; then
    echo "  [OK] ${label}: ${dims} fits within ${max_w}x${max_h}"
    return 0
  else
    echo "  [FAIL] ${label}: ${dims} exceeds ${max_w}x${max_h}"
    return 1
  fi
}

# assert_aspect_ratio_preserved <orig_W> <orig_H> <file> <label>
# Checks that W/H ratio is preserved (within 1px rounding tolerance)
assert_aspect_ratio_preserved() {
  local orig_w="$1" orig_h="$2" file="$3" label="$4"
  local dims actual_w actual_h
  dims=$(get_image_dimensions "$file")
  actual_w=$(echo "$dims" | cut -dx -f1)
  actual_h=$(echo "$dims" | cut -dx -f2)
  python3 - "$orig_w" "$orig_h" "$actual_w" "$actual_h" "$label" <<'EOF'
import sys
ow, oh, aw, ah = int(sys.argv[1]), int(sys.argv[2]), int(sys.argv[3]), int(sys.argv[4])
label = sys.argv[5]
# expected height if width scaled proportionally
expected_h = round(oh * aw / ow)
if abs(ah - expected_h) <= 1:
    print(f"  [OK] {label}: {aw}x{ah} preserves aspect ratio of {ow}x{oh}")
    sys.exit(0)
else:
    print(f"  [FAIL] {label}: {aw}x{ah} does not preserve aspect ratio of {ow}x{oh} (expected height ~{expected_h})")
    sys.exit(1)
EOF
}

# is_animated_image <file>
# Prints "yes" when the file is an APNG / animated GIF / animated WebP.
# Container-level check only (no decoding): APNG → acTL chunk, GIF → 2+ frames,
# WebP → ANIM chunk.
is_animated_image() {
  python3 - "$1" <<'EOF'
import sys

with open(sys.argv[1], 'rb') as f:
    data = f.read()

animated = False
if data[:8] == b'\x89PNG\r\n\x1a\n':
    head = data.split(b'IDAT', 1)[0]
    animated = b'acTL' in head
elif data[:6] in (b'GIF87a', b'GIF89a'):
    # Graphics Control Extension appears once per frame; the NETSCAPE
    # application extension is only written for looping animations.
    animated = data.count(b'\x21\xf9\x04') >= 2 or b'NETSCAPE2.0' in data
elif data[:4] == b'RIFF' and data[8:12] == b'WEBP':
    animated = b'ANIM' in data

print("yes" if animated else "no")
EOF
}

# assert_animated <file> <label>
assert_animated() {
  local file="$1" label="$2" actual
  actual=$(is_animated_image "$file")
  if [ "$actual" = "yes" ]; then
    echo "  [OK] ${label}: animation preserved"
    return 0
  else
    echo "  [FAIL] ${label}: expected an animated image, got a still image"
    return 1
  fi
}

# assert_not_animated <file> <label>
assert_not_animated() {
  local file="$1" label="$2" actual
  actual=$(is_animated_image "$file")
  if [ "$actual" = "no" ]; then
    echo "  [OK] ${label}: still image"
    return 0
  else
    echo "  [FAIL] ${label}: expected a still image, got an animated image"
    return 1
  fi
}

# run_test <name> <function>
run_test() {
  local name="$1" fn="$2"
  echo ""
  echo "==> ${name}"
  if $fn; then
    PASS=$((PASS + 1))
  else
    FAIL=$((FAIL + 1))
  fi
}

# print_summary — call at end of each test script
print_summary() {
  echo ""
  echo "========================================"
  echo "Results: ${PASS} passed, ${FAIL} failed"
  echo "========================================"
  [ "$FAIL" -eq 0 ]
}
