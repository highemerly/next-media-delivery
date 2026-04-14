#!/usr/bin/env bash
# Test 12: ETag and If-None-Match conditional requests
#
# Case A — Origin fetch → ETag present (W/"<unix>-<size>" format)
#   HTTP status : 200
#   ETag        : W/"<unix>-<size>"
#
# Case B — L1 HIT → ETag present and identical to first response
#   HTTP status : 200
#   ETag        : same value as Case A
#
# Case C — L1 HIT + If-None-Match matches ETag → 304
#   HTTP status : 304
#   Nmd-Cache   : L1=HIT
#   ETag        : present
#
# Case D — L1 HIT + If-None-Match does not match → 200
#   HTTP status : 200
#   Nmd-Cache   : L1=HIT
#
# Case E — L1 MISS + If-None-Match present → 200 (ignored)
#   HTTP status : 200
#   Nmd-Cache   : contains ORI
source "$(dirname "$0")/lib.sh"

ORIGIN_URL="https://raw.githubusercontent.com/highemerly/next-media-delivery/main/assets/test-image.png"
ORIGIN_URL_MISS="https://raw.githubusercontent.com/highemerly/next-media-delivery/main/assets/test-large.jpg"

# Case A: Origin fetch → ETag present
test_etag_on_origin_fetch() {
  local encoded url
  encoded=$(encode_url "$ORIGIN_URL")
  url=$(proxy_url "12a-test-image.png" "$encoded" "avatar")

  get_response "$url"

  local ok=0
  local etag
  etag=$(extract_header "etag" "$RESP_HEADERS")

  assert_http_status "200"       "$RESP_STATUS" "HTTP status 200"        || ok=1
  assert_weak_etag   "$etag"     "ETag format"                           || ok=1

  return $ok
}

# Case B: L1 HIT → ETag identical to first response
test_etag_stable_on_l1_hit() {
  local encoded url
  encoded=$(encode_url "$ORIGIN_URL")
  url=$(proxy_url "12b-test-image.png" "$encoded" "avatar")

  # 1st request — populate L1
  get_response "$url"
  local etag_first
  etag_first=$(extract_header "etag" "$RESP_HEADERS")
  sleep 3

  # 2nd request — must hit L1
  get_response "$url"
  local etag_second nc
  etag_second=$(extract_header "etag" "$RESP_HEADERS")
  nc=$(extract_header "nmd-cache" "$RESP_HEADERS")

  local ok=0
  assert_http_status "200"          "$RESP_STATUS"  "HTTP status 200"       || ok=1
  assert_eq          "L1=HIT"       "$nc"            "Nmd-Cache L1=HIT"     || ok=1
  assert_weak_etag   "$etag_second" "ETag format"                           || ok=1
  assert_eq          "$etag_first"  "$etag_second"   "ETag unchanged on L1 HIT" || ok=1

  return $ok
}

# Case C: L1 HIT + If-None-Match matches → 304
test_etag_not_modified() {
  local encoded url
  encoded=$(encode_url "$ORIGIN_URL")
  url=$(proxy_url "12c-test-image.png" "$encoded" "avatar")

  # 1st request — populate L1
  get_response "$url"
  sleep 3

  # 2nd request — get ETag from L1 HIT
  get_response "$url"
  local etag nc_body
  etag=$(extract_header "etag" "$RESP_HEADERS")

  # 3rd request — send If-None-Match equal to ETag → expect 304
  get_response_with_header "$url" "If-None-Match" "$etag"

  local ok=0
  local nc
  nc=$(extract_header "nmd-cache" "$RESP_HEADERS")

  assert_http_status "304"                         "$RESP_STATUS" "HTTP status 304"   || ok=1
  assert_eq          "L1=HIT"                      "$nc"          "Nmd-Cache L1=HIT"  || ok=1
  assert_weak_etag   "$(extract_header 'etag' "$RESP_HEADERS")"  "ETag present in 304" || ok=1

  return $ok
}

# Case D: L1 HIT + If-None-Match does not match → 200
test_etag_mismatch() {
  local encoded url
  encoded=$(encode_url "$ORIGIN_URL")
  url=$(proxy_url "12d-test-image.png" "$encoded" "avatar")

  # 1st request — populate L1
  get_response "$url"
  sleep 3

  # 2nd request — send a wrong ETag → expect 200
  get_response_with_header "$url" "If-None-Match" 'W/"0-0"'

  local ok=0
  local nc
  nc=$(extract_header "nmd-cache" "$RESP_HEADERS")

  assert_http_status "200"    "$RESP_STATUS" "HTTP status 200 (ETag mismatch)" || ok=1
  assert_eq          "L1=HIT" "$nc"          "Nmd-Cache L1=HIT"                || ok=1

  return $ok
}

# Case E: L1 MISS + If-None-Match present → 200 (ignored)
test_etag_on_miss() {
  local encoded url
  # Use a different origin URL that has never been cached in this test run
  encoded=$(encode_url "$ORIGIN_URL_MISS")
  # Use "raw" variant so the cache key differs from any avatar/emoji cached in other tests
  url=$(proxy_url "12e-test-image-miss.png" "$encoded")

  get_response_with_header "$url" "If-None-Match" 'W/"9999999999-999999"'

  local ok=0
  local nc etag
  nc=$(extract_header "nmd-cache" "$RESP_HEADERS")
  etag=$(extract_header "etag" "$RESP_HEADERS")

  assert_http_status "200"  "$RESP_STATUS" "HTTP status 200 (L1 MISS, INM ignored)" || ok=1
  assert_match       "ORI"  "$nc"          "Nmd-Cache contains ORI"                  || ok=1
  assert_weak_etag   "$etag" "ETag present even on MISS"                             || ok=1

  return $ok
}

# Case F: L1 HIT + If-None-Match does NOT match + If-Modified-Since matches
#         → 200 (INM takes precedence over IMS per RFC 9110 §13.1.3)
test_etag_inm_precedence_over_ims() {
  local encoded url
  encoded=$(encode_url "$ORIGIN_URL")
  url=$(proxy_url "12f-test-image.png" "$encoded" "avatar")

  # 1st request — populate L1
  get_response "$url"
  sleep 3

  # 2nd request — get Last-Modified from L1 HIT
  get_response "$url"
  local lm
  lm=$(extract_header "last-modified" "$RESP_HEADERS")

  # 3rd request — INM mismatch + IMS matches
  # RFC 9110 §13.1.3: INM present → IMS must be ignored → 200
  get_response_with_two_headers "$url" \
    "If-None-Match"    'W/"0-0"' \
    "If-Modified-Since" "$lm"

  local ok=0
  assert_http_status "200" "$RESP_STATUS" "HTTP status 200 (INM mismatch overrides IMS match)" || ok=1

  return $ok
}

run_test "12a: Origin fetch → ETag present (weak format)"                    test_etag_on_origin_fetch
run_test "12b: L1 HIT → ETag identical to first response"                    test_etag_stable_on_l1_hit
run_test "12c: If-None-Match matches ETag → 304 Not Modified"                test_etag_not_modified
run_test "12d: If-None-Match does not match ETag → 200"                      test_etag_mismatch
run_test "12e: L1 MISS with If-None-Match present → 200 (INM ignored)"      test_etag_on_miss
run_test "12f: INM mismatch + IMS match → 200 (INM takes precedence, RFC 9110 §13.1.3)" test_etag_inm_precedence_over_ims
print_summary
