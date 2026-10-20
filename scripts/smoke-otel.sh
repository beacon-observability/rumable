#!/usr/bin/env bash
# Real native modules -> real Collector -> local OTLP/protobuf HTTP receiver.
set -euo pipefail
cd "$(dirname "$0")/.."
python=${PYTHON:-python3}
: "${OTELCOL_BINARY:?Set OTELCOL_BINARY to a Linux arm64 otelcol executable for the OrbStack host}"
image=${NGINX_IMAGE:-rumable-nginx-otel:1.28.3}
arch=arm64; native=/usr/sbin/nginx; module=/usr/lib/nginx/modules/ngx_otel_module.so; platform=linux/arm64
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
 *" -s reload "*)
 if [ -f /work/fail-reload ];then rm /work/fail-reload;echo 'intentional reload failure' >&2;exit 1;fi;;
esac
exec $native "\$@"
SH
chmod 755 "$work/native-wrapper"
echo '<html><head><title>static</title></head><body>static</body></html>' > "$work/www/static.html"
cat > "$work/native.conf" <<'CONF'
pid /work/nginx.pid;
error_log /dev/stderr notice;
events {}
include /work/http.conf;
CONF
cat > "$work/http.conf" <<'CONF'
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
cp "$work/native.conf" "$work/original.conf"
cp "$work/http.conf" "$work/original-http.conf"
original_config() {
 cmp "$work/native.conf" "$work/original.conf"
 cmp "$work/http.conf" "$work/original-http.conf"
}
cat > "$work/config.toml" <<CONF
schemaVersion = 1
[[sites]]
id = "smoke"
publicURL = "http://127.0.0.1:19780/"
includePaths = ["/"]
excludePaths = ["/api/"]
probePaths = ["/", "/static.html"]
[sites.nginx]
binary = "/work/native-wrapper"
args = ["-c", "/work/native.conf"]
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
serviceName = "native-nginx"
modulePath = "$module"
sampleRate = 100
excludePaths = ["/excluded/"]
CONF
cp "$work/config.toml" "$work/original.toml"
if [ "${INSTALL_OTEL:-0}" = 1 ];then
 sed '/^modulePath = /d' "$work/config.toml" > "$work/auto.toml"
 mv "$work/auto.toml" "$work/config.toml"
  : "${NGINX_OTEL_ARCHIVE:?Set NGINX_OTEL_ARCHIVE to the official exact-version module .deb}"
  cp "$NGINX_OTEL_ARCHIVE" "$work/module.deb"
  digest=$("$python" -c 'import hashlib,sys;print(hashlib.sha256(open(sys.argv[1],"rb").read()).hexdigest())' "$work/module.deb")
  if [ "${NGINX_APT_FIXTURE:-0}" != 1 ];then printf '\n[sites.otel.install]\narchive = "/work/module.deb"\nsha256 = "%s"\n' "$digest" >> "$work/config.toml";fi
 cp "$work/config.toml" "$work/original.toml"
fi
docker network create "$name-net" >/dev/null
docker run -d --name "$name-fixture" --network "$name-net" --network-alias fixture -p 127.0.0.1:19781:19781 -v "$work:/work" --entrypoint /work/fixture golang:1.26-bookworm >/dev/null
args=(-c /work/native.conf -g 'daemon off;')
docker run -d --platform "$platform" --name "$name-server" --network "$name-net" -p 127.0.0.1:19780:19780 -p 127.0.0.1:19782:19782 -v "$work:/work" --entrypoint "$native" "$image" "${args[@]}" >/dev/null
cli(){ docker exec "$name-server" /work/rumable "$@" --config /work/config.toml --state-dir /work/state --site smoke; }
for i in $(seq 1 60);do if curl -fsS http://127.0.0.1:19780/ >/dev/null;then break;fi;sleep 0.2;done

if [ "${INSTALL_OTEL:-0}" = 1 ];then
 if [ "${NGINX_APT_FIXTURE:-0}" = 1 ];then
  # A local repository exists only in this throwaway container. This exercises
  # real apt-cache/apt-get/dpkg-deb without modifying the host repositories.
  docker exec "$name-server" sh -c '
   mkdir /work/repo
   cp /work/module.deb /work/repo/module.deb
   dpkg-deb -f /work/module.deb > /work/repo/Packages
   printf "Filename: module.deb\nSize: %s\nSHA256: %s\n" "$(wc -c < /work/module.deb)" "$(sha256sum /work/module.deb | cut -d " " -f 1)" >> /work/repo/Packages
   printf "deb [trusted=yes] file:/work/repo ./\n" > /etc/apt/sources.list.d/rumable-acceptance.list
   apt-get -o Dir::Etc::sourcelist=/etc/apt/sources.list.d/rumable-acceptance.list -o Dir::Etc::sourceparts=- update > /work/apt-update.log 2>&1
  '
 fi
 cli install-otel --dry-run
 test ! -d "$work/state-otel" || fail 'dry-run installed a dependency'
 cli install-otel --yes
 cli install-otel --yes
 original_config
fi
cli render
collector=$(find "$work/state/sites/smoke/collector" -name '*.json')
collector=/work/${collector#"$work/"}
docker run -d --name "$name-collector" --network "container:$name-server" -e RUMABLE_ACCEPTANCE_TOKEN=local-test-token -v "$work:/work" --entrypoint /work/otelcol golang:1.26-bookworm --config "$collector" >/dev/null
sleep 1
cli attach --dry-run
original_config
cli attach --yes
cli enable
cli enable
cli status
if [ "${INSTALL_OTEL:-0}" = 1 ];then
 # Losing state must never turn an actively loaded dependency into a removable
 # installation-only package. The live server remains unchanged on rejection.
 docker exec "$name-server" mv /work/state/sites/smoke/state.json /work/saved-live-state.json
 expect_failure 'still referenced' cli uninstall --yes --remove-dependencies
 docker exec "$name-server" mv /work/saved-live-state.json /work/state/sites/smoke/state.json
fi
trace(){ curl -fsS -H "traceparent: 00-$1-0123456789abcdef-01" "http://127.0.0.1:$2$3" > "$work/body"; }
assert_received(){
 local id=$1 token=$2
 for i in $(seq 1 40);do
  curl -fsS http://127.0.0.1:19781/records > "$work/records.json"
  if "$python" - "$work/records.json" "$id" "$token" "native-nginx" 2>/dev/null <<'PY'
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
# Exercise receiver downtime after export assertions: native SDKs may drop
# queued spans while a receiver is stopped; delivery during an outage is not tested.
docker stop "$name-collector" >/dev/null
peer enable --component rum
peer disable --component rum
docker start "$name-collector" >/dev/null
expect_failure 'still depends' cli uninstall --yes
cli disable
expect_failure 'still depends' cli rollback --attachment --yes
peer disable
peer rollback --attachment --yes
# Model an interrupted two-file restore: main is original, while http still
# contains OTel directives. The on-disk candidate cannot pass -T until resumed.
docker exec "$name-server" cp /work/original.conf /work/native.conf
"$python" - "$work/state/sites/smoke/attachment.json" <<'PY'
import json,sys
path=sys.argv[1]
with open(path) as f: record=json.load(f)
record['phase']='restoring'
with open(path,'w') as f: json.dump(record,f)
PY
expect_failure 'unknown directive' docker exec "$name-server" "$native" -c /work/native.conf -t
cli rollback --attachment --yes
original_config
peer uninstall --yes
cli uninstall --dry-run
test -f "$work/state/sites/smoke/state.json" || fail 'dry-run uninstalled the site'
if [ "${INSTALL_OTEL:-0}" = 1 ];then
 cli uninstall --yes --remove-dependencies
 test -z "$(find "$work/state-otel" -name rumable-install.json)" || fail 'owned dependency not removed'
else
 cli uninstall --yes
 docker exec "$name-server" test -f "$module" || fail 'external module removed'
fi
cli uninstall --yes
test ! -f "$work/state/sites/smoke/state.json" || fail 'site state not archived'
original_config
echo "PASS: nginx real spans, DataKit/gRPC, Dataway/protobuf/token, sampling, path/site isolation, shared exporter, independent switches, reload failure recovery, rollback and original attachment restored"
