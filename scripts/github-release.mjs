import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import {execFileSync} from 'node:child_process';
import {fileURLToPath} from 'node:url';
import {artifactsFromDirectory, sha256} from './ci/publish-oss.mjs';

const gh = args => execFileSync('gh', args, {maxBuffer:128 * 1024 * 1024});
const setupNames = ['index.html', 'app.mjs', 'generator.mjs', 'i18n.mjs', 'styles.css'];
export function newestVersion(versions) {
  return [...versions].sort((a,b) => {
    const x=a.split('.').map(BigInt), y=b.split('.').map(BigInt);
    for (let i=0;i<3;i++) if (x[i] !== y[i]) return x[i] < y[i] ? -1 : 1;
    return 0;
  }).at(-1);
}
function publishedReleases(repository) {
  return JSON.parse(gh(['release','list','--repo',repository,'--limit','1000','--json','tagName,isDraft,isPrerelease']));
}
export function releaseArtifacts(directory, version, revision, repository) {
  if (!/^[A-Za-z0-9][A-Za-z0-9-]*\/[A-Za-z0-9][A-Za-z0-9_.-]*$/.test(repository)) throw Error('Invalid GitHub repository');
  const artifacts = artifactsFromDirectory(directory, version, revision);
  const manifest = JSON.parse(artifacts.find(a => a.name === 'manifest.json').content);
  if (manifest.artifactBase !== `https://github.com/${repository}/releases/download/v${version}/`) throw Error('Manifest must pin downloads to this GitHub release');
  return artifacts;
}

function download(repository, tag, names, directory) {
  gh(['release', 'download', tag, '--repo', repository, '--dir', directory, ...names.flatMap(name => ['--pattern', name])]);
}
function verifyFiles(directory, artifacts) {
  for (const artifact of artifacts) {
    if (sha256(fs.readFileSync(path.join(directory, artifact.name))) !== artifact.sha256) throw Error(`Remote checksum mismatch: ${artifact.name}`);
  }
}
export function writeSiteVersion(site, version, files) {
  const sums = new Map(files.get('SHA256SUMS').toString().trim().split('\n').map(line => [line.slice(66), line.slice(0,64)]));
  for (const [name, bytes] of files) {
    if (name !== 'SHA256SUMS' && sha256(bytes) !== sums.get(name)) throw Error(`Page checksum mismatch: ${name}`);
    const destination = path.join(site, version, name);
    fs.mkdirSync(path.dirname(destination), {recursive:true});
    fs.writeFileSync(destination, bytes);
  }
}

async function run() {
  const [command, directory, version, revision, repository, extra] = process.argv.slice(2);
  const artifacts = releaseArtifacts(directory, version, revision, repository);
  const assets = artifacts.filter(a => !a.name.startsWith('setup/'));
  const tag = `v${version}`;
  if (command === 'validate') { console.log(`Verified ${artifacts.length} release files for ${tag} (${revision})`); return; }
  if (command === 'site') {
    if (!extra) throw Error('Missing site output directory');
    // Rebuild older published pages from their tagged sources and checksum files.
    // Releases retain the archives; Pages contains only configuration UI/metadata.
    const releases = publishedReleases(repository);
    const latest = newestVersion([version,...releases.filter(r => !r.isDraft && !r.isPrerelease && /^v\d+\.\d+\.\d+$/.test(r.tagName)).map(r => r.tagName.slice(1))]);
    for (const release of releases) {
      if (release.isDraft || release.isPrerelease || release.tagName === tag || !/^v\d+\.\d+\.\d+$/.test(release.tagName)) continue;
      const temporary = fs.mkdtempSync(path.join(os.tmpdir(), 'rumable-page-'));
      try {
        download(repository, release.tagName, ['manifest.json','sdk.json','SHA256SUMS'], temporary);
        const files = new Map(['manifest.json','sdk.json','SHA256SUMS'].map(name => [name,fs.readFileSync(path.join(temporary,name))]));
        for (const name of setupNames) files.set(`setup/${name}`, execFileSync('git',['show',`${release.tagName}:web/${name}`]));
        writeSiteVersion(extra, release.tagName.slice(1), files);
      } finally { fs.rmSync(temporary,{recursive:true,force:true}); }
    }
    const files = new Map(artifacts.filter(a => a.name.startsWith('setup/') || ['manifest.json','sdk.json','SHA256SUMS'].includes(a.name)).map(a => [a.name,a.content]));
    writeSiteVersion(extra, version, files);
    fs.mkdirSync(path.join(extra,'latest','setup'),{recursive:true});
    fs.writeFileSync(path.join(extra,'latest','manifest.json'), fs.readFileSync(path.join(extra,latest,'manifest.json')));
    const redirect = target => `<!doctype html><html lang="en"><meta charset="utf-8"><meta http-equiv="refresh" content="0;url=${target}"><title>Rumable setup</title><a href="${target}">Open Rumable setup</a></html>\n`;
    fs.writeFileSync(path.join(extra,'latest','setup','index.html'), redirect(`../../${latest}/setup/index.html`));
    fs.writeFileSync(path.join(extra,'index.html'), redirect('latest/setup/index.html'));
    fs.writeFileSync(path.join(extra,'.nojekyll'),'');
    return;
  }
  if (command === 'publish') {
    if (!extra || !fs.statSync(extra).isFile()) throw Error('Missing release notes file');
    const result = execFileSync('gh',['api',`repos/${repository}/releases/tags/${tag}`],{encoding:'utf8',stdio:['ignore','pipe','pipe']});
    const release = JSON.parse(result);
    if (release.target_commitish !== revision) throw Error('Release points to a different commit');
    if (release.assets.some(asset => !assets.some(a => a.name === asset.name))) throw Error('Unexpected existing release asset');
    const temporary = fs.mkdtempSync(path.join(os.tmpdir(),'rumable-verify-'));
    try {
      const existing = assets.filter(a => release.assets.some(asset => asset.name === a.name));
      if (existing.length) { download(repository,tag,existing.map(a => a.name),temporary); verifyFiles(temporary,existing); }
      const missing = assets.filter(a => !existing.includes(a));
      if (missing.length && !release.draft) throw Error('Published release is incomplete; refusing to modify it');
      for (const artifact of missing) gh(['release','upload',tag,path.join(directory,artifact.name),'--repo',repository]);
      const check = path.join(temporary,'complete'); fs.mkdirSync(check);
      download(repository,tag,assets.map(a => a.name),check); verifyFiles(check,assets);
      if (release.draft) {
        const stable = publishedReleases(repository).filter(r => !r.isDraft && !r.isPrerelease && /^v\d+\.\d+\.\d+$/.test(r.tagName)).map(r => r.tagName.slice(1));
        const latest = newestVersion([...stable,version]) === version;
        gh(['release','edit',tag,'--repo',repository,'--draft=false',`--latest=${latest}`,'--title',`Rumable ${version}`,'--notes-file',extra]);
      }
      console.log(`Published and verified ${assets.length} assets: https://github.com/${repository}/releases/tag/${tag}`);
    } finally { fs.rmSync(temporary,{recursive:true,force:true}); }
    return;
  }
  if (command === 'verify-public') {
    for (const artifact of assets) {
      const body = execFileSync('curl',['--proto','=https','--proto-redir','=https','--retry','4','--retry-all-errors','--max-time','180','-fsSL',`https://github.com/${repository}/releases/download/${tag}/${artifact.name}`],{maxBuffer:128*1024*1024});
      if (sha256(body) !== artifact.sha256) throw Error(`Public checksum mismatch: ${artifact.name}`);
    }
    console.log(`Verified anonymous HTTPS downloads for all ${assets.length} release assets`);
    return;
  }
  throw Error('Expected validate, site, publish, or verify-public');
}
if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) await run();
