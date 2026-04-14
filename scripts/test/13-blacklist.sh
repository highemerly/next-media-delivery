#!/usr/bin/env bash
# Test 13: Blacklist — domain and IP/CIDR blocking
#
# 13a: blocked-test-1.invalid (ORIGIN_BLACKLIST_DOMAINS 1件目) → 403, L1=DENY/BAD_DOMAIN, max-age=86400
# 13b: Blocked-Test-2.INVALID (大文字混在、2件目) → 403, L1=DENY/BAD_DOMAIN, max-age=86400
# 13d: 203.0.113.1 (ORIGIN_BLACKLIST_IPS 個別IP) → 403, L1=DENY/BAD_DOMAIN, max-age=86400
# 13e: 198.51.100.5 (ORIGIN_BLACKLIST_IPS CIDR 198.51.100.0/24 内) → 403, L1=DENY/BAD_DOMAIN, max-age=86400
source "$(dirname "$0")/lib.sh"

# ---- 13a: ブラックリスト登録ドメイン (1件目) ----
test_blocked_domain_first() {
  local encoded url
  encoded=$(encode_url "http://blocked-test-1.invalid/image.png")
  url=$(proxy_url "13a-blocked-domain-1.png" "$encoded" "emoji")

  get_response "$url"
  local nc cc
  nc=$(extract_header "nmd-cache"     "$RESP_HEADERS")
  cc=$(extract_header "cache-control" "$RESP_HEADERS")

  local ok=0
  assert_http_status "403"           "$RESP_STATUS" "HTTP status"   || ok=1
  assert_eq "L1=DENY/BAD_DOMAIN"    "$nc"          "Nmd-Cache"     || ok=1
  assert_eq "max-age=86400"         "$cc"          "Cache-Control" || ok=1
  return $ok
}

# ---- 13b: ブラックリスト登録ドメイン (大文字混在・2件目) ----
test_blocked_domain_case_insensitive() {
  local encoded url
  encoded=$(encode_url "http://Blocked-Test-2.INVALID/image.png")
  url=$(proxy_url "13b-blocked-domain-case.png" "$encoded" "emoji")

  get_response "$url"
  local nc cc
  nc=$(extract_header "nmd-cache"     "$RESP_HEADERS")
  cc=$(extract_header "cache-control" "$RESP_HEADERS")

  local ok=0
  assert_http_status "403"           "$RESP_STATUS" "HTTP status"   || ok=1
  assert_eq "L1=DENY/BAD_DOMAIN"    "$nc"          "Nmd-Cache"     || ok=1
  assert_eq "max-age=86400"         "$cc"          "Cache-Control" || ok=1
  return $ok
}

# ---- 13d: ブラックリスト登録IP (個別IP 203.0.113.1) ----
test_blocked_ip_exact() {
  local encoded url
  encoded=$(encode_url "http://203.0.113.1/image.png")
  url=$(proxy_url "13d-blocked-ip-exact.png" "$encoded" "emoji")

  get_response "$url"
  local nc cc
  nc=$(extract_header "nmd-cache"     "$RESP_HEADERS")
  cc=$(extract_header "cache-control" "$RESP_HEADERS")

  local ok=0
  assert_http_status "403"           "$RESP_STATUS" "HTTP status"   || ok=1
  assert_eq "L1=DENY/BAD_DOMAIN"    "$nc"          "Nmd-Cache"     || ok=1
  assert_eq "max-age=86400"         "$cc"          "Cache-Control" || ok=1
  return $ok
}

# ---- 13e: ブラックリスト登録CIDR内のIP (198.51.100.0/24 → 198.51.100.5) ----
test_blocked_ip_cidr() {
  local encoded url
  encoded=$(encode_url "http://198.51.100.5/image.png")
  url=$(proxy_url "13e-blocked-ip-cidr.png" "$encoded" "emoji")

  get_response "$url"
  local nc cc
  nc=$(extract_header "nmd-cache"     "$RESP_HEADERS")
  cc=$(extract_header "cache-control" "$RESP_HEADERS")

  local ok=0
  assert_http_status "403"           "$RESP_STATUS" "HTTP status"   || ok=1
  assert_eq "L1=DENY/BAD_DOMAIN"    "$nc"          "Nmd-Cache"     || ok=1
  assert_eq "max-age=86400"         "$cc"          "Cache-Control" || ok=1
  return $ok
}

run_test "13a: Blocked domain (1st entry) → 403"              test_blocked_domain_first
run_test "13b: Blocked domain (case-insensitive, 2nd) → 403"  test_blocked_domain_case_insensitive
run_test "13d: Blocked IP exact (203.0.113.1) → 403"          test_blocked_ip_exact
run_test "13e: Blocked IP in CIDR (198.51.100.5/24) → 403"   test_blocked_ip_cidr
print_summary
