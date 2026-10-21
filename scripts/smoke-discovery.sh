#!/usr/bin/env bash
# Generated automatic command, real Nginx, and a controlling terminal for choices.
set -euo pipefail
cd "$(dirname "$0")/.."
work=$(mktemp -d "${TMPDIR:-/tmp}/rumable-discovery.XXXXXX")
server="$(basename "$work" | tr '[:upper:]' '[:lower:]')"
cleanup() {
  local result=$?
  if [ "$result" != 0 ] && [ -f "$work/failure.log" ]; then cat "$work/failure.log"; fi
  docker rm -f "$server" >/dev/null 2>&1 || true
  rm -rf "$work"
}
trap cleanup EXIT
arch=$(docker version --format '{{.Server.Arch}}')
if [ -z "${RUMABLE_PUBLIC_MANIFEST:-}" ]; then
  CGO_ENABLED=0 GOOS=linux GOARCH="$arch" go build -o "$work/rumable" ./cmd/rumable
  tar -czf "$work/package.tar.gz" -C "$work" rumable
fi
mkdir "$work/www" "$work/bin"
chmod 755 "$work"
printf '%s' '<html><head><title>Discovery fixture</title></head><body>OK</body></html>' > "$work/www/index.html"
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
cat > "$work/bin/sudo" <<'SH'
#!/bin/sh
exec "$@"
SH
if [ -z "${RUMABLE_PUBLIC_MANIFEST:-}" ]; then
  cat > "$work/bin/curl" <<'SH'
#!/bin/sh
destination= source=
while [ "$#" -gt 0 ]; do
  case "$1" in -o) destination=$2; shift 2 ;; https://*) source=$1; shift ;; *) shift ;; esac
done
case "$source" in
  */install.sh) cat /work/install.sh ;;
  */rumable_0.1.3_linux_*.tar.gz) cp /work/package.tar.gz "$destination" ;;
  *) echo 'Unexpected download URL' >&2; exit 1 ;;
esac
SH
fi
chmod 755 "$work/bin/"*
docker run -d --name "$server" -v "$work:/work" --entrypoint sh nginx:1.28-alpine -c 'cp /work/nginx.conf /etc/nginx/nginx.conf; exec nginx -g "daemon off;"' >/dev/null
for i in $(seq 1 40); do if docker exec "$server" wget -qO- http://127.0.0.1:18080/ >/dev/null 2>&1; then break; fi; sleep 0.2; done
if [ -n "${RUMABLE_PUBLIC_MANIFEST:-}" ]; then
  docker exec "$server" sh -c 'command -v curl >/dev/null || apk add --no-cache curl'
fi
generate() {
  node --input-type=module - "$work" "$arch" "$1" <<'JS'
import fs from 'node:fs';
import crypto from 'node:crypto';
import {makeCommand} from './web/generator.mjs';
const [work, arch, service] = process.argv.slice(2);
let manifest, source = process.env.RUMABLE_PUBLIC_MANIFEST;
if (source) {
  if (!source.startsWith('https://')) throw Error('Public acceptance requires HTTPS');
  const response = await fetch(source, {redirect:'error'});
  if (!response.ok) throw Error(`Public manifest returned ${response.status}`);
  manifest = await response.json();
  if (manifest.version !== '0.1.3') throw Error('This smoke fixture expects version 0.1.3');
} else {
  const digest = crypto.createHash('sha256').update(fs.readFileSync(work+'/package.tar.gz')).digest('hex');
  const script = fs.readFileSync('scripts/install.sh','utf8').replaceAll('@@VERSION@@','0.1.3').replaceAll(/@@LINUX_(?:386|AMD64|ARM64)_SHA256@@/g,digest);
  fs.writeFileSync(work+'/install.sh',script);
  manifest = {schemaVersion:1,version:'0.1.3',artifactBase:'../0.1.3/',capabilities:['rum-setup-v1','rum-install-v1','rum-discovery-v1'],installers:{linux:{file:'install.sh',sha256:digest},windows:{file:'install.ps1',sha256:digest}},sdk:{version:'3.3.21'},packages:[{os:'linux',arch,flavor:'modern',file:`rumable_0.1.3_linux_${arch}.tar.gz`,sha256:digest}]};
  source = 'https://oss.example.test/rumable/latest/manifest.json';
}
const m = {applicationId:'fixture',reporting:'datakit',endpoint:'http://127.0.0.1:18080',service,server:'nginx',sessionSampleRate:100,sessionReplayEnabled:false,defaultPrivacyLevel:'mask',sessionPersistence:'cookie'};
fs.writeFileSync(work+'/command.sh',makeCommand(m,manifest,source));
JS
}
run_generated() { docker exec -e PATH=/work/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin "$server" sh /work/command.sh; }
cli() { docker exec "$server" /opt/rumable/0.1.3/rumable "$@" --state-dir /var/lib/rumable; }
generate fixture
run_generated
cli probe
docker exec "$server" sh -c 'cp /var/lib/rumable/sites/*/state.json /work/state-before.json'
run_generated
docker exec "$server" sh -c 'cmp /work/state-before.json /var/lib/rumable/sites/*/state.json'
generate upgraded
run_generated
cli rollback
cli probe
cli disable
cli rollback --attachment --yes
docker exec "$server" cmp /etc/nginx/nginx.conf /work/nginx.conf
# Add a second distinct virtual server. A piped installer with --yes must not
# choose the first site when no controlling terminal is available.
python3 - "$work" <<'PY'
import pathlib,sys
root=pathlib.Path(sys.argv[1]); source=(root/'nginx.conf').read_text()
source=source.rsplit('}',1)[0]+'  server {\n    listen 18081;\n    server_name 127.0.0.1;\n    location / { root /work/www; }\n  }\n}\n'
(root/'multi.conf').write_text(source)
PY
docker exec "$server" sh -c 'cp /work/multi.conf /etc/nginx/nginx.conf; nginx -s reload'
generate second
if run_generated > "$work/failure.log" 2>&1; then echo 'Multiple sites silently selected'; exit 1; fi
grep -q 'requires an interactive terminal' "$work/failure.log"
docker exec "$server" cmp /etc/nginx/nginx.conf /work/multi.conf
# Exercise /dev/tty while the install script itself still arrives on stdin.
python3 - "$server" <<'PY'
import os,pty,select,subprocess,sys,time
master,slave=pty.openpty()
p=subprocess.Popen(['docker','exec','-it','-e','PATH=/work/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin',sys.argv[1],'sh','/work/command.sh'],stdin=slave,stdout=slave,stderr=slave)
os.close(slave); output=b''; sent=False; deadline=time.monotonic()+90
try:
    while p.poll() is None and time.monotonic()<deadline:
        if not select.select([master],[],[],0.2)[0]: continue
        try: data=os.read(master,65536)
        except OSError: break
        if not data: break
        output+=data
        if not sent and b'Select a number' in output:
            os.write(master,b'2\n'); sent=True
    if p.poll() is None: p.wait(timeout=5)
    if p.returncode or not sent: raise RuntimeError(output.decode(errors='replace'))
finally:
    if p.poll() is None: p.kill()
    os.close(master)
print(output.decode(errors='replace'))
PY
# Only the selected second server is enabled; first remains detached.
docker exec "$server" sh -c "wget -qO- http://127.0.0.1:18080/ > /work/first.html; wget -qO- http://127.0.0.1:18081/ > /work/second.html"
if grep -q 'data-rumable' "$work/first.html"; then echo 'Unselected site changed'; exit 1; fi
grep -q 'data-rumable' "$work/second.html"
second_id=$(python3 - "$work/second.html" <<'PY'
import re,sys
print(re.search(r'data-rumable="([^"]+)"',open(sys.argv[1]).read()).group(1))
PY
)
cli disable --site "$second_id"
cli rollback --attachment --yes --site "$second_id"
docker exec "$server" cmp /etc/nginx/nginx.conf /work/multi.conf
echo 'PASS: RUM-only generated command, single-site discovery, stable ID, repeat install, update/rollback/disable/exact detach, ambiguous noninteractive rejection, terminal selection through piped installer, and unselected-site isolation.'
if [ -n "${RUMABLE_PUBLIC_MANIFEST:-}" ]; then echo "Public OSS downloads verified through: $RUMABLE_PUBLIC_MANIFEST"; fi
