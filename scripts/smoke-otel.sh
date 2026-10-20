#!/usr/bin/env bash
# Real native modules -> real Collector -> local OTLP/protobuf HTTP receiver.
set -euo pipefail
cd "$(dirname "$0")/.."
kind=${SERVER:-nginx}
python=${PYTHON:-python3}
: "${OTELCOL_BINARY:?Set OTELCOL_BINARY to a Linux arm64 otelcol executable for the OrbStack host}"
case "$kind" in
 nginx) image=${NGINX_IMAGE:-rumable-nginx-otel:1.28.3}; arch=arm64; native=/usr/sbin/nginx; module=/usr/lib/nginx/modules/ngx_otel_module.so; platform=linux/arm64;;
 apache) image=${APACHE_IMAGE:-rumable-apache-otel:1.0.4-isolated}; arch=amd64; native=/usr/local/apache2/bin/httpd; module=/opt/otel/WebServerModule/Apache/libmod_apache_otel.so; platform=linux/amd64;;
 *) echo 'SERVER must be nginx or apache' >&2;exit 1;;
esac
work=$(mktemp -d "${TMPDIR:-/tmp}/rumable-otel.XXXXXX")
name=$(basename "$work" | tr '[:upper:]' '[:lower:]')
cleanup(){
 if [ "${KEEP:-0}" = 1 ]; then echo "Kept $work $name-server $name-collector $name-fixture $name-net";else
 docker rm -f "$name-server" "$name-collector" "$name-fixture" >/dev/null 2>&1 || true
 docker network rm "$name-net" >/dev/null 2>&1 || true;rm -rf "$work";fi
}
trap cleanup EXIT
fail(){ echo "FAIL: $*" >&2;exit 1; }
expect_failure(){ local message=$1;shift;if "$@" > "$work/failure.log" 2>&1;then fail "Expected $message";fi;grep -q "$message" "$work/failure.log" || { cat "$work/failure.log";fail "$message"; }; }
if [ -n "${RUMABLE_BINARY:-}" ];then cp "$RUMABLE_BINARY" "$work/rumable";else
CGO_ENABLED=0 GOOS=linux GOARCH=$arch go build -buildvcs=false -o "$work/rumable" ./cmd/rumable
fi
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -buildvcs=false -tags integration -o "$work/fixture" ./internal/integration/otel-fixture.go
cp "$OTELCOL_BINARY" "$work/otelcol"
chmod 755 "$work" "$work/rumable" "$work/fixture" "$work/otelcol"
mkdir "$work/www"
cat > "$work/native-wrapper" <<SH
#!/bin/sh
case " \$* " in
 *" -s reload "*|*" -k graceful "*)
 if [ -f /work/fail-reload ];then rm /work/fail-reload;echo 'intentional reload failure' >&2;exit 1;fi;;
esac
exec $native "\$@"
SH
chmod 755 "$work/native-wrapper"
echo '<html><head><title>static</title></head><body>static</body></html>' > "$work/www/static.html"
if [ "$kind" = nginx ]; then
 cat > "$work/native.conf" <<'CONF'
pid /work/nginx.pid;
error_log /dev/stderr notice;
events {}
http {
 default_type text/html;
 access_log off;
 gzip on;
 gzip_min_length 1;
 proxy_set_header Host preserved.example.com;
 proxy_set_header Authorization $http_authorization;
 server {
  listen 19780;
  server_name 127.0.0.1;
  location /static.html {root /work/www;}
  location / {proxy_pass http://fixture:19781;}
 }
 server {listen 19782;server_name other.test 127.0.0.1;location / {proxy_pass http://fixture:19781;}}
}
CONF
else
 cat > "$work/native.conf" <<'CONF'
ServerRoot /usr/local/apache2
PidFile /work/httpd.pid
Listen 19780
Listen 19782
ServerName localhost
LoadModule mpm_event_module modules/mod_mpm_event.so
LoadModule unixd_module modules/mod_unixd.so
LoadModule authz_core_module modules/mod_authz_core.so
LoadModule alias_module modules/mod_alias.so
LoadModule headers_module modules/mod_headers.so
LoadModule filter_module modules/mod_filter.so
LoadModule lua_module modules/mod_lua.so
LoadModule mime_module modules/mod_mime.so
LoadModule proxy_module modules/mod_proxy.so
LoadModule proxy_http_module modules/mod_proxy_http.so
User daemon
Group daemon
ErrorLog /proc/self/fd/2
TypesConfig conf/mime.types
<VirtualHost *:19780>
 ServerName 127.0.0.1
 DocumentRoot /work/www
 <Directory /work/www>
  Require all granted
  AllowOverride None
 </Directory>
 ProxyPass /static.html !
 ProxyPass / http://fixture:19781/
</VirtualHost>
<VirtualHost *:19782>
 ServerName other.test
 ServerAlias 127.0.0.1
 ProxyPass / http://fixture:19781/
</VirtualHost>
CONF
fi
cp "$work/native.conf" "$work/original.conf"
cat > "$work/config.toml" <<CONF
schemaVersion = 1
[[sites]]
id = "smoke"
publicURL = "http://127.0.0.1:19780/"
includePaths = ["/"]
excludePaths = ["/api/"]
probePaths = ["/", "/static.html"]
[sites.$kind]
binary = "/work/native-wrapper"
args = ["$(if [ "$kind" = nginx ];then echo -c;else echo -f;fi)", "/work/native.conf"]
[sites.rum]
enabled = true
applicationId = "local-acceptance"
datakitOrigin = "http://127.0.0.1:19781"
[sites.otel]
enabled = true
backend = "dataway"
endpoint = "http://fixture:19781/otel"
collectorEndpoint = "http://127.0.0.1:4317"
tokenEnv = "RUMABLE_ACCEPTANCE_TOKEN"
serviceName = "native-$kind"
modulePath = "$module"
sampleRate = 100
excludePaths = ["/excluded/"]
CONF
cp "$work/config.toml" "$work/original.toml"
docker network create "$name-net" >/dev/null
docker run -d --name "$name-fixture" --network "$name-net" --network-alias fixture -p 127.0.0.1:19781:19781 -v "$work:/work" --entrypoint /work/fixture golang:1.26-bookworm >/dev/null
args=(-c /work/native.conf -g 'daemon off;');if [ "$kind" = apache ];then args=(-f /work/native.conf -DFOREGROUND);fi
docker run -d --platform "$platform" --name "$name-server" --network "$name-net" -p 127.0.0.1:19780:19780 -p 127.0.0.1:19782:19782 -v "$work:/work" --entrypoint "$native" "$image" "${args[@]}" >/dev/null
cli(){ docker exec "$name-server" /work/rumable "$@" --config /work/config.toml --state-dir /work/state --site smoke; }
for i in $(seq 1 60);do if curl -fsS http://127.0.0.1:19780/ >/dev/null;then break;fi;sleep 0.2;done
if [ "$kind" = apache ];then
 reject_module(){
  local path=$1 message=$2
  # Write through the Linux mount so configtest sees coherent file sizes under
  # OrbStack; host replacements of different lengths can briefly read stale.
  sed "s#^modulePath = .*#modulePath = \"$path\"#" "$work/config.toml" | docker exec -i "$name-server" sh -c 'cat > /work/rejected.toml'
  local rejected=/work/rejected-$(basename "$path")
  docker exec "$name-server" /work/rumable render --config /work/rejected.toml --state-dir "$rejected" >/dev/null
  expect_failure "$message" docker exec "$name-server" /work/rumable attach --config /work/rejected.toml --state-dir "$rejected" --yes
  cmp "$work/native.conf" "$work/original.conf"
  test ! -f "$work/${rejected#/work/}/sites/smoke/attachment.json" || fail 'module preflight created a live attachment backup'
 }
 if [ -n "${UNPATCHED_APACHE_MODULE:-}" ];then
  cp "$UNPATCHED_APACHE_MODULE" "$work/unpatched.so"
  reject_module /work/unpatched.so 'lacks the per-request isolation fix'
 fi
 # A real ELF dependency with a capability string still lacks Apache's module
 # symbol. Native -t -C must reject it before the first business-file write.
 docker exec "$name-server" sh -c 'cp /opt/otel/sdk_lib/lib/libopentelemetry_webserver_sdk.so /work/invalid-loader.so; printf rumable-apache-isolation-v1 >> /work/invalid-loader.so'
 reject_module /work/invalid-loader.so 'module load preflight failed before attachment'
fi
cli render
collector=$(find "$work/state/sites/smoke/collector" -name '*.json')
collector=/work/${collector#"$work/"}
docker run -d --name "$name-collector" --network "container:$name-server" -e RUMABLE_ACCEPTANCE_TOKEN=local-test-token -v "$work:/work" --entrypoint /work/otelcol golang:1.26-bookworm --config "$collector" >/dev/null
sleep 1
cli attach --dry-run
cmp "$work/native.conf" "$work/original.conf"
cli attach --yes
cli enable
cli enable
cli status
trace(){ curl -fsS -H "traceparent: 00-$1-0123456789abcdef-01" "http://127.0.0.1:$2$3" > "$work/body"; }
assert_received(){
 local id=$1 token=$2
 for i in $(seq 1 40);do
  curl -fsS http://127.0.0.1:19781/records > "$work/records.json"
  if "$python" - "$work/records.json" "$id" "$token" "native-$kind" 2>/dev/null <<'PY'
import json,sys
records=json.load(open(sys.argv[1])) or []
assert any(sys.argv[2] in r['bodyHex'] and r['token']==sys.argv[3] and sys.argv[4].encode().hex() in r['bodyHex'] for r in records)
PY
  then return;fi
  sleep 0.3
 done
 docker logs "$name-server";docker logs "$name-collector";fail "Trace $id not exported with expected token"
}
trace a1111111111111111111111111111111 19780 /headers
assert_received a1111111111111111111111111111111 local-test-token
grep -qi traceparent "$work/body" || fail 'upstream context missing'
trace a2222222222222222222222222222222 19782 /
trace a3333333333333333333333333333333 19780 /excluded/page
sleep 2
curl -fsS http://127.0.0.1:19781/records > "$work/records.json"
if grep -qE 'a2222222222222222222222222222222|a3333333333333333333333333333333' "$work/records.json";then fail 'other site or excluded path instrumented';fi
cli disable --component rum
trace a4444444444444444444444444444444 19780 /
if grep -q data-rumable "$work/body";then fail 'RUM still injected';fi
assert_received a4444444444444444444444444444444 local-test-token
cli rollback
cli probe
cli disable --component otel
trace a5555555555555555555555555555555 19780 /
grep -q data-rumable "$work/body" || fail 'disabling OTel disabled RUM'
sleep 2
curl -fsS http://127.0.0.1:19781/records > "$work/records.json"
if grep -q a5555555555555555555555555555555 "$work/records.json";then fail 'disabled OTel emitted spans';fi
cli rollback
trace a6666666666666666666666666666666 19780 /api/value
assert_received a6666666666666666666666666666666 local-test-token
# Same native gRPC path, now configured as a direct DataKit receiver.
sed -e 's/backend = "dataway"/backend = "datakit"/' -e 's#endpoint = "http://fixture:19781/otel"#endpoint = "http://127.0.0.1:4317"#' -e '/^collectorEndpoint =/d' -e '/^tokenEnv =/d' "$work/config.toml" > "$work/datakit.toml"
cp "$work/datakit.toml" "$work/config.toml"
cli enable
trace a7777777777777777777777777777777 19780 /
assert_received a7777777777777777777777777777777 local-test-token
cli rollback
cp "$work/original.toml" "$work/config.toml"
# A post-write reload failure must restore both independent switches and history.
cp "$work/state/sites/smoke/state.json" "$work/before-failure.json"
docker exec "$name-server" touch /work/fail-reload
expect_failure 'operation failed and was rolled back' cli disable --component otel
cmp "$work/state/sites/smoke/state.json" "$work/before-failure.json"
trace b1111111111111111111111111111111 19780 /
assert_received b1111111111111111111111111111111 local-test-token
# Sampling zero must affect native export even with an incoming sampled parent.
sed 's/sampleRate = 100/sampleRate = 0/' "$work/original.toml" > "$work/config.toml"
cli enable
trace b2222222222222222222222222222222 19780 /
sleep 2
curl -fsS http://127.0.0.1:19781/records > "$work/records.json"
if grep -q b2222222222222222222222222222222 "$work/records.json";then fail 'zero sampling emitted spans';fi
cli rollback
cp "$work/original.toml" "$work/config.toml"
# The second managed site reuses the exporter and can outlive owner collection.
sed -e 's/id = "smoke"/id = "peer"/' -e 's/19780/19782/g' "$work/original.toml" > "$work/peer.toml"
peer(){ docker exec "$name-server" /work/rumable "$@" --config /work/peer.toml --state-dir /work/state --site peer; }
peer render
peer attach --yes
peer enable --component otel
trace b3333333333333333333333333333333 19782 /
assert_received b3333333333333333333333333333333 local-test-token
cli disable --component otel
trace b4444444444444444444444444444444 19782 /
assert_received b4444444444444444444444444444444 local-test-token
cli disable
expect_failure 'still depends' cli rollback --attachment --yes
peer disable
peer rollback --attachment --yes
cli rollback --attachment --yes
cmp "$work/native.conf" "$work/original.conf"
echo "PASS: $kind real spans, DataKit/gRPC, Dataway/protobuf/token, sampling, path/site isolation, shared exporter, independent switches, reload failure recovery, rollback and original attachment restored"
