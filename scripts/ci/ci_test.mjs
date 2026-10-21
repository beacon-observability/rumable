import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import {validateEnvironment, runRelease} from './ci.mjs';
import {sha256, releasePaths, artifactsFromDirectory, publishArtifacts, publishLatest} from './publish-oss.mjs';

const version = '0.1.0', revision = 'a'.repeat(40);
const env = {GITLAB_CI:'true', CI_COMMIT_REF_PROTECTED:'true', CI_COMMIT_TAG:`release-oss-v${version}`, CI_COMMIT_SHA:revision,
  OSS_REGION:'oss-test', OSS_BUCKET:'test', OSS_ACCESS_KEY_ID:'fixture-id', OSS_ACCESS_KEY_SECRET:'fixture-secret'};
const manifestPath = releasePaths(version).manifest;

function fixture(t, version = '0.1.0') {
  const cwd = fs.mkdtempSync(path.join(os.tmpdir(), 'rumable-ci-test-'));
  t.after(() => fs.rmSync(cwd, {recursive:true, force:true}));
  const write = (name, value) => { fs.mkdirSync(path.dirname(path.join(cwd, name)), {recursive:true}); fs.writeFileSync(path.join(cwd, name), value); };
  write('internal/rumable/config.go', `var Version = "${version}"\n`);
  const sdk = {version:'3.3.21', sha256:sha256('sdk fixture')};
  write('internal/rumable/assets/sdk.json', JSON.stringify(sdk));
  write('internal/rumable/assets/dataflux-rum.js', 'sdk fixture');
  const files = new Map();
  const packages = [];
  for (const os of ['linux', 'windows']) for (const arch of ['386', 'amd64', 'arm64']) for (const flavor of ['modern', 'legacy']) {
    if (flavor === 'legacy' && arch === 'arm64') continue;
    const file = `rumable_${version}_${os}_${arch}${flavor === 'legacy' ? '_legacy' : ''}${os === 'windows' ? '.zip' : '.tar.gz'}`;
    files.set(file, Buffer.from(file));
    packages.push({os, arch, flavor, file, sha256:sha256(files.get(file))});
  }
  for (const name of ['index.html', 'app.mjs', 'generator.mjs', 'styles.css']) files.set(`setup/${name}`, Buffer.from('page fixture'));
  const installers = {};
  for (const [platform,file] of [['linux','install.sh'],['windows','install.ps1']]) {
    const content = Buffer.from('installer fixture'); files.set(file, content);
    installers[platform] = {file, sha256:sha256(content)};
  }
  files.set('sdk.json', Buffer.from(JSON.stringify(sdk)));
  files.set('manifest.json', Buffer.from(JSON.stringify({schemaVersion:1, version, revision, capabilities:['rum-setup-v1','rum-install-v1'], installers, sdk, packages})));
  const directory = path.join(cwd, 'dist', version);
  const save = () => {
    for (const [name, content] of files) write(`dist/${version}/${name}`, content);
    write(`dist/${version}/SHA256SUMS`, [...files].map(([name, content]) => `${sha256(content)}  ${name}`).join('\n') + '\n');
  };
  save();
  return {cwd, directory, files, save, load:() => artifactsFromDirectory(directory, version, revision)};
}

function store() {
  const objects = new Map(), puts = [];
  return {objects, puts, client:{
    async get(name) { if (!objects.has(name)) throw Object.assign(Error('fixture signed request'), {code:'NoSuchKey'}); return {content:objects.get(name)}; },
    async put(name, content, options) { puts.push({name, options}); objects.set(name, content); }
  }};
}

test('only matching protected release tags with OSS variables can publish', t => {
  const f = fixture(t);
  assert.equal(validateEnvironment(env, f.cwd).version, version);
  assert.equal(validateEnvironment(env, f.cwd).manifest, manifestPath);
  assert(!Object.keys(env).some(k => /PUBLIC_BASE/.test(k)));
  for (const patch of [{GITLAB_CI:'false'}, {CI_COMMIT_REF_PROTECTED:'false'}, {CI_DEBUG_TRACE:'true'}, {CI_COMMIT_TAG:'main'},
    {CI_COMMIT_TAG:'release-oss-v9.0.0'}, {CI_COMMIT_SHA:'uncommitted'}, {OSS_ACCESS_KEY_SECRET:''}]) {
    assert.throws(() => validateEnvironment({...env, ...patch}, f.cwd));
  }
});

test('preflight rejects missing/tampered files, incorrect platform hashes and commit drift', t => {
  const f = fixture(t);
  const artifacts = f.load();
  assert.equal(artifacts.at(-1).name, 'manifest.json');
  assert.equal(artifacts.length, 19);
  fs.writeFileSync(path.join(f.directory, 'setup/app.mjs'), 'tampered');
  assert.throws(f.load, /checksum/);
  f.save();
  fs.unlinkSync(path.join(f.directory, 'setup/styles.css'));
  assert.throws(f.load, /Missing/);
  f.save();
  assert.throws(() => artifactsFromDirectory(f.directory, version, 'b'.repeat(40)), /commit/);
  const manifest = JSON.parse(f.files.get('manifest.json'));
  manifest.packages[0].sha256 = 'b'.repeat(64);
  f.files.set('manifest.json', Buffer.from(JSON.stringify(manifest))); f.save();
  assert.throws(f.load, /package\/hash/);
  manifest.packages.shift(); f.files.set('manifest.json', Buffer.from(JSON.stringify(manifest))); f.save();
  assert.throws(f.load, /supported platform/);
});

test('manifest is uploaded last, all objects are read back, identical retry writes nothing', async t => {
  const f = fixture(t), s = store(), results = [];
  const artifacts = f.load();
  await publishArtifacts(artifacts, s.client, x => results.push(x));
  assert.equal(s.puts.at(-1).name, `rumable/${version}/manifest.json`);
  assert.equal(results.length, artifacts.length);
  assert(s.puts.every(p => p.options.headers['x-oss-forbid-overwrite'] === 'true'));
  assert.equal(s.puts.find(p => p.name.endsWith('app.mjs')).options.mime, 'application/javascript; charset=utf-8');
  s.puts.length = 0;
  await publishArtifacts(artifacts, s.client);
  assert.equal(s.puts.length, 0);
  s.objects.set(artifacts.at(-1).object, Buffer.from('another version'));
  s.objects.delete(artifacts[0].object);
  await assert.rejects(publishArtifacts(artifacts, s.client), /different bytes/);
  assert.equal(s.puts.length, 0);
});

test('failed upload/readback never exposes manifest and retry completes the partial version', async t => {
  const f = fixture(t), s = store(), artifacts = f.load();
  const originalPut = s.client.put;
  let failed = false;
  s.client.put = async (...args) => {
    if (!failed && s.puts.length === 2) { failed = true; throw Error('secret signed URL must not escape'); }
    return originalPut(...args);
  };
  await assert.rejects(publishArtifacts(artifacts, s.client), error => /OSS upload failed/.test(error.message) && !/secret/.test(error.message));
  assert(!s.objects.has(artifacts.at(-1).object));
  await publishArtifacts(artifacts, s.client);
  assert(s.objects.has(artifacts.at(-1).object));
  const bad = store();
  bad.client.put = async (name) => { bad.objects.set(name, Buffer.from('corrupt remote')); };
  await assert.rejects(publishArtifacts(artifacts, bad.client), /read-back checksum/);
  assert(!bad.objects.has(artifacts.at(-1).object));
  const denied = store();
  denied.client.get = async () => { throw Object.assign(Error('credential=secret'), {code:'AccessDenied'}); };
  await assert.rejects(publishArtifacts(artifacts, denied.client), error => /OSS read failed/.test(error.message) && !/secret/.test(error.message));
  assert.equal(denied.puts.length, 0);
});

test('CI builds the pinned SDK and matching commit, and emits a credential-free report', async t => {
  const f = fixture(t), s = store(), commands = [];
  const report = await runRelease({env, cwd:f.cwd, client:s.client, execute:(...args) => commands.push(args)});
  const build = commands.find(([command]) => command === 'go')[1];
  assert(build.includes('--skip-sdk-sync'));
  assert(build.includes(revision));
  assert(build.includes('/opt/go-legacy/bin/go'));
  assert.equal(report.status, 'verified');
  const content = fs.readFileSync(path.join(f.cwd, 'release-report.json'), 'utf8');
  assert(!content.includes(env.OSS_ACCESS_KEY_SECRET));
  assert(!content.includes(env.OSS_ACCESS_KEY_ID));
  assert.equal(report.objects.length, f.load().length + 4);
  assert.deepEqual(report.latest, {status:'verified', version});
  const failed = store();
  await assert.rejects(runRelease({env, cwd:f.cwd, client:failed.client, execute:() => { throw Error('build failed'); }}), /install release dependencies/);
  assert.equal(failed.puts.length, 0);
  assert.equal(JSON.parse(fs.readFileSync(path.join(f.cwd, 'release-report.json'))).status, 'failed');
});

test('latest is promoted after immutable verification, pins assets and avoids mutable caching', async t => {
  const f = fixture(t), s = store(), artifacts = f.load();
  await assert.rejects(publishLatest(artifacts, s.client), /Immutable release/);
  assert.equal(s.puts.length, 0);
  await publishArtifacts(artifacts, s.client);
  const results = [];
  await publishLatest(artifacts, s.client, x => results.push(x));
  assert.equal(results.length, 4);
  assert.equal(s.puts.at(-1).name, 'rumable/latest/manifest.json');
  const latest = JSON.parse(s.objects.get('rumable/latest/manifest.json'));
  assert.equal(latest.artifactBase, '../0.1.0/');
  assert.equal(latest.revision, revision);
  assert(s.objects.get('rumable/latest/install.ps1').equals(f.files.get('install.ps1')));
  const page = s.objects.get('rumable/latest/setup/index.html').toString();
  assert(page.includes('url=../../0.1.0/setup/index.html'));
  assert.equal(new URL('../../0.1.0/setup/index.html','https://example.com/rumable/latest/setup/index.html').pathname, '/rumable/0.1.0/setup/index.html');
  for (const p of s.puts.filter(p => p.name.startsWith('rumable/latest/'))) {
    assert.equal(p.options.headers['Cache-Control'], 'no-store, max-age=0');
    assert(!('x-oss-forbid-overwrite' in p.options.headers));
  }
});

test('latest update failure preserves the previous manifest and a retry completes it', async t => {
  const first = fixture(t), next = fixture(t, '0.1.1'), s = store();
  await publishArtifacts(first.load(), s.client); await publishLatest(first.load(), s.client);
  const previous = s.objects.get('rumable/latest/manifest.json');
  await publishArtifacts(next.load(), s.client);
  const put = s.client.put;
  s.client.put = async (...args) => {
    if (args[0] === 'rumable/latest/install.ps1') throw Error('secret');
    return put(...args);
  };
  await assert.rejects(publishLatest(next.load(), s.client), e => /latest update failed/.test(e.message) && !e.message.includes('secret'));
  assert(s.objects.get('rumable/latest/manifest.json').equals(previous));
  s.client.put = put;
  await publishLatest(next.load(), s.client);
  assert.equal(JSON.parse(s.objects.get('rumable/latest/manifest.json')).version, '0.1.1');
  const get = s.client.get;
  s.client.get = async name => name === 'rumable/latest/install.sh' ? {content:Buffer.from('corrupt readback')} : get(name);
  await assert.rejects(publishLatest(next.load(), s.client), /latest read-back checksum/);
});

test('old releases and prereleases cannot roll latest back; conflicting metadata is refused', async t => {
  const newer = fixture(t, '0.10.0'), older = fixture(t, '0.9.0'), prerelease = fixture(t, '0.11.0-test.1'), s = store();
  await publishArtifacts(newer.load(), s.client); await publishLatest(newer.load(), s.client);
  await publishArtifacts(older.load(), s.client);
  const count = s.puts.length;
  assert.deepEqual(await publishLatest(older.load(), s.client), {status:'skipped_newer',version:'0.10.0'});
  assert.deepEqual(await publishLatest(prerelease.load(), s.client), {status:'skipped_prerelease'});
  assert.equal(s.puts.length, count);
  const changed = JSON.parse(s.objects.get('rumable/latest/manifest.json'));
  changed.revision = 'b'.repeat(40);
  s.objects.set('rumable/latest/manifest.json', Buffer.from(JSON.stringify(changed)));
  await assert.rejects(publishLatest(newer.load(), s.client), /different metadata/);
  assert.equal(s.puts.length, count);
});

test('failed immutable publishing never touches latest', async t => {
  const f = fixture(t), s = store();
  s.client.put = async () => { throw Error('upload unavailable'); };
  await assert.rejects(runRelease({env,cwd:f.cwd,client:s.client,execute:() => {}}), /upload and read back OSS/);
  assert([...s.objects.keys()].every(k => !k.startsWith('rumable/latest/')));
  assert.equal(JSON.parse(fs.readFileSync(path.join(f.cwd, 'release-report.json'))).latest, undefined);
});
