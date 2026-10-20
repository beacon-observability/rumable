#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
image=${NGINX_IMAGE:-nginx:1.28-alpine}
work=$(mktemp -d "${TMPDIR:-/tmp}/rumable-smoke.XXXXXX")
suffix=$(basename "$work" | tr '[:upper:]' '[:lower:]')
server="${suffix}-nginx"
fixture="${suffix}-fixture"
network="${suffix}-network"
cleanup() {
  if [ "${KEEP:-0}" = 1 ]; then
    echo "Kept: $server $fixture $network $work"
  else
    docker rm -f "$server" "$fixture" >/dev/null 2>&1 || true
    docker network rm "$network" >/dev/null 2>&1 || true
    rm -rf "$work"
  fi
}
trap cleanup EXIT
fail() { echo "FAIL: $*" >&2; exit 1; }
expect_failure() {
  local reason=$1; shift
  if "$@" > "$work/expected-failure.log" 2>&1; then fail "Expected failure: $reason"; fi
  cat "$work/expected-failure.log"
  grep -q "$reason" "$work/expected-failure.log" || fail "Unexpected failure, expected: $reason"
}
fetch() { curl -fsS --max-time 15 -D "$work/headers" -o "$work/body" "$@"; }
injected() {
  test "$(grep -o 'data-rumable="smoke"' "$work/body" | wc -l | tr -d ' ')" = 1 || fail "Expected one injection"
}
not_injected() { if grep -q 'data-rumable=' "$work/body"; then fail "Unexpected injection"; fi; }
header() { grep -i "^$1:" "$work/headers" | tr -d '\r' | cut -d ' ' -f 2-; }
clean_headers() {
  if grep -Eiq '^(ETag|Last-Modified|X-Backend-Secret):' "$work/headers"; then
    cat "$work/headers"; fail "Rewritten HTML leaked an original validator or hidden header"
  fi
}
script_url() { sed -n 's/.*data-rumable="smoke" src="\([^"]*\)".*/\1/p' "$work/body"; }
arch=$(docker version --format '{{.Server.Arch}}')
if [ -n "${RUMABLE_BINARY:-}" ]; then
  cp "$RUMABLE_BINARY" "$work/rumable"
  chmod 755 "$work/rumable"
else
  CGO_ENABLED=0 GOOS=linux GOARCH="$arch" go build -o "$work/rumable" ./cmd/rumable
fi
CGO_ENABLED=0 GOOS=linux GOARCH="$arch" go build -tags integration -o "$work/fixture" ./internal/integration/fixture.go
chmod 755 "$work"
docker network create "$network" >/dev/null
docker run -d --name "$fixture" --network "$network" --network-alias fixture -p 127.0.0.1:19181:19181 -v "$work:/work/smoke" --entrypoint /work/smoke/fixture "$image" >/dev/null
docker run -d --name "$server" --network "$network" -p 127.0.0.1:19180:19180 -v "$work:/work/smoke" --entrypoint sh "$image" -c 'while [ ! -f /work/smoke/nginx.conf ]; do sleep 1; done; exec nginx -c /work/smoke/nginx.conf -g "daemon off;"' >/dev/null
cat > "$work/nginx-wrapper" <<'SH'
#!/bin/sh
case " $* " in
  *" -t "*) if [ -f /work/smoke/fail-test ]; then rm /work/smoke/fail-test; echo 'intentional config test failure' >&2; exit 1; fi;;
  *" -s reload "*)
    echo reload >> /work/smoke/reload-attempts
    if [ -f /work/smoke/fail-reload ]; then rm /work/smoke/fail-reload; echo 'intentional reload failure' >&2; exit 1; fi
    if [ -f /work/smoke/fail-probe-on-reload ]; then
      rm /work/smoke/fail-probe-on-reload; touch /work/smoke/fail-probe
    else
      rm -f /work/smoke/fail-probe
    fi;;
esac
exec /usr/sbin/nginx "$@"
SH
chmod 755 "$work/nginx-wrapper"
use_config() {
  # Atomically install in the server namespace; host truncation can be observed
  # before all bind-mounted bytes are visible to the container.
  docker exec "$server" cp "/work/smoke/$1" /work/smoke/config.install
  docker exec "$server" mv /work/smoke/config.install /work/smoke/config.toml
}
docker exec "$server" /work/smoke/rumable init --server nginx --reporting datakit --site smoke --config /work/smoke/config.toml
awk '
  /^publicURL =/ {print "publicURL = \"http://127.0.0.1:19180/\""; next}
  /^probePaths =/ {print "probePaths = [\"/\", \"/static.html\", \"/stream\", \"/split-head\", \"/conditional\", \"/validators\", \"/cache\", \"/inherit/page\", \"/business\"]"; next}
  /^excludePaths =/ {print "excludePaths = [\"/api/\"]"; next}
  /^binary =/ {print "binary = \"/work/smoke/nginx-wrapper\""; print "args = [\"-c\", \"/work/smoke/nginx.conf\"]"; next}
  /^applicationId =/ {print "applicationId = \"rumable-local-verification\""; next}
  /^datakitOrigin =/ {print "datakitOrigin = \"http://127.0.0.1:19181\""; print "env = \"smoke\""; next}
  {print}
' "$work/config.toml" > "$work/config.ready"
use_config config.ready
cli() { docker exec "$server" /work/smoke/rumable "$@" --config /work/smoke/config.toml --state-dir /work/smoke/state; }
reload() {
  docker exec "$server" /work/smoke/nginx-wrapper -c /work/smoke/nginx.conf -t
  docker exec "$server" /work/smoke/nginx-wrapper -c /work/smoke/nginx.conf -s reload
  sleep 0.3
}
cli render
mkdir -p "$work/www"
echo '<!doctype html><html><head><title>Static</title></head><body>Static page</body></html>' > "$work/www/static.html"
gzip -n -c "$work/www/static.html" > "$work/www/static.html.gz"
echo 'gzip_static off;' > "$work/static-compression.conf"
cat > "$work/proxy-headers.conf" <<'NGINX'
proxy_set_header Host fixture.business.test;
proxy_set_header Authorization $http_authorization;
proxy_set_header X-Tenant tenant-preserved;
proxy_set_header Accept-Encoding "";
proxy_set_header If-None-Match "";
proxy_set_header If-Modified-Since "";
NGINX
# Existing business sub_filter rules must be repeated alongside the managed
# location include, because Nginx replaces the parent's entire rule list.
printf "%s\n" "sub_filter 'BUSINESS_ORIGINAL' 'BUSINESS_FILTERED';" "sub_filter 'use strict' 'PARENT_FILTER_CHANGED';" > "$work/business-filter.conf"
cat > "$work/nginx.conf" <<'NGINX'
user root;
pid /work/smoke/nginx.pid;
error_log /dev/stderr notice;
events { worker_connections 128; }
http {
  include /etc/nginx/mime.types;
  include /work/smoke/state/sites/smoke/http.conf;
  access_log off;
  gzip on;
  gzip_min_length 1;
  gzip_vary on;
  gzip_static on;
  sub_filter_types *;
  include /work/smoke/proxy-headers.conf;
  proxy_hide_header X-Backend-Secret;
  proxy_cache_path /work/smoke/cache keys_zone=pages:1m;
  add_header X-Site-Security preserved;
  server {
    listen 19180;
    # 1.9.4 has an upstream crash bug for http-level sub_filter inheritance.
    include /work/smoke/business-filter.conf;
    if (-f /work/smoke/fail-probe) { return 503; }
    include /work/smoke/state/sites/smoke/server.conf;
    location = /static.html {
      root /work/smoke/www;
      include /work/smoke/static-compression.conf;
      include /work/smoke/state/sites/smoke/location.conf;
    }
    location /cache {
      proxy_pass http://fixture:19181;
      proxy_cache pages;
      proxy_cache_valid 200 1h;
      include /work/smoke/business-filter.conf;
      include /work/smoke/state/sites/smoke/location.conf;
    }
    location = /validators {
      proxy_pass http://fixture:19181;
      include /work/smoke/state/sites/smoke/location.conf;
    }
    location /inherit/ {
      # Merging new proxy_set_header entries requires the complete existing list.
      include /work/smoke/proxy-headers.conf;
      location /inherit/page {
        proxy_pass http://fixture:19181;
        include /work/smoke/business-filter.conf;
        include /work/smoke/state/sites/smoke/location.conf;
      }
      location /inherit/headers {
        proxy_pass http://fixture:19181/headers;
        include /work/smoke/state/sites/smoke/location.conf;
      }
    }
    location / {
      proxy_pass http://fixture:19181;
      proxy_http_version 1.1;
      proxy_buffering off;
      include /work/smoke/business-filter.conf;
      include /work/smoke/state/sites/smoke/location.conf;
    }
  }
}
NGINX
base=http://127.0.0.1:19180
for attempt in $(seq 1 30); do
  if curl -fsS "$base/" -o /dev/null 2>/dev/null; then break; fi
  if [ "$(docker inspect "$server" --format '{{.State.Running}}')" != true ]; then
    docker logs "$server"; fail 'Nginx did not start'
  fi
  sleep 1
done
nginx_version=$(docker exec "$server" nginx -v 2>&1)
docker exec "$server" cp /work/smoke/nginx.conf /work/smoke/nginx.before
set_http_directive() {
  docker exec "$server" awk -v "addition=$1" -v destination=/work/smoke/nginx.candidate \
    '{print > destination; if ($0 == "http {") print addition > destination;}' /work/smoke/nginx.before
  docker exec "$server" cp /work/smoke/nginx.candidate /work/smoke/nginx.conf
}
restore_config() { docker exec "$server" cp /work/smoke/nginx.before /work/smoke/nginx.conf; }
refuse_unsafe_activation() {
  local reason=$1
  docker exec "$server" cp /work/smoke/reload-attempts /work/smoke/reload.before
  expect_failure "$reason" cli check
  docker exec "$server" cp /work/smoke/state/sites/smoke/state.json /work/smoke/state.before
  expect_failure "$reason" cli enable
  docker exec "$server" cmp /work/smoke/state/sites/smoke/state.json /work/smoke/state.before
  docker exec "$server" cmp /work/smoke/reload-attempts /work/smoke/reload.before
  docker exec "$server" cmp /work/smoke/nginx.conf /work/smoke/nginx.candidate
  test ! -f "$work/state/sites/smoke/transaction.json"
}
docker exec "$server" touch /work/smoke/reload-attempts
echo 'Stage: first attachment and disabled snapshot retain safe recovery'
expect_failure 'attachmentReviewed=true' cli check
cp "$work/state/sites/smoke/state.json" "$work/state.before"
expect_failure 'attachmentReviewed=true' cli enable
cmp "$work/state/sites/smoke/state.json" "$work/state.before"
test ! -s "$work/reload-attempts"
test ! -f "$work/state/sites/smoke/transaction.json"
cli disable
cmp "$work/state/sites/smoke/state.json" "$work/state.before"
sed 's/attachmentReviewed = false/attachmentReviewed = true/' "$work/config.toml" > "$work/reviewed.toml"
use_config reviewed.toml
if [[ "$nginx_version" == *nginx/1.9.4 ]]; then
  echo 'Stage: automatic 1.9.4 HTTP filter guard, with attachmentReviewed=true'
  docker exec "$server" sh -c 'mkdir -p /work/smoke/http-risk/nested; echo "include http-risk/rules.conf;" > "/work/smoke/http-risk/nested/risk with spaces.conf"; echo "sub_filter GUARD_ORIGINAL GUARD_CHANGED;" > /work/smoke/http-risk/rules.conf'
  for directive in \
    'sub_filter GUARD_ORIGINAL GUARD_CHANGED;' \
    'include http-risk/nested/*.conf;' \
    'include "/work/smoke/http-risk/nested/risk with spaces.conf";'; do
    set_http_directive "$directive"
    refuse_unsafe_activation 'http-level sub_filter'
  done
  # The shared file is parsed first at server/location scope, then at http scope.
  docker exec "$server" awk -v destination=/work/smoke/nginx.candidate \
    '{if ($0 == "}") print "  include business-filter.conf;" > destination; print > destination;}' /work/smoke/nginx.before
  docker exec "$server" cp /work/smoke/nginx.candidate /work/smoke/nginx.conf
  refuse_unsafe_activation 'http-level sub_filter'
  set_http_directive 'include http-risk/nested/[a-z]*.conf;'
  refuse_unsafe_activation 'cannot inspect Nginx 1.9.4'
  set_http_directive "log_format guard '}; sub_filter a b; {'; # sub_filter a b;"
  cli check
  set_http_directive 'include http-risk/empty/*.conf;'
  cli check
  restore_config
else
  echo 'Stage: newer Nginx accepts HTTP filters without the 1.9.4 restriction'
  set_http_directive 'sub_filter GUARD_ORIGINAL GUARD_CHANGED;'
  cli check
  restore_config
fi
echo 'Stage: disabled baseline, warm proxy cache'
fetch "$base/cache"; not_injected
cache_request=$(header X-Fixture-Request)
baseline_etag=$(header ETag || true)
test -n "$cache_request" || fail 'Missing cache fixture counter'
fetch "$base/cache"; test "$(header X-Fixture-Request)" = "$cache_request" || fail 'Proxy cache was not warmed'
fetch "$base/payload.json"; cp "$work/body" "$work/payload.original"
fetch "$base/business"; not_injected; grep -q BUSINESS_FILTERED "$work/body"
fetch "$base/validators"
test "$(header ETag)" = '"original"'; test -n "$(header Last-Modified)"
fetch "$base/static.html"; static_etag=$(header ETag)
test -n "$static_etag"; test -n "$(header Last-Modified)"
cli check
echo 'Stage: stale precompressed scripts reject enable and recover the disabled state'
for script_file in "$work/state/sites/smoke/public/smoke/"init.*.js "$work/state/sites/smoke/public/smoke/"sdk.*.js; do
  printf '/* stale compressed script */' | gzip -n > "$script_file.gz"
  fetch -H 'Accept-Encoding: identity' "$base/__rumable__/smoke/$(basename "$script_file")"
  cmp "$work/body" "$script_file"
  fetch --compressed "$base/__rumable__/smoke/$(basename "$script_file")"
  grep -q 'stale compressed script' "$work/body" || fail 'Stale gzip fixture was not served'
  cp "$work/state/sites/smoke/state.json" "$work/state.before"
  expect_failure 'script checksum mismatch' cli enable
  grep -q 'operation failed and was rolled back' "$work/expected-failure.log"
  cmp "$work/state/sites/smoke/state.json" "$work/state.before"
  test ! -f "$work/state/sites/smoke/transaction.json"
  rm "$script_file.gz"
done
cli enable
cli rollback; cli probe
fetch "$base/"; not_injected
cp "$work/state/sites/smoke/state.json" "$work/state.before"
cli disable; cmp "$work/state/sites/smoke/state.json" "$work/state.before"
touch "$work/fail-test"
expect_failure 'operation failed and was rolled back' cli enable
cmp "$work/state/sites/smoke/state.json" "$work/state.before"
test ! -f "$work/state/sites/smoke/transaction.json"
cli enable
echo 'Stage: compression, HTML boundaries and inherited business configuration'
for path in / /static.html /stream /split-head /conditional /validators /inherit/page /business /cache; do
  for encoding in identity gzip; do
    fetch --compressed -H "Accept-Encoding: $encoding" "$base$path"
    injected; clean_headers
    test "$(header X-Site-Security)" = preserved || fail "Lost security header: $path"
    if [ "$encoding" = gzip ]; then test "$(header Content-Encoding)" = gzip || fail "Output gzip not exercised: $path"; fi
  done
done
old_script=$(script_url)
cli disable --component rum
fetch "$base$old_script"
cmp "$work/body" "$work/state/sites/smoke/public/smoke/${old_script##*/}" || fail 'component disable changed cached-page bootstrap bytes through inherited filters'
cli enable --component rum
cli enable
echo 'Stage: HEAD metadata follows the rewritten GET representation'
for path in /static.html /validators /conditional /cache; do
  for encoding in identity gzip; do
    fetch --head -H "Accept-Encoding: $encoding" -H "If-None-Match: $static_etag" -H 'If-Modified-Since: Mon, 19 Oct 2037 00:00:00 GMT' "$base$path"
    clean_headers
    grep -q '^HTTP/1.1 200 ' "$work/headers" || fail 'HEAD validated stale HTML'
    if grep -iq '^Content-Length:' "$work/headers"; then cat "$work/headers"; fail 'HEAD retained original HTML length'; fi
  done
done
fetch "$base/split-head"; grep -q BUSINESS_FILTERED "$work/body"
fetch "$base/business"; grep -q BUSINESS_FILTERED "$work/body"
fetch "$base/cache"; test "$(header X-Fixture-Request)" = "$cache_request" || fail 'Enable invalidated upstream cache unexpectedly'
fetch "$base$old_script"; grep -q 'max-age=31536000' "$work/headers"
test "$(header X-Site-Security)" = preserved || fail 'SDK resource lost inherited security header'
fetch -H 'Authorization: Bearer fixture-only' -H 'If-None-Match: "original"' -H 'If-Modified-Since: Mon, 19 Oct 2026 00:00:00 GMT' -H 'Accept-Encoding: gzip' "$base/inherit/headers"
grep -q '"host":"fixture.business.test"' "$work/body"
grep -q '"authorization":"Bearer fixture-only"' "$work/body"
grep -q '"tenant":"tenant-preserved"' "$work/body"
for name in encoding etag modified; do grep -q "\"$name\":\"\"" "$work/body" || fail "Upstream received $name"; done
fetch "$base/payload.json"; cmp "$work/body" "$work/payload.original"
for path in /api/data /download /api/html /missinghead /status/503; do
  curl -sS --max-time 15 "$base$path" -o "$work/body"; not_injected
done
for path in /download /api/html /missinghead; do
  fetch "$base$path"; cp "$work/body" "$work/served"
  fetch "http://127.0.0.1:19181$path"; cmp "$work/body" "$work/served"
done
fetch --compressed "$base/forced-gzip"; not_injected
fetch -H 'If-None-Match: "original"' -H 'If-Modified-Since: Mon, 19 Oct 2026 00:00:00 GMT' "$base/conditional"; injected; clean_headers
fetch -H 'If-Modified-Since: Mon, 19 Oct 2037 00:00:00 GMT' "$base/static.html"; injected; clean_headers
fetch -H "If-None-Match: $static_etag" "$base/static.html"; injected; clean_headers
docker exec "$server" cp /work/smoke/config.toml /work/smoke/before.toml
for path in /forced-gzip /missinghead; do
  sed "s|probePaths = .*|probePaths = [\"$path\"]|" "$work/before.toml" > "$work/config.next"
  use_config config.next
  if [ "$path" = /forced-gzip ]; then reason='encoded response'; else reason='no literal </head>'; fi
  expect_failure "$reason" cli check
done
use_config before.toml
echo 'Stage: idempotence, disable, rollback and immutable assets'
cp "$work/state/sites/smoke/state.json" "$work/state.before"
cli enable; cmp "$work/state/sites/smoke/state.json" "$work/state.before"
cli disable
fetch "$base/cache"; not_injected
test "$(header ETag || true)" = "$baseline_etag" || fail 'Disable did not restore baseline validator behavior'
test "$(header X-Fixture-Request)" = "$cache_request" || fail 'Cache no longer hit after disable'
fetch "$base/validators"
test "$(header ETag)" = '"original"'; test -n "$(header Last-Modified)"
fetch --head -H "If-None-Match: $static_etag" "$base/static.html"
grep -q '^HTTP/1.1 304 ' "$work/headers" || fail 'Disable did not restore HEAD conditions'
cp "$work/state/sites/smoke/state.json" "$work/state.before"
cli disable; cmp "$work/state/sites/smoke/state.json" "$work/state.before"
fetch "$base/inherit/headers"; not_injected
fetch "$base$old_script"; test "$(header X-Site-Security)" = preserved
cli rollback; cli probe
fetch "$base/cache"; injected; test "$(script_url)" = "$old_script"
echo 'Stage: reject precompressed static variant and restore deployment'
cli disable
docker exec "$server" sh -c 'echo "gzip_static always;" > /work/smoke/static-compression.conf'; reload
expect_failure 'encoded response' cli check
cp "$work/state/sites/smoke/state.json" "$work/state.before"
expect_failure 'encoded response' cli enable
cmp "$work/state/sites/smoke/state.json" "$work/state.before"
docker exec "$server" sh -c 'echo "gzip_static on;" > /work/smoke/static-compression.conf'; reload
cp "$work/state/sites/smoke/state.json" "$work/state.before"
expect_failure 'operation failed and was rolled back' cli enable
cmp "$work/state/sites/smoke/state.json" "$work/state.before"
test ! -f "$work/state/sites/smoke/transaction.json"
fetch --compressed "$base/static.html"; not_injected
docker exec "$server" sh -c 'echo "gzip_static off;" > /work/smoke/static-compression.conf'; reload
cli enable
echo 'Stage: upgrade, test/reload/post-commit HTTP failures and rollback'
sed -e 's/env = "smoke"/env = "upgraded"/' -e 's/sessionReplayEnabled = false/sessionReplayEnabled = true/' "$work/before.toml" > "$work/config.next"
use_config config.next
for fault in fail-test fail-reload fail-probe-on-reload; do
  cp "$work/state/sites/smoke/state.json" "$work/state.before"
  touch "$work/$fault"
  expect_failure 'operation failed and was rolled back' cli enable
  cmp "$work/state/sites/smoke/state.json" "$work/state.before"
  test ! -f "$work/state/sites/smoke/transaction.json"
  cli probe
  fetch "$base/cache"; test "$(script_url)" = "$old_script"
done
docker exec "$server" sh -c 'echo "# operator-owned change survives upgrade and rollback" >> /work/smoke/proxy-headers.conf'
cp "$work/proxy-headers.conf" "$work/proxy-headers.before"
cli enable
fetch "$base/cache"; injected; new_script=$(script_url)
test "$new_script" != "$old_script" || fail 'Upgrade did not change asset URL'
test "$(header X-Fixture-Request)" = "$cache_request" || fail 'Upgrade did not exercise cached HTML'
fetch "$base$old_script"; fetch "$base$new_script"; grep -q '"env":"upgraded"' "$work/body"
cli rollback
fetch "$base/cache"; injected; test "$(script_url)" = "$old_script"
fetch "$base$new_script"
cmp "$work/proxy-headers.conf" "$work/proxy-headers.before"
echo 'Stage: external managed-file change refuses enable, disable and rollback'
cp "$work/state/sites/smoke/location.conf" "$work/location.backup"
docker exec "$server" sh -c 'echo "# external edit" >> /work/smoke/state/sites/smoke/location.conf'
cp "$work/state/sites/smoke/location.conf" "$work/location.external"
for action in enable disable rollback; do
  expect_failure 'external change' cli "$action"
  cmp "$work/state/sites/smoke/location.conf" "$work/location.external"
done
docker exec "$server" cp /work/smoke/location.backup /work/smoke/state/sites/smoke/location.conf
cli probe; cli status
if [[ "$nginx_version" == *nginx/1.9.4 ]]; then
  echo 'Stage: unsafe external config cannot reactivate through rollback; disable remains available'
  set_http_directive 'include http-risk/nested/*.conf;'
  cli disable
  fetch "$base/"; not_injected
  docker exec "$server" cp /work/smoke/state/sites/smoke/state.json /work/smoke/state.before
  docker exec "$server" cp /work/smoke/reload-attempts /work/smoke/reload.before
  expect_failure 'http-level sub_filter' cli rollback
  docker exec "$server" cmp /work/smoke/state/sites/smoke/state.json /work/smoke/state.before
  docker exec "$server" cmp /work/smoke/reload-attempts /work/smoke/reload.before
  test ! -f "$work/state/sites/smoke/transaction.json"
  restore_config
  cli rollback; cli probe
fi
docker exec "$server" nginx -v
docker image inspect "$image" --format 'Image: {{.Id}}'
docker logs "$server" > "$work/server.log" 2>&1
if grep -Eq 'exited on signal|\[(alert|crit|emerg)\]' "$work/server.log"; then
  cat "$work/server.log"; fail 'Nginx worker or runtime error'
fi
echo "Nginx integration passed. Browser: $base/ ; local event receiver: http://127.0.0.1:19181/__stats"
