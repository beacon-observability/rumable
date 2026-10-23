#!/usr/bin/env bash
set -euo pipefail
repo=$(cd "$(dirname "$0")/../.." && pwd)
cd "$repo"
compose=(docker compose -p rumable-user-demo -f examples/local-demo/compose.yaml)
mkdir -p .local-demo
"${compose[@]}" up -d
"${compose[@]}" exec -T nginx apk add --no-cache bash sudo curl ca-certificates > .local-demo/prepare.log
curl --proto '=https' --proto-redir '=https' -fsSL https://github.com/beacon-observability/rumable/releases/download/v0.1.9/manifest.json -o .local-demo/manifest.json
node --input-type=module <<'JS'
import fs from 'node:fs';
import {makeCommand,makeConfig} from './web/generator.mjs';
const manifest=JSON.parse(fs.readFileSync('.local-demo/manifest.json','utf8'));
const model={server:'nginx',reporting:'openway',applicationId:'rumable-local-demo',clientToken:'local-demo-token',endpoint:'http://127.0.0.1:18874',env:'local-demo',service:'rumable-demo-web',version:'1.0.0',sessionSampleRate:100,sessionOnErrorSampleRate:0,sessionReplayEnabled:false,defaultPrivacyLevel:'mask',sessionPersistence:'cookie',trackUserInteractions:true,remoteConfiguration:false,compressIntakeRequests:false,allowedTracingUrls:''};
fs.writeFileSync('.local-demo/rumable.toml',makeConfig(model));
fs.writeFileSync('.local-demo/command.sh',makeCommand(model,manifest,'https://github.com/beacon-observability/rumable/releases/download/v0.1.9/manifest.json')+'\n');
JS
"${compose[@]}" cp .local-demo/command.sh nginx:/tmp/rumable-demo-command.sh
"${compose[@]}" exec -T nginx bash -o pipefail /tmp/rumable-demo-command.sh | tee .local-demo/install.log
"${compose[@]}" exec -T nginx /opt/rumable/0.1.9/rumable status --state-dir /var/lib/rumable > .local-demo/status.json
"${compose[@]}" exec -T nginx /opt/rumable/0.1.9/rumable probe --state-dir /var/lib/rumable | tee .local-demo/probe.log
"${compose[@]}" exec -T nginx /opt/rumable/0.1.9/rumable probe --state-dir /var/lib/rumable --path /checkout/ | tee -a .local-demo/probe.log
printf '\nDemo ready: http://127.0.0.1:18874/\nLocal intake only. No real application ingestion is performed.\n'
