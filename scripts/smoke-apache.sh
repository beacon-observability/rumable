#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
image=${APACHE_IMAGE:-httpd:2.4}
work=$(mktemp -d "${TMPDIR:-/tmp}/rumable-apache.XXXXXX")
suffix=$(basename "$work" | tr '[:upper:]' '[:lower:]')
server="$suffix-server"; fixture="$suffix-fixture"; network="$suffix-net"
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
header() { grep -i "^$1:" "$work/headers" | tr -d '\r' | cut -d ' ' -f 2-; }
injected() { test "$(grep -o 'data-rumable="smoke"' "$work/body" | wc -l | tr -d ' ')" = 1 || fail 'Expected one injection'; }
not_injected() { if grep -q data-rumable= "$work/body"; then fail 'Unexpected injection'; fi; }
script_url() { sed -n 's/.*data-rumable="smoke" src="\([^"]*\)".*/\1/p' "$work/body"; }
clean_headers() {
  if grep -Eiq '^(ETag|Last-Modified|Content-MD5|Digest|X-Backend-Secret):' "$work/headers"; then cat "$work/headers"; fail 'Original validator or hidden header leaked'; fi
}
arch=$(docker version --format '{{.Server.Arch}}')
if [ -n "${RUMABLE_BINARY:-}" ]; then cp "$RUMABLE_BINARY" "$work/rumable"; chmod 755 "$work/rumable"
else CGO_ENABLED=0 GOOS=linux GOARCH="$arch" go build -o "$work/rumable" ./cmd/rumable; fi
CGO_ENABLED=0 GOOS=linux GOARCH="$arch" go build -tags integration -o "$work/fixture" ./internal/integration/fixture.go
chmod 755 "$work"
docker network create "$network" >/dev/null
docker run -d --name "$fixture" --network "$network" --network-alias fixture -p 127.0.0.1:19181:19181 -v "$work:/work/smoke" --entrypoint /work/smoke/fixture "$image" >/dev/null
docker run -d --name "$server" --network "$network" -p 127.0.0.1:19180:19180 -p 127.0.0.1:19182:19182 -v "$work:/work/smoke" --entrypoint sh "$image" -c 'while [ ! -f /work/smoke/httpd.conf ]; do sleep 1; done; exec httpd -f /work/smoke/httpd.conf -DFOREGROUND' >/dev/null
cat > "$work/apache-wrapper" <<'SH'
#!/bin/sh
case " $* " in
  *" -v "*)
    if [ -f /work/smoke/old-version ]; then echo 'Server version: Apache/2.4.22 (test)'; exit 0; fi;;
  *" -k graceful "*)
    echo reload >> /work/smoke/reload-attempts
    if [ -f /work/smoke/fail-reload ]; then rm /work/smoke/fail-reload; echo 'intentional reload failure' >&2; exit 1; fi
    if [ -f /work/smoke/fail-probe-on-reload ]; then
      rm /work/smoke/fail-probe-on-reload; echo 'Redirect 503 /' > /work/smoke/fault.conf
    else echo '# healthy' > /work/smoke/fault.conf; fi;;
  *" -t "*)
    if [ -f /work/smoke/fail-candidate-test ] && grep -q LuaOutputFilter /work/smoke/state/sites/smoke/apache.conf; then
      rm /work/smoke/fail-candidate-test; echo 'intentional configtest failure' >&2; exit 1
    fi;;
esac
exec /usr/local/apache2/bin/httpd "$@"
SH
chmod 755 "$work/apache-wrapper"
cat > "$work/config.toml" <<'TOML'
schemaVersion = 1
[[sites]]
id = "smoke"
publicURL = "http://127.0.0.1:19180/"
probePaths = ["/", "/static.html", "/stream", "/split-head", "/conditional", "/inherit/page", "/business", "/cache", "/twice.html"]
includePaths = ["/"]
excludePaths = ["/api/"]
[sites.apache]
binary = "/work/smoke/apache-wrapper"
args = ["-f", "/work/smoke/httpd.conf"]
attachmentReviewed = false
[sites.rum]
applicationId = "rumable-local-verification"
datakitOrigin = "http://127.0.0.1:19181"
env = "smoke"
sessionSampleRate = 100
sessionReplayEnabled = false
sessionReplaySampleRate = 100
TOML
cli() { docker exec "$server" /work/smoke/rumable "$@" --config /work/smoke/config.toml --site smoke --state-dir /work/smoke/state; }
cli render
sed -e 's/id = "smoke"/id = "other"/' -e 's/:19180\//:19182\//' -e 's/attachmentReviewed = false/attachmentReviewed = true/' -e 's/probePaths = .*/probePaths = ["\/"]/' "$work/config.toml" > "$work/other.toml"
other_cli() { docker exec "$server" /work/smoke/rumable "$@" --config /work/smoke/other.toml --site other --state-dir /work/smoke/state; }
other_cli render
mkdir -p "$work/www" "$work/cache"
chmod 777 "$work/cache"
printf '<!doctype html><html><head><title>Static</title></HeAd><body>Static page</body></html>' > "$work/www/static.html"
printf '<html><head></HeAd>first\n</head>second</html>' > "$work/www/twice.html"
echo '# healthy' > "$work/fault.conf"
cat > "$work/output-compression.conf" <<'APACHE'
FilterDeclare OUTPUT_GZIP CONTENT_SET
FilterProvider OUTPUT_GZIP DEFLATE "%{CONTENT_TYPE} =~ m|^text/html|"
FilterChain +OUTPUT_GZIP
APACHE
cat > "$work/httpd.conf" <<'APACHE'
ServerRoot /usr/local/apache2
PidFile /work/smoke/httpd.pid
Listen 19180
Listen 19182
ServerName localhost
LoadModule mpm_event_module modules/mod_mpm_event.so
LoadModule unixd_module modules/mod_unixd.so
LoadModule authz_core_module modules/mod_authz_core.so
LoadModule authz_host_module modules/mod_authz_host.so
LoadModule alias_module modules/mod_alias.so
LoadModule mime_module modules/mod_mime.so
LoadModule headers_module modules/mod_headers.so
LoadModule filter_module modules/mod_filter.so
LoadModule lua_module modules/mod_lua.so
LoadModule proxy_module modules/mod_proxy.so
LoadModule proxy_http_module modules/mod_proxy_http.so
LoadModule deflate_module modules/mod_deflate.so
LoadModule substitute_module modules/mod_substitute.so
LoadModule cache_module modules/mod_cache.so
LoadModule cache_disk_module modules/mod_cache_disk.so
User daemon
Group daemon
ErrorLog /proc/self/fd/2
LogLevel warn
TypesConfig conf/mime.types
CacheRoot /work/smoke/cache
CacheHeader on
Include /work/smoke/output-compression.conf
FilterDeclare BUSINESS
FilterProvider BUSINESS SUBSTITUTE "%{CONTENT_TYPE} =~ m|^text/html|"
FilterChain +BUSINESS
<Location "/">
    Substitute "s/BUSINESS_ORIGINAL/BUSINESS_FILTERED/n"
</Location>
Header set X-Site-Security preserved
Header unset X-Backend-Secret
<VirtualHost *:19180>
    ServerName localhost
    DocumentRoot /work/smoke/www
    <Directory /work/smoke/www>
        Require all granted
    </Directory>
    ProxyPreserveHost On
    RequestHeader set X-Tenant tenant-preserved
    ProxyPass /__rumable__/ !
    ProxyPass /static.html !
    ProxyPass /twice.html !
    ProxyPass / http://fixture:19181/
    CacheEnable disk /cache
    <Location "/__rumable__/">
        SetOutputFilter DEFLATE
    </Location>
    Include /work/smoke/fault.conf
    Include /work/smoke/state/sites/smoke/apache.conf
</VirtualHost>
<VirtualHost *:19182>
    ServerName other.localhost
    ProxyPass /__rumable__/ !
    ProxyPass / http://fixture:19181/
    Header set X-Other-Site isolated
    Include /work/smoke/state/sites/other/apache.conf
</VirtualHost>
APACHE
base=http://127.0.0.1:19180
for attempt in $(seq 1 30); do
  if curl -fsS "$base/" -o /dev/null 2>/dev/null; then break; fi
  if [ "$(docker inspect "$server" --format '{{.State.Running}}')" != true ]; then docker logs "$server"; fail 'Apache did not start'; fi
  sleep 1
done
echo 'Stage: disabled baseline, warm Apache cache and inherited filters'
docker exec "$server" sed -i 's/attachmentReviewed = false/attachmentReviewed = true/' /work/smoke/config.toml
fetch "$base/cache"; not_injected; cache_request=$(header X-Fixture-Request)
fetch "$base/cache"; test "$(header X-Fixture-Request)" = "$cache_request"; test "$(header X-Cache)" = 'HIT from localhost'
fetch "$base/static.html"; static_etag=$(header ETag); test -n "$static_etag"
fetch "$base/business"; grep -q BUSINESS_FILTERED "$work/body"
echo 'Stage: missing modules and missing/duplicate attachment refuse activation'
docker exec "$server" cp /work/smoke/httpd.conf /work/smoke/httpd.before
for change in module missing duplicate; do
  case "$change" in
    module) expression='/LoadModule lua_module/d'; reason='lacks loaded lua_module';;
    missing) expression='/Include .*apache.conf/d'; reason='expected exactly one effective Apache include';;
    duplicate) expression='/Include .*apache.conf/p'; reason='expected exactly one effective Apache include';;
  esac
  docker exec "$server" sh -c "sed '$expression' /work/smoke/httpd.before > /work/smoke/httpd.conf"
  expect_failure "$reason" cli check
  docker exec "$server" cp /work/smoke/state/sites/smoke/state.json /work/smoke/state.before
  expect_failure "$reason" cli enable
  docker exec "$server" cmp /work/smoke/state/sites/smoke/state.json /work/smoke/state.before
  test ! -f "$work/state/sites/smoke/transaction.json"
done
docker exec "$server" cp /work/smoke/httpd.before /work/smoke/httpd.conf
docker exec "$server" touch /work/smoke/old-version
expect_failure 'Apache 2.4.23+' cli check
expect_failure 'Apache 2.4.23+' cli enable
docker exec "$server" rm /work/smoke/old-version
cli check
if docker exec "$server" httpd -v | grep -q 'Apache/2.4.23 '; then
  echo 'Stage: legacy untyped inherited gzip is rejected and restored'
  docker exec "$server" cp /work/smoke/output-compression.conf /work/smoke/compression.before
  docker exec "$server" sh -c 'echo "AddOutputFilterByType DEFLATE text/html" > /work/smoke/output-compression.conf'
  docker exec "$server" /work/smoke/apache-wrapper -f /work/smoke/httpd.conf -k graceful
  sleep 0.3
  docker exec "$server" cp /work/smoke/state/sites/smoke/state.json /work/smoke/state.before
  expect_failure 'operation failed and was rolled back' cli enable
  docker exec "$server" cmp /work/smoke/state/sites/smoke/state.json /work/smoke/state.before
  docker exec "$server" cp /work/smoke/compression.before /work/smoke/output-compression.conf
  docker exec "$server" /work/smoke/apache-wrapper -f /work/smoke/httpd.conf -k graceful
  sleep 0.3
fi
cli enable
cli rollback; fetch "$base/"; not_injected
cli rollback; cli probe
other_cli enable
docker exec "$server" cp /work/smoke/state/sites/other/state.json /work/smoke/other.before
echo 'Stage: static/proxy/chunked HTML, gzip and inheritance'
for path in / /static.html /stream /split-head /conditional /inherit/page /business /cache /twice.html; do
  for encoding in identity gzip; do
    fetch --compressed -H "Accept-Encoding: $encoding" "$base$path"; injected; clean_headers
    test "$(header X-Site-Security)" = preserved || fail 'Lost inherited header'
    if [ "$encoding" = gzip ]; then test "$(header Content-Encoding)" = gzip || fail "Output gzip not exercised: $path"; fi
  done
done
old_script=$(script_url)
echo 'Stage: inherited script compression preserves decoded resource bytes'
for script_file in "$work/state/sites/smoke/public/smoke/"init.*.js "$work/state/sites/smoke/public/smoke/"sdk.*.js; do
  fetch --compressed "$base/__rumable__/smoke/$(basename "$script_file")"
  test "$(header Content-Encoding)" = gzip || fail 'Script gzip path was not exercised'
  cmp "$work/body" "$script_file"
done
fetch "$base/business"; grep -q BUSINESS_FILTERED "$work/body"
fetch "$base/twice.html"; grep -q '</head>second' "$work/body"
fetch -H 'Authorization: Bearer fixture-only' -H 'If-None-Match: "original"' -H 'If-Modified-Since: Mon, 19 Oct 2037 00:00:00 GMT' "$base/headers"
grep -q '"host":"127.0.0.1:19180"' "$work/body"; grep -q '"authorization":"Bearer fixture-only"' "$work/body"; grep -q '"tenant":"tenant-preserved"' "$work/body"
for name in etag modified; do grep -q "\"$name\":\"\"" "$work/body"; done
fetch -X POST -H 'If-None-Match: "original"' -H 'If-Modified-Since: preserve-post-condition' "$base/headers"
grep -q '"etag":"\\"original\\""' "$work/body"; grep -q '"modified":"preserve-post-condition"' "$work/body"
fetch -H "If-None-Match: $static_etag" -H 'If-Modified-Since: Mon, 19 Oct 2037 00:00:00 GMT' -H 'Range: bytes=0-9' "$base/static.html"; injected
for path in /static.html /conditional; do
  for encoding in identity gzip; do
    fetch --head -H "Accept-Encoding: $encoding" -H "If-None-Match: $static_etag" -H 'If-Modified-Since: Mon, 19 Oct 2037 00:00:00 GMT' "$base$path"
    clean_headers
    grep -q '^HTTP/1.1 200 ' "$work/headers" || fail 'HEAD validated stale HTML'
    if grep -iq '^Content-Length:' "$work/headers"; then cat "$work/headers"; fail 'HEAD retained original HTML length'; fi
  done
done
for path in /download /api/html /missinghead /payload.json; do
  fetch "$base$path"; not_injected; cp "$work/body" "$work/served"
  fetch "http://127.0.0.1:19181$path"; cmp "$work/body" "$work/served"
done
fetch --compressed "$base/forced-gzip"; not_injected
cp "$work/config.toml" "$work/before.toml"
for path in /forced-gzip /missinghead; do
  sed "s|probePaths = .*|probePaths = [\"$path\"]|" "$work/before.toml" > "$work/config.toml"
  if [ "$path" = /forced-gzip ]; then reason='encoded response'; else reason='no literal </head>'; fi
  expect_failure "$reason" cli check
done
cp "$work/before.toml" "$work/config.toml"
echo 'Stage: idempotence, cache behavior, disable and rollback'
docker exec "$server" cp /work/smoke/state/sites/smoke/state.json /work/smoke/state.before
cli enable; docker exec "$server" cmp /work/smoke/state/sites/smoke/state.json /work/smoke/state.before
cli disable
fetch "$base/cache"; not_injected; test "$(header X-Fixture-Request)" = "$cache_request"
fetch "$base/static.html"; test "$(header ETag)" = "$static_etag"
fetch --head -H "If-None-Match: $static_etag" "$base/static.html"
grep -q '^HTTP/1.1 304 ' "$work/headers" || fail 'Disable did not restore HEAD conditions'
docker exec "$server" cp /work/smoke/state/sites/smoke/state.json /work/smoke/state.before
cli disable; docker exec "$server" cmp /work/smoke/state/sites/smoke/state.json /work/smoke/state.before
fetch "$base$old_script"; test "$(header X-Site-Security)" = preserved
cli rollback; cli probe
echo 'Stage: candidate configtest, graceful reload and HTTP failure recovery'
cli disable
docker exec "$server" touch /work/smoke/fail-candidate-test
expect_failure 'operation failed and was rolled back' cli enable
cli enable
sed -e 's/env = "smoke"/env = "upgraded"/' -e 's/sessionReplayEnabled = false/sessionReplayEnabled = true/' "$work/before.toml" > "$work/config.toml"
for fault in fail-reload fail-probe-on-reload; do
  docker exec "$server" cp /work/smoke/state/sites/smoke/state.json /work/smoke/state.before
  docker exec "$server" touch "/work/smoke/$fault"
  expect_failure 'operation failed and was rolled back' cli enable
  docker exec "$server" cmp /work/smoke/state/sites/smoke/state.json /work/smoke/state.before
  test ! -f "$work/state/sites/smoke/transaction.json"; cli probe
done
cli enable
fetch "$base/"; injected; new_script=$(script_url); test "$new_script" != "$old_script"
fetch "$base$new_script"; grep -q '"env":"upgraded"' "$work/body"
fetch "$base$old_script"
cli rollback; fetch "$base/"; injected; test "$(script_url)" = "$old_script"
fetch "$base$new_script"
echo 'Stage: external managed configuration and private Lua drift'
docker exec "$server" cp /work/smoke/state/sites/smoke/apache.conf /work/smoke/apache.backup
docker exec "$server" sh -c 'echo "# external edit" >> /work/smoke/state/sites/smoke/apache.conf'
for action in enable disable rollback; do expect_failure 'external change' cli "$action"; done
docker exec "$server" cp /work/smoke/apache.backup /work/smoke/state/sites/smoke/apache.conf
filter_file=$(docker exec "$server" sed -n 's/^LuaOutputFilter [^ ]* "\([^"]*\)".*/\1/p' /work/smoke/state/sites/smoke/apache.conf)
docker exec "$server" cp "$filter_file" /work/smoke/filter.backup
docker exec "$server" sh -c 'echo "-- external edit" >> "$1"' sh "$filter_file"
for action in enable disable rollback; do expect_failure 'asset' cli "$action"; done
docker exec "$server" cp /work/smoke/filter.backup "$filter_file"
cli probe; cli status
other_cli probe
fetch http://127.0.0.1:19182/
test "$(grep -o 'data-rumable="other"' "$work/body" | wc -l | tr -d ' ')" = 1
if grep -q 'data-rumable="smoke"' "$work/body"; then fail 'Injection crossed VirtualHosts'; fi
test "$(header X-Other-Site)" = isolated
docker exec "$server" cmp /work/smoke/state/sites/other/state.json /work/smoke/other.before
docker exec "$server" httpd -v
docker image inspect "$image" --format 'Image: {{.Id}}'
docker logs "$server" > "$work/server.log" 2>&1
if grep -Eiq '\[[^]]*:(error|emerg|alert|crit)\]|segmentation|unknown filter' "$work/server.log"; then cat "$work/server.log"; fail 'Apache runtime errors'; fi
echo 'Apache smoke passed: filtering, compression, inheritance, cache, lifecycle and recovery'
