#!/usr/bin/env bash
# Automatic first attachment, isolated native services, and original-file recovery.
set -euo pipefail
cd "$(dirname "$0")/.."
kind=${SERVER:-nginx}
case "$kind" in
 nginx) image=${NGINX_IMAGE:-nginx:1.28-alpine}; native=/usr/sbin/nginx;;
 apache) image=${APACHE_IMAGE:-httpd:2.4}; native=/usr/local/apache2/bin/httpd;;
 *) echo 'SERVER must be nginx or apache' >&2; exit 1;;
esac
work=$(mktemp -d "${TMPDIR:-/tmp}/rumable-attach.XXXXXX")
suffix=$(basename "$work" | tr '[:upper:]' '[:lower:]')
server="$suffix-server"; fixture="$suffix-fixture"; network="$suffix-net"
cleanup() {
 if [ "${KEEP:-0}" = 1 ]; then echo "Kept: $server $fixture $network $work"; else
 docker rm -f "$server" "$fixture" >/dev/null 2>&1 || true
 docker network rm "$network" >/dev/null 2>&1 || true; rm -rf "$work"; fi
}
trap cleanup EXIT
fail() { echo "FAIL: $*" >&2; exit 1; }
expect_failure() { local reason=$1; shift; if "$@" > "$work/failure.log" 2>&1; then fail "Expected $reason"; fi; grep -q "$reason" "$work/failure.log" || { cat "$work/failure.log"; fail "$reason"; }; }
arch=$(docker version --format '{{.Server.Arch}}')
if [ -n "${RUMABLE_BINARY:-}" ]; then cp "$RUMABLE_BINARY" "$work/rumable"; else CGO_ENABLED=0 GOOS=linux GOARCH="$arch" go build -o "$work/rumable" ./cmd/rumable; fi
CGO_ENABLED=0 GOOS=linux GOARCH="$arch" go build -tags integration -o "$work/fixture" ./internal/integration/fixture.go
chmod 755 "$work" "$work/rumable"
mkdir "$work/www"
echo '<html><head><title>Static</title></head><body>static</body></html>' > "$work/www/static.html"
gzip -n -c "$work/www/static.html" > "$work/www/static.html.gz"
cat > "$work/wrapper" <<SH
#!/bin/sh
case " \$* " in
 *" -t "*) if [ -f /work/attach/fail-test ]; then rm /work/attach/fail-test; echo 'intentional config failure' >&2; exit 1; fi;;
 *" -s reload "*|*" -k graceful "*)
 echo reload >> /work/attach/reloads
 if [ -f /work/attach/fail-reload ]; then rm /work/attach/fail-reload; echo 'intentional reload failure' >&2; exit 1; fi
 if [ -f /work/attach/fail-probe-next ]; then rm /work/attach/fail-probe-next; touch /work/attach/fail-probe; else rm -f /work/attach/fail-probe; fi;;
esac
exec $native "\$@"
SH
chmod 755 "$work/wrapper"
if [ "$kind" = nginx ]; then
 cat > "$work/native.conf" <<'NGINX'
pid /work/attach/nginx.pid;
error_log /dev/stderr warn;
events {}
http {
 access_log off;
 default_type text/html;
 gzip on;
 gzip_min_length 1;
 gzip_static on;
 proxy_cache_path /tmp/rumable-attach-cache keys_zone=pages:1m;
 proxy_set_header Host fixture.business.test;
 proxy_set_header Authorization $http_authorization;
 proxy_set_header X-Tenant tenant-preserved;
 server {
  listen 19180;
  server_name 127.0.0.1;
  proxy_cache pages;
  proxy_cache_valid 200 1h;
  location /static.html { root /work/attach/www; }
  location / { proxy_pass http://fixture:19181; }
 }
 server { listen 19182; server_name other.test; location / { proxy_pass http://fixture:19181; } }
}
NGINX
else
 cat > "$work/native.conf" <<'APACHE'
ServerRoot /usr/local/apache2
PidFile /work/attach/httpd.pid
Listen 19180
Listen 19182
ServerName localhost
LoadModule mpm_event_module modules/mod_mpm_event.so
LoadModule unixd_module modules/mod_unixd.so
LoadModule authz_core_module modules/mod_authz_core.so
LoadModule alias_module modules/mod_alias.so
LoadModule mime_module modules/mod_mime.so
LoadModule headers_module modules/mod_headers.so
LoadModule filter_module modules/mod_filter.so
LoadModule lua_module modules/mod_lua.so
LoadModule proxy_module modules/mod_proxy.so
LoadModule proxy_http_module modules/mod_proxy_http.so
LoadModule deflate_module modules/mod_deflate.so
User daemon
Group daemon
ErrorLog /proc/self/fd/2
LogLevel warn
TypesConfig conf/mime.types
FilterDeclare GZIP CONTENT_SET
FilterProvider GZIP DEFLATE "%{CONTENT_TYPE} =~ m|^text/html|"
FilterChain +GZIP
<VirtualHost *:19180>
 ServerName 127.0.0.1
 Include /work/attach/target.conf
 DocumentRoot /work/attach/www
 <Directory /work/attach/www>
  AllowOverride None
  Require all granted
 </Directory>
 ProxyPreserveHost On
 RequestHeader set X-Tenant tenant-preserved
 ProxyPass /static.html !
 ProxyPass / http://fixture:19181/
</VirtualHost>
<VirtualHost *:19182>
 ServerName other.test
 Include /work/attach/other.conf
 ProxyPass / http://fixture:19181/
</VirtualHost>
APACHE
fi
printf '<Location "/">\n Header set X-Target preserved\n</Location>\n' > "$work/target.conf"
printf 'SetOutputFilter DEFLATE\n' > "$work/other.conf"
chmod 640 "$work/native.conf"
cp "$work/native.conf" "$work/original.conf"
docker network create "$network" >/dev/null
docker run -d --name "$fixture" --network "$network" --network-alias fixture -p 127.0.0.1:19181:19181 -e RUMABLE_FIXTURE_ALLOW_CDN="${CDN:-0}" -e RUMABLE_FIXTURE_FAILURE_FILE=/work/attach/fail-probe -v "$work:/work/attach" --entrypoint /work/attach/fixture "$image" >/dev/null
if [ "$kind" = nginx ]; then
 docker run -d --name "$server" --network "$network" -p 127.0.0.1:19180:19180 -p 127.0.0.1:19182:19182 -v "$work:/work/attach" --entrypoint "$native" "$image" -c /work/attach/native.conf -g 'daemon off;' >/dev/null
 args='["-c", "/work/attach/native.conf"]'
else
 docker run -d --name "$server" --network "$network" -p 127.0.0.1:19180:19180 -p 127.0.0.1:19182:19182 -v "$work:/work/attach" --entrypoint "$native" "$image" -f /work/attach/native.conf -DFOREGROUND >/dev/null
 args='["-f", "/work/attach/native.conf"]'
fi
for i in $(seq 1 60); do if curl -fsS http://127.0.0.1:19180/static.html >/dev/null 2>&1; then break; fi; sleep 0.25; done
cat > "$work/site.toml" <<TOML
schemaVersion = 1
[[sites]]
id = "smoke"
publicURL = "http://127.0.0.1:19180/"
probePaths = ["/", "/static.html", "/stream", "/conditional", "/cache"]
includePaths = ["/"]
[sites.$kind]
binary = "/work/attach/wrapper"
args = $args
attachmentReviewed = false
[sites.rum]
applicationId = "local-attachment-test"
datakitOrigin = "http://127.0.0.1:19181"
TOML
cli() { docker exec -i "$server" /work/attach/rumable "$@" --config /work/attach/site.toml --state-dir /work/attach/state; }
cache_counter() {
 curl -fsS -D "$work/cache.headers" -o /dev/null http://127.0.0.1:19180/cache
 sed -n 's/^[Xx]-[Ff]ixture-[Rr]equest: *//p' "$work/cache.headers" | tr -d '\r'
}
cli render
if [ "$kind" = nginx ]; then
 warm=$(cache_counter); test -n "$warm" || fail 'missing cache counter'
 test "$(cache_counter)" = "$warm" || fail 'original cache did not warm'
fi
cli attach --dry-run
cmp "$work/native.conf" "$work/original.conf" || fail 'preview mutated server'
expect_failure 'cancelled' cli attach < /dev/null
cmp "$work/native.conf" "$work/original.conf" || fail 'cancel mutated server'
# Modify a valid source only after the confirmation prompt is displayed.
if (
 for i in $(seq 1 100); do if grep -q 'Apply these changes?' "$work/race.log" 2>/dev/null; then break; fi; sleep 0.1; done
 docker exec "$server" sh -c 'echo "# changed during confirmation" >> /work/attach/native.conf'
 printf 'yes\n'
) | cli attach > "$work/race.log" 2>&1; then fail 'changed preview applied'; fi
grep 'configuration changed after preview' "$work/race.log" >/dev/null || { cat "$work/race.log"; fail 'preview race diagnostic'; }
tail -1 "$work/native.conf" | grep 'changed during confirmation' >/dev/null || fail 'preview race overwrote external edit'
docker exec "$server" cp /work/attach/original.conf /work/attach/native.conf
# An external backup appearing during confirmation must never be replaced.
if (
 for i in $(seq 1 100); do if grep -q 'Apply these changes?' "$work/backup-race.log" 2>/dev/null; then break; fi; sleep 0.1; done
 docker exec "$server" sh -c 'echo "external backup" > /work/attach/state/sites/smoke/attachment.json'
 printf 'yes\n'
) | cli attach > "$work/backup-race.log" 2>&1; then fail 'external backup replaced'; fi
grep 'external backup' "$work/state/sites/smoke/attachment.json" >/dev/null || fail 'backup race lost bytes'
cmp "$work/native.conf" "$work/original.conf" || fail 'backup conflict changed config'
docker exec "$server" rm /work/attach/state/sites/smoke/attachment.json
if [ "$kind" = nginx ] && docker exec "$server" "$native" -v 2>&1 | grep -q 'nginx/1.9.4$'; then
 awk '{print; if ($0 == "http {") print " sub_filter unsafe replacement;"}' "$work/original.conf" > "$work/risky.conf"
 docker exec "$server" cp /work/attach/risky.conf /work/attach/native.conf
 expect_failure 'http-level sub_filter' cli attach --yes
 cmp "$work/native.conf" "$work/risky.conf" || fail 'unsafe 1.9.4 config was edited'
 test ! -f "$work/state/sites/smoke/attachment.json" || fail 'unsafe 1.9.4 created backup transaction'
 docker exec "$server" cp /work/attach/original.conf /work/attach/native.conf
fi
# Faults happen at candidate configtest/reload, after discovery has passed.
# Nginx discovery uses -T; Apache uses -t DUMP_INCLUDES, so arm the wrapper only
# when a managed marker is visible in the candidate.
sed 's/if \[ -f \/work\/attach\/fail-test \];/if [ -f \/work\/attach\/fail-test ] \&\& grep -q "# BEGIN rumable-smoke" \/work\/attach\/native.conf;/' "$work/wrapper" > "$work/wrapper.next"
docker exec "$server" cp /work/attach/wrapper.next /work/attach/wrapper
docker exec "$server" chmod 755 /work/attach/wrapper
for fault in fail-test fail-reload fail-probe-next; do
 docker exec "$server" touch "/work/attach/$fault"
 expect_failure 'attachment failed and was restored' cli attach --yes
 cmp "$work/native.conf" "$work/original.conf" || fail "$fault did not restore original"
 cli status | grep '"phase": "detached"' >/dev/null || fail 'recovery state'
done
printf 'yes\n' | cli attach
cli status | grep '"phase": "attached"' >/dev/null || fail 'attachment not recorded'
metadata=$(docker exec "$server" stat -c "%u:%g:%a" /work/attach/native.conf)
original_metadata=$(docker exec "$server" stat -c "%u:%g:%a" /work/attach/original.conf)
test "$metadata" = "$original_metadata" || fail "file metadata changed"
count=$(wc -l < "$work/reloads")
cli attach --yes
test "$(wc -l < "$work/reloads")" = "$count" || fail 'repeat attach reloaded'
cli enable
if [ "$kind" = nginx ]; then first=$(cache_counter); test "$(cache_counter)" != "$first" || fail 'automatic hook did not bypass warm cache'; fi
cli probe
curl -fsS --compressed http://127.0.0.1:19180/static.html | grep 'data-rumable="smoke"' >/dev/null || fail 'gzip static not injected'
if curl -fsS http://127.0.0.1:19182/ | grep -q 'data-rumable='; then fail 'other site changed'; fi
curl -fsS -H 'Authorization: Bearer attach-check' http://127.0.0.1:19180/headers > "$work/headers.json"
grep 'tenant-preserved' "$work/headers.json" >/dev/null || fail 'business headers lost'
expect_failure 'run disable' cli rollback --attachment --yes
cli disable
cli rollback
cli probe
cli disable
if [ "${CDN:-0}" = 1 ]; then
 cli enable
 cp "$work/site.toml" "$work/bundled.toml"
 printf '\n[sites.sdk]\nsource = "cdn"\n' >> "$work/site.toml"
 cli enable
 cli probe
 cli status | grep '"sdkURL": "https://static.guance.com/' >/dev/null || fail 'CDN URL not recorded'
 cli rollback
 cli probe
 if grep -q '"sdkURL"' "$work/state/sites/smoke/state.json"; then
  # The CDN snapshot remains in previous; current must be bundled.
  cli status | grep '"sdkURL": ""' >/dev/null || fail 'rollback did not restore bundled source'
 fi
 docker exec "$server" cp /work/attach/bundled.toml /work/attach/site.toml
 cli disable
fi
cp "$work/native.conf" "$work/attached.conf"
docker exec "$server" sh -c 'echo "# external business change" >> /work/attach/native.conf'
expect_failure 'external change' cli rollback --attachment --yes
tail -1 "$work/native.conf" | grep 'external business change' >/dev/null || fail 'external change overwritten'
docker exec "$server" cp /work/attach/attached.conf /work/attach/native.conf
cli rollback --attachment --dry-run
cmp "$work/native.conf" "$work/attached.conf" || fail 'undo preview changed server'
cli rollback --attachment --yes
cmp "$work/native.conf" "$work/original.conf" || fail 'undo did not restore exact bytes'
test -f "$work/state/sites/smoke/attachment.json" || fail 'backup removed'
cli rollback --attachment --yes
cli attach --yes
cli enable
cli disable
cli rollback --attachment --yes
cmp "$work/native.conf" "$work/original.conf" || fail 'reattach undo failed'
if [ "$kind" = nginx ]; then
 # Graceful reloads acknowledge the signal before the new workers accept every
 # connection; old disabled workers and new cache-enabled workers may overlap.
 restored_ok=0
 for i in $(seq 1 30); do restored=$(cache_counter); if [ "$(cache_counter)" = "$restored" ]; then restored_ok=1; break; fi; sleep 0.1; done
 test "$restored_ok" = 1 || fail 'original cache behavior not restored by attachment undo'
fi
if [ "${KEEP:-0}" = 1 ]; then
 if [ "${CDN:-0}" = 1 ]; then printf '\n[sites.sdk]\nsource = "cdn"\n' >> "$work/site.toml"; fi
 cli attach --yes; cli enable
fi
echo "PASS: automatic $kind attachment, consent, idempotence, compression, isolation, failure recovery, enable/disable/rollback, external-edit conflict, exact undo and retained backups ($image)"
