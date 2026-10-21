#!/usr/bin/env bash
# Real generated Linux command + package integrity + Nginx setup lifecycle.
# Only OSS transfers are substituted by local installer/archive files (no public release).
set -euo pipefail
cd "$(dirname "$0")/.."
work=$(mktemp -d "${TMPDIR:-/tmp}/rumable-onboarding.XXXXXX")
server="$(basename "$work" | tr '[:upper:]' '[:lower:]')"
cleanup() {
  local result=$?
  if [ "$result" != 0 ] && [ -f "$work/failure.log" ]; then cat "$work/failure.log"; fi
  docker rm -f "$server" >/dev/null 2>&1 || true
  rm -rf "$work"
}
trap cleanup EXIT
arch=$(docker version --format '{{.Server.Arch}}')
CGO_ENABLED=0 GOOS=linux GOARCH="$arch" go build -o "$work/rumable" ./cmd/rumable
mkdir "$work/www" "$work/bin"
chmod 755 "$work"
printf '%s' '<html><head><script>const sample="</head>";</script></head><body>OK</body></html>' > "$work/www/index.html"
printf '%s' '<html><body>No head</body></html>' > "$work/www/nohead.html"
cat > "$work/nginx.conf" <<'NGINX'
events {}
http {
  default_type text/html;
  server {
    listen 18080;
    server_name 127.0.0.1;
    location / { root /work/www; }
  }
}
NGINX
cp "$work/nginx.conf" "$work/original.conf"
tar -czf "$work/package.tar.gz" -C "$work" rumable
# No host sudo or external installation. These shims exist only in this container.
cat > "$work/bin/sudo" <<'SH'
#!/bin/sh
exec "$@"
SH
cat > "$work/bin/curl" <<'SH'
#!/bin/sh
destination= source=
while [ "$#" -gt 0 ]; do
  case "$1" in -o) destination=$2; shift 2 ;; https://*) source=$1; shift ;; *) shift ;; esac
done
case "$source" in
  */install.sh) cat /work/install.sh ;;
  */rumable_0.1.1_linux_*.tar.gz) cp /work/package.tar.gz "$destination" ;;
  *) echo 'Unexpected download URL' >&2; exit 1 ;;
esac
SH
chmod 755 "$work/bin/"*
docker run -d --name "$server" -v "$work:/work" --entrypoint nginx nginx:1.28-alpine -c /work/nginx.conf -g 'daemon off;' >/dev/null
for i in $(seq 1 40); do if docker exec "$server" wget -qO- http://127.0.0.1:18080/ >/dev/null 2>&1; then break; fi; sleep 0.2; done
generate() {
  node --input-type=module - "$work" "$arch" "$1" "$2" <<'JS'
import fs from 'node:fs';
import crypto from 'node:crypto';
import {makeCommand} from './web/generator.mjs';
const [work, arch, service, probe] = process.argv.slice(2);
const digest = crypto.createHash('sha256').update(fs.readFileSync(work+'/package.tar.gz')).digest('hex');
const archiveHash = service==='bad-digest'?'0'.repeat(64):digest;
const script = fs.readFileSync('scripts/install.sh','utf8').replaceAll('@@VERSION@@','0.1.1').replaceAll('@@LINUX_386_SHA256@@',archiveHash).replaceAll('@@LINUX_AMD64_SHA256@@',archiveHash).replaceAll('@@LINUX_ARM64_SHA256@@',archiveHash);
fs.writeFileSync(work+'/install.sh',script);
const manifest = {schemaVersion:1,version:'0.1.1',artifactBase:'../0.1.1/',capabilities:['rum-setup-v1','rum-install-v1'],installers:{linux:{file:'install.sh',sha256:crypto.createHash('sha256').update(script).digest('hex')},windows:{file:'install.ps1',sha256:'a'.repeat(64)}},sdk:{version:'3.3.21'},packages:[{os:'linux',arch,flavor:'modern',file:`rumable_0.1.1_linux_${arch}.tar.gz`,sha256:archiveHash}]};
const m = {manualTarget:true,id:'website',applicationId:'fixture',reporting:'datakit',endpoint:'http://127.0.0.1:18080',service,publicURL:'http://127.0.0.1:18080/',server:'nginx',arch,nginxBinary:'/usr/sbin/nginx',nginxConfig:'/work/nginx.conf',includePaths:'/',excludePaths:'',probePaths:probe,sessionSampleRate:100,sessionReplayEnabled:false,defaultPrivacyLevel:'mask',sessionPersistence:'cookie'};
fs.writeFileSync(work+'/command.sh',makeCommand(m,manifest,'https://oss.example.test/latest/manifest.json'));
JS
}
run_generated() { docker exec -e PATH=/work/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin "$server" sh /work/command.sh; }
cli() { docker exec "$server" /opt/rumable/0.1.1/rumable "$@" --state-dir /var/lib/rumable --site website; }
generate bad-digest /
if run_generated > "$work/failure.log" 2>&1; then echo 'Bad checksum was accepted'; exit 1; fi
docker exec "$server" test ! -e /var/lib/rumable/sites/website/state.json
cmp "$work/nginx.conf" "$work/original.conf"
generate fixture /nohead.html
if run_generated > "$work/failure.log" 2>&1; then echo 'Unsupported HTML was enabled'; exit 1; fi
grep -q 'no safe literal <head>' "$work/failure.log"
cli status | grep '"rumEnabled": false'
generate fixture /
run_generated
cli probe
docker exec "$server" cp /var/lib/rumable/sites/website/state.json /work/state-before.json
run_generated
docker exec "$server" cmp /work/state-before.json /var/lib/rumable/sites/website/state.json
generate upgraded /
run_generated
cli rollback
cli probe
cli disable
cli rollback --attachment --yes
cmp "$work/nginx.conf" "$work/original.conf"
echo 'PASS: latest manifest, streamed installer, automatic architecture, generated short command, checksum rejection, first setup, failed preflight retry, idempotence, upgrade, rollback, disable and exact detach. OSS transfer was a local fixture.'
