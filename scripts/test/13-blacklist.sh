#!/usr/bin/env bash
# Test 13: Blacklist — domain and IP/CIDR blocking
#
# 13a: blocked-test-1.invalid (ORIGIN_BLACKLIST_DOMAINS 1件目) → 403, L1=DENY/BAD_DOMAIN, max-age=86400
# 13b: Blocked-Test-2.INVALID (大文字混在、2件目) → 403, L1=DENY/BAD_DOMAIN, max-age=86400
# 13d: 203.0.113.1 (ORIGIN_BLACKLIST_IPS 個別IP) → 403, L1=DENY/BAD_DOMAIN, max-age=86400
# 13e: 198.51.100.5 (ORIGIN_BLACKLIST_IPS CIDR 198.51.100.0/24 内) → 403, L1=DENY/BAD_DOMAIN, max-age=86400
# 13f: blocked-test-3.invalid. (末尾のドット付き) → 403, L1=DENY/BAD_DOMAIN, max-age=86400
# 13g: cdn.blocked-test-1.invalid (登録ドメインのサブドメイン) → 403, L1=DENY/BAD_DOMAIN, max-age=86400
# 13h: httpbin から blocked-test-2.invalid へのリダイレクト → 403, L1=DENY/BAD_DOMAIN, max-age=86400
# 13i: blocked-ip.test (198.51.100.7 に解決される名前。docker-compose.yml の extra_hosts) → 403, L1=DENY/BAD_DOMAIN, max-age=86400
source "$(dirname "$0")/lib.sh"

HTTPBIN_BASE="${HTTPBIN_BASE:-http://httpbin}"

# assert_denied <origin_url> <filename>
# The request must be refused by the blacklist (403, L1=DENY/BAD_DOMAIN).
assert_denied() {
  local encoded url
  encoded=$(encode_url "$1")
  url=$(proxy_url "$2" "$encoded" "emoji")

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

# ---- 13f: 末尾のドット（FQDN 表記）でも同じホスト ----
test_blocked_domain_trailing_dot() {
  assert_denied "http://blocked-test-3.invalid./image.png" "13f-trailing-dot.png"
}

# ---- 13g: 登録ドメインのサブドメイン ----
test_blocked_subdomain() {
  assert_denied "http://cdn.blocked-test-1.invalid/image.png" "13g-subdomain.png"
}

# ---- 13h: リダイレクト先がブラックリスト ----
test_blocked_redirect_target() {
  local target
  target=$(encode_url "http://blocked-test-2.invalid/image.png")
  assert_denied "${HTTPBIN_BASE}/redirect-to?url=${target}" "13h-redirect.png"
}

# ---- 13i: 名前がブラックリストの IP に解決される ----
test_blocked_resolved_ip() {
  assert_denied "http://blocked-ip.test/image.png" "13i-resolved-ip.png"
}

run_test "13a: Blocked domain (1st entry) → 403"              test_blocked_domain_first
run_test "13b: Blocked domain (case-insensitive, 2nd) → 403"  test_blocked_domain_case_insensitive
run_test "13d: Blocked IP exact (203.0.113.1) → 403"          test_blocked_ip_exact
run_test "13e: Blocked IP in CIDR (198.51.100.5/24) → 403"   test_blocked_ip_cidr
run_test "13f: Blocked domain with trailing dot → 403"       test_blocked_domain_trailing_dot
run_test "13g: Subdomain of a blocked domain → 403"          test_blocked_subdomain
run_test "13h: Redirect to a blocked domain → 403"           test_blocked_redirect_target
run_test "13i: Name resolving to a blocked IP → 403"         test_blocked_resolved_ip
print_summary
