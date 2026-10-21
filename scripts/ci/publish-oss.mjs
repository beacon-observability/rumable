import fs from 'node:fs';
import path from 'node:path';
import {createHash} from 'node:crypto';
import {createRequire} from 'node:module';
import {validateManifest} from '../../web/generator.mjs';

export const sha256 = bytes => createHash('sha256').update(bytes).digest('hex');
const safeName = name => /^[A-Za-z0-9_.-]+(?:\/[A-Za-z0-9_.-]+)*$/.test(name) && !name.split('/').some(p => p === '.' || p === '..');

export function ossConfiguration(env = process.env) {
  const fields = {region:'OSS_REGION', bucket:'OSS_BUCKET', accessKeyId:'OSS_ACCESS_KEY_ID', accessKeySecret:'OSS_ACCESS_KEY_SECRET'};
  for (const name of Object.values(fields)) if (!env[name]?.trim()) throw Error(`Missing CI variable: ${name}`);
  return {...Object.fromEntries(Object.entries(fields).map(([key, name]) => [key, env[name]])),
    ...(env.OSS_STS_TOKEN ? {stsToken:env.OSS_STS_TOKEN} : {}), secure:true};
}

export function releasePaths(version) {
  return {manifest:`rumable/${version}/manifest.json`, setup:`rumable/${version}/setup/index.html`, installer:`rumable/${version}/install.sh`,
    latestManifest:'rumable/latest/manifest.json', latestSetup:'rumable/latest/setup/index.html'};
}

// Load and validate the complete output before making any OSS write.
export function artifactsFromDirectory(directory, version, revision) {
  const checksums = fs.readFileSync(path.join(directory, 'SHA256SUMS'));
  const entries = new Map();
  for (const line of checksums.toString().trim().split('\n')) {
    const match = /^([a-f0-9]{64})  (.+)$/.exec(line);
    if (!match || !safeName(match[2]) || match[2] === 'SHA256SUMS' || entries.has(match[2])) throw Error('Invalid SHA256SUMS');
    entries.set(match[2], match[1]);
  }
  const files = new Map();
  const walk = (relative = '') => {
    for (const entry of fs.readdirSync(path.join(directory, relative), {withFileTypes:true})) {
      const name = relative ? `${relative}/${entry.name}` : entry.name;
      if (!safeName(name)) throw Error('Invalid artifact path');
      if (entry.isDirectory()) walk(name);
      else if (!entry.isFile()) throw Error(`Non-regular artifact: ${name}`);
      else if (name !== 'SHA256SUMS') {
        const bytes = fs.readFileSync(path.join(directory, name));
        if (!bytes.length || sha256(bytes) !== entries.get(name)) throw Error(`Artifact checksum mismatch: ${name}`);
        files.set(name, bytes);
      }
    }
  };
  walk();
  if (files.size !== entries.size) throw Error('Missing release artifact');
  const manifest = validateManifest(JSON.parse(files.get('manifest.json')), 'https://release.invalid/manifest.json');
  if (manifest.version !== version || manifest.revision !== revision) throw Error('Manifest does not match the release tag/commit');
  const platforms = new Set([
    'linux/386/modern', 'linux/amd64/modern', 'linux/arm64/modern',
    'windows/386/modern', 'windows/amd64/modern', 'windows/arm64/modern',
    'linux/386/legacy', 'linux/amd64/legacy', 'windows/386/legacy', 'windows/amd64/legacy'
  ]);
  for (const p of manifest.packages) {
    if (!platforms.delete(`${p.os}/${p.arch}/${p.flavor}`) || p.sha256 !== entries.get(p.file)) throw Error('Manifest package/hash does not match the release');
  }
  if (platforms.size) throw Error('Release is missing supported platform packages');
  for (const installer of Object.values(manifest.installers)) {
    if (installer.sha256 !== entries.get(installer.file)) throw Error('Installer hash does not match the release');
  }
  const publicFiles = new Set([
    'manifest.json', 'sdk.json', 'install.sh', 'install.ps1',
    'setup/index.html', 'setup/app.mjs', 'setup/generator.mjs', 'setup/styles.css',
    ...manifest.packages.map(p => p.file),
  ]);
  // Checksums prove integrity, not eligibility for public distribution. Reject
  // extra files even when they were accidentally added to SHA256SUMS too.
  for (const name of files.keys()) {
    if (!publicFiles.has(name)) throw Error(`Unexpected public artifact: ${name}`);
  }
  for (const name of publicFiles) {
    if (!files.has(name)) throw Error(`Missing release artifact: ${name}`);
  }
  if (JSON.stringify(manifest.sdk) !== JSON.stringify(JSON.parse(files.get('sdk.json')))) throw Error('Manifest SDK metadata differs from sdk.json');
  files.set('SHA256SUMS', checksums);
  // Publish the discoverable entry only after every package/page/checksum is verified.
  return [...files].sort(([a], [b]) => a === 'manifest.json' ? 1 : b === 'manifest.json' ? -1 : a.localeCompare(b))
    .map(([name, content]) => ({name, content, sha256:sha256(content), object:`rumable/${version}/${name}`}));
}

function mime(name) {
  if (name.endsWith('.html')) return 'text/html; charset=utf-8';
  if (name.endsWith('.mjs')) return 'application/javascript; charset=utf-8';
  if (name.endsWith('.css')) return 'text/css; charset=utf-8';
  if (name.endsWith('.json')) return 'application/json';
  if (name.endsWith('.zip')) return 'application/zip';
  if (name.endsWith('.tar.gz')) return 'application/gzip';
  return 'text/plain; charset=utf-8';
}

export async function publishArtifacts(artifacts, client, onVerified = () => {}) {
  const missing = new Set();
  const read = async artifact => {
    try { return (await client.get(artifact.object)).content; }
    catch (error) {
      if (error.code === 'NoSuchKey') return null;
      // ali-oss errors can contain signed headers/temporary credentials.
      throw Error(`OSS read failed: ${artifact.object}`);
    }
  };
  // Retry identical files, but reject a version conflict before uploading anything.
  for (const artifact of artifacts) {
    const remote = await read(artifact);
    if (remote === null) missing.add(artifact.object);
    else if (sha256(remote) !== artifact.sha256) throw Error(`OSS version already contains different bytes: ${artifact.object}`);
  }
  for (const artifact of artifacts) {
    if (missing.has(artifact.object)) {
      try {
        await client.put(artifact.object, artifact.content, {
          mime:mime(artifact.name),
          headers:{'Cache-Control':'public, max-age=31536000, immutable', 'x-oss-forbid-overwrite':'true',
            ...(artifact.name.endsWith('.html') ? {'Content-Disposition':'inline'} : {})}
        });
      } catch { throw Error(`OSS upload failed: ${artifact.object}`); }
    }
    const remote = await read(artifact);
    if (remote === null || sha256(remote) !== artifact.sha256) throw Error(`OSS read-back checksum mismatch: ${artifact.object}`);
    onVerified({object:artifact.object, sha256:artifact.sha256, status:missing.has(artifact.object) ? 'uploaded_verified' : 'already_verified'});
  }
}

function compareStable(a, b) {
  const left = a.split('.').map(BigInt), right = b.split('.').map(BigInt);
  for (let i = 0; i < 3; i++) if (left[i] !== right[i]) return left[i] > right[i] ? 1 : -1;
  return 0;
}

// Run only after the immutable release has been uploaded and read back. Each
// alias is self-contained and pins its archive/page to one immutable version.
export async function publishLatest(artifacts, client, onVerified = () => {}) {
  const byName = new Map(artifacts.map(a => [a.name, a]));
  const release = byName.get('manifest.json');
  const manifest = validateManifest(JSON.parse(release.content), 'https://release.invalid/manifest.json');
  if (!/^\d+\.\d+\.\d+$/.test(manifest.version)) return {status:'skipped_prerelease'};
  const read = async name => {
    try { return (await client.get(name)).content; }
    catch (error) { if (error.code === 'NoSuchKey') return null; throw Error(`OSS read failed: ${name}`); }
  };
  const immutable = await read(release.object);
  if (immutable === null || sha256(immutable) !== release.sha256) throw Error('Immutable release must be verified before updating latest');
  const latestManifest = Buffer.from(JSON.stringify({...manifest, artifactBase:`../${manifest.version}/`}, null, 2) + '\n');
  const previous = await read('rumable/latest/manifest.json');
  if (previous !== null) {
    let current;
    try { current = validateManifest(JSON.parse(previous), 'https://release.invalid/latest/manifest.json'); }
    catch { throw Error('Existing latest manifest is invalid; manual review required'); }
    if (!/^\d+\.\d+\.\d+$/.test(current.version)) throw Error('Existing latest version is not stable');
    const comparison = compareStable(current.version, manifest.version);
    if (comparison > 0) return {status:'skipped_newer', version:current.version};
    if (comparison === 0 && sha256(previous) !== sha256(latestManifest)) throw Error('Latest already contains different metadata for this version');
  }
  const destination = `../../${manifest.version}/setup/index.html`;
  const page = Buffer.from(`<!doctype html><html lang="zh-CN"><head><meta charset="utf-8"><meta http-equiv="refresh" content="0;url=${destination}"><title>Rumable ${manifest.version}</title></head><body><a href="${destination}">打开 Rumable ${manifest.version} 配置页</a></body></html>\n`);
  const aliases = [
    {name:'setup/index.html', content:page},
    ...['install.sh', 'install.ps1'].map(name => ({name, content:byName.get(name).content})),
    {name:'manifest.json', content:latestManifest},
  ];
  for (const alias of aliases) {
    const object = `rumable/latest/${alias.name}`, digest = sha256(alias.content);
    try {
      await client.put(object, alias.content, {mime:mime(alias.name), headers:{'Cache-Control':'no-store, max-age=0', ...(alias.name.endsWith('.html') ? {'Content-Disposition':'inline'} : {})}});
    } catch { throw Error(`OSS latest update failed: ${object}`); }
    const result = await read(object);
    if (result === null || sha256(result) !== digest) throw Error(`OSS latest read-back checksum mismatch: ${object}`);
    onVerified({object, sha256:digest, status:'latest_verified'});
  }
  return {status:'verified', version:manifest.version};
}

export function createClient(env) {
  const OSS = createRequire(import.meta.url)('ali-oss');
  return new OSS(ossConfiguration(env));
}
