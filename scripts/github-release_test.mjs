import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import {createHash} from 'node:crypto';
import {validateManifest, makeCommand} from '../web/generator.mjs';
import {writeSiteVersion, newestVersion} from './github-release.mjs';
const model = {manualTarget:true,id:'website',applicationId:'app-test',reporting:'openway',endpoint:'https://openway.example.com',clientToken:'token',service:'web',env:'production',version:'1',sessionSampleRate:'100',sessionReplayEnabled:false,trackUserInteractions:true,defaultPrivacyLevel:'mask',sessionPersistence:'cookie',publicURL:'https://www.example.com/',server:'nginx',nginxBinary:'/usr/sbin/nginx',nginxConfig:'/etc/nginx/nginx.conf',includePaths:'/',excludePaths:'/api/',probePaths:'/',iisSite:'Default Web Site',iisApplication:'/'};
const manifest = {schemaVersion:1,version:'0.1.9',capabilities:['rum-setup-v1','rum-install-v1'],sdk:{version:'3.3.21'},packages:[],installers:{linux:{file:'install.sh',sha256:'a'.repeat(64)},windows:{file:'install.ps1',sha256:'b'.repeat(64)}}};

test('GitHub manifest pins Linux and Windows downloads to its version', () => {
  const base = `https://github.com/beacon-observability/rumable/releases/download/v${manifest.version}/`;
  const m = {...manifest, artifactBase:base};
  const source = 'https://beacon-observability.github.io/rumable/latest/manifest.json';
  validateManifest(m, source);
  assert(makeCommand(model,m,source).includes(base + 'install.sh'));
  assert(makeCommand({...model,server:'iis'},m,source).includes(base + 'install.ps1'));
  for (const artifactBase of [base.replace(manifest.version,'9.9.9'),base.replace('https:','http:'),base.replace('github.com','example.com'),base+'../',base+'?token=a',base.replace('github.com','user@github.com')]) assert.throws(() => validateManifest({...m,artifactBase},source));
});

test('Pages output refuses files that differ from release checksums', t => {
  const directory = fs.mkdtempSync(path.join(os.tmpdir(),'rumable-pages-test-'));
  t.after(() => fs.rmSync(directory,{recursive:true,force:true}));
  const bytes = Buffer.from('page');
  const digest = createHash('sha256').update(bytes).digest('hex');
  const files = new Map([['setup/index.html',bytes],['SHA256SUMS',Buffer.from(digest+'  setup/index.html\n')]]);
  writeSiteVersion(directory,'0.1.9',files);
  assert.equal(fs.readFileSync(path.join(directory,'0.1.9/setup/index.html'),'utf8'),'page');
  files.set('setup/index.html',Buffer.from('changed'));
  assert.throws(() => writeSiteVersion(directory,'0.1.9',files),/checksum mismatch/);
});

test('retries of older versions cannot move latest backwards', () => {
  assert.equal(newestVersion(['0.1.9','0.1.10','0.1.8']),'0.1.10');
  assert.equal(newestVersion(['0.9.99','1.0.0']),'1.0.0');
});
