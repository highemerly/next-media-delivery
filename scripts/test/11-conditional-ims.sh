#!/usr/bin/env bash
# Test 11: If-Modified-Since conditional requests
#
# Case A — L1 HIT + IMS matches (not modified) → 304, no body
#   HTTP status    : 304
#   Nmd-Cache      : L1=HIT
#   Cache-Control  : max-age=31536000, immutable
#   Last-Modified  : present
#
# Case B — L1 HIT + IMS is stale (older than Last-Modified) → 200
#   HTTP status    : 200
#   Nmd-Cache      : L1=HIT
#
# Case C — L1 MISS (no prior cache) + IMS present → 200 (IMS ignored)
#   HTTP status    : 200
#   Nmd-Cache      : L1=MISS, ..., ORI
source "$(dirname "$0")/lib.sh"

ORIGIN_URL="https://raw.githubusercontent.com/highemerly/next-media-delivery/main/assets/test-image.png"
ORIGIN_URL_MISS="https://raw.githubusercontent.com/highemerly/next-media-delivery/main/assets/test-large.png"

# Case A: L1 HIT + IMS matches → 304
test_ims_not_modified() {
  local encoded url
  encoded=$(encode_url "$ORIGIN_URL")
  url=$(proxy_url "11a-test-image.png" "$encoded" "avatar")

  # 1st request — populate L1
  get_response "$url"
  sleep 3

  # 2nd request — L1 HIT; get Last-Modified from that response
  get_response "$url"
  local lm
  lm=$(extract_header "last-modified" "$RESP_HEADERS")

  # 3rd request — send If-Modified-Since equal to Last-Modified → expect 304
  get_response_with_header "$url" "If-Modified-Since" "$lm"

  local ok=0
  local nc cc

  nc=$(extract_header "nmd-cache"     "$RESP_HEADERS")
  cc=$(extract_header "cache-control" "$RESP_HEADERS")

  assert_http_status "304"                          "$RESP_STATUS" "HTTP status 304"            || ok=1
  assert_eq          "L1=HIT"                       "$nc"          "Nmd-Cache"                  || ok=1
  assert_eq          "max-age=31536000, immutable"  "$cc"          "Cache-Control"              || ok=1

  return $ok
}

# Case B: L1 HIT + IMS is stale (epoch 0 = Thu, 01 Jan 1970) → 200
test_ims_stale() {
  local encoded url
  encoded=$(encode_url "$ORIGIN_URL")
  url=$(proxy_url "11b-test-image.png" "$encoded" "avatar")

  # 1st request — populate L1
  get_response "$url"
  sleep 3

  # 2nd request — IMS is far in the past; cache is newer → expect 200
  get_response_with_header "$url" "If-Modified-Since" "Thu, 01 Jan 1970 00:00:00 GMT"

  local ok=0
  local nc

  nc=$(extract_header "nmd-cache" "$RESP_HEADERS")

  assert_http_status "200"    "$RESP_STATUS" "HTTP status 200 (stale IMS)"  || ok=1
  assert_eq          "L1=HIT" "$nc"          "Nmd-Cache L1=HIT"             || ok=1

  return $ok
}

# Case C: L1 MISS + IMS present → 200 (IMS ignored on origin fetch)
test_ims_on_miss() {
  local encoded url
  # Use a different origin URL that has never been cached in this test run
  encoded=$(encode_url "$ORIGIN_URL_MISS")
  # Use "raw" variant so the cache key differs from any avatar/emoji cached in other tests
  url=$(proxy_url "11c-test-image-miss.png" "$encoded")

  get_response_with_header "$url" "If-Modified-Since" "Thu, 01 Jan 2099 00:00:00 GMT"

  local ok=0
  local nc

  nc=$(extract_header "nmd-cache" "$RESP_HEADERS")

  assert_http_status "200" "$RESP_STATUS" "HTTP status 200 (L1 MISS, IMS ignored)" || ok=1
  assert_match       "ORI" "$nc"          "Nmd-Cache contains ORI"                  || ok=1

  return $ok
}

run_test "11a: If-Modified-Since matches Last-Modified → 304 Not Modified"      test_ims_not_modified
run_test "11b: If-Modified-Since older than Last-Modified → 200 (stale IMS)"    test_ims_stale
run_test "11c: L1 MISS with If-Modified-Since present → 200 (IMS ignored)"      test_ims_on_miss
print_summary
