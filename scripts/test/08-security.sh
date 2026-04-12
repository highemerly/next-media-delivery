#!/usr/bin/env bash
# Test 08: Security — bad scheme / SSRF / DNS failure
#
# 08a: file:///etc/passwd  → スキームNG → 403, L1=DENY/BAD_REQ, max-age=86400
# 08b: ftp://example.com   → スキームNG → 403, L1=DENY/BAD_REQ, max-age=86400
# 08c: http://127.0.0.1    → SSRF (loopback) → networkError → 502, L1=MISS, ORI=TIMEOUT, max-age=120, must-revalidate
# 08d: http://192.168.10.1 → SSRF (private)  → networkError → 502, L1=MISS, ORI=TIMEOUT, max-age=120, must-revalidate
# 08e: https://dnserror.piyo.me → DNS失敗 → networkError → 502, L1=MISS, ORI=TIMEOUT, max-age=120, must-revalidate
source "$(dirname "$0")/lib.sh"

# ---- 08a: file:// スキーム ----
test_bad_scheme_file() {
  local encoded url
  encoded=$(encode_url "file:///etc/passwd")
  url=$(proxy_url "08a-file.txt" "$encoded" "emoji")

  get_response "$url"
  local nc cc
  nc=$(extract_header "nmd-cache"     "$RESP_HEADERS")
  cc=$(extract_header "cache-control" "$RESP_HEADERS")

  local ok=0
  assert_http_status "403"              "$RESP_STATUS" "HTTP status"   || ok=1
  assert_eq "L1=DENY/BAD_REQ"          "$nc"          "Nmd-Cache"     || ok=1
  assert_eq "max-age=86400"            "$cc"          "Cache-Control" || ok=1
  return $ok
}

# ---- 08b: ftp:// スキーム ----
test_bad_scheme_ftp() {
  local encoded url
  encoded=$(encode_url "ftp://example.com/file.png")
  url=$(proxy_url "08b-ftp.png" "$encoded" "emoji")

  get_response "$url"
  local nc cc
  nc=$(extract_header "nmd-cache"     "$RESP_HEADERS")
  cc=$(extract_header "cache-control" "$RESP_HEADERS")

  local ok=0
  assert_http_status "403"              "$RESP_STATUS" "HTTP status"   || ok=1
  assert_eq "L1=DENY/BAD_REQ"          "$nc"          "Nmd-Cache"     || ok=1
  assert_eq "max-age=86400"            "$cc"          "Cache-Control" || ok=1
  return $ok
}

# ---- 08c: http://127.0.0.1 (loopback SSRF) ----
test_ssrf_loopback() {
  local encoded url
  encoded=$(encode_url "http://127.0.0.1/image.png")
  url=$(proxy_url "08c-loopback.png" "$encoded" "emoji")

  get_response "$url"
  local nc cc
  nc=$(extract_header "nmd-cache"     "$RESP_HEADERS")
  cc=$(extract_header "cache-control" "$RESP_HEADERS")

  local ok=0
  assert_http_status "502"                       "$RESP_STATUS" "HTTP status"   || ok=1
  assert_eq "L1=MISS, ORI=TIMEOUT"              "$nc"          "Nmd-Cache"     || ok=1
  assert_eq "max-age=120, must-revalidate"       "$cc"          "Cache-Control" || ok=1
  return $ok
}

# ---- 08d: http://192.168.10.1 (private SSRF) ----
test_ssrf_private() {
  local encoded url
  encoded=$(encode_url "http://192.168.10.1/image.png")
  url=$(proxy_url "08d-private.png" "$encoded" "emoji")

  get_response "$url"
  local nc cc
  nc=$(extract_header "nmd-cache"     "$RESP_HEADERS")
  cc=$(extract_header "cache-control" "$RESP_HEADERS")

  local ok=0
  assert_http_status "502"                       "$RESP_STATUS" "HTTP status"   || ok=1
  assert_eq "L1=MISS, ORI=TIMEOUT"              "$nc"          "Nmd-Cache"     || ok=1
  assert_eq "max-age=120, must-revalidate"       "$cc"          "Cache-Control" || ok=1
  return $ok
}

# ---- 08e: DNS 失敗 ----
test_dns_failure() {
  local encoded url
  encoded=$(encode_url "https://dnserror.piyo.me/image.png")
  url=$(proxy_url "08e-dns.png" "$encoded" "emoji")

  get_response "$url"
  local nc cc
  nc=$(extract_header "nmd-cache"     "$RESP_HEADERS")
  cc=$(extract_header "cache-control" "$RESP_HEADERS")

  local ok=0
  assert_http_status "502"                       "$RESP_STATUS" "HTTP status"   || ok=1
  assert_eq "L1=MISS, ORI=TIMEOUT"              "$nc"          "Nmd-Cache"     || ok=1
  assert_eq "max-age=120, must-revalidate"       "$cc"          "Cache-Control" || ok=1
  return $ok
}

run_test "08a: Bad scheme file:// → 403"          test_bad_scheme_file
run_test "08b: Bad scheme ftp:// → 403"           test_bad_scheme_ftp
run_test "08c: SSRF loopback 127.0.0.1 → 502"    test_ssrf_loopback
run_test "08d: SSRF private 192.168.10.1 → 502"  test_ssrf_private
run_test "08e: DNS failure → 502"                 test_dns_failure
print_summary
