import fs from 'node:fs';
import path from 'node:path';
import {fileURLToPath} from 'node:url';
import {execFileSync} from 'node:child_process';
import {sha256, ossConfiguration, releasePaths, artifactsFromDirectory, publishArtifacts, createClient} from './publish-oss.mjs';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../..');
export function validateEnvironment(env, cwd) {
  if (env.GITLAB_CI !== 'true' || env.CI_COMMIT_REF_PROTECTED !== 'true') throw Error('Publishing requires a protected GitLab tag pipeline');
  if (env.CI_DEBUG_TRACE === 'true') throw Error('Disable CI_DEBUG_TRACE before using release credentials');
  const match = /^release-oss-v(\d+\.\d+\.\d+(?:-[a-z0-9]+(?:\.[a-z0-9]+)*)?)$/.exec(env.CI_COMMIT_TAG || '');
  if (!match || !/^[a-f0-9]{40,64}$/.test(env.CI_COMMIT_SHA || '')) throw Error('Invalid release tag/commit');
  const version = match[1];
  const source = fs.readFileSync(path.join(cwd, 'internal/rumable/config.go'), 'utf8');
  if (/^var Version = "([^"]+)"$/m.exec(source)?.[1] !== version) throw Error('Release tag must match internal/rumable/config.go Version');
  ossConfiguration(env);
  return {version, revision:env.CI_COMMIT_SHA, ...releasePaths(version)};
}

export async function runRelease({env = process.env, cwd = root, execute, client} = {}) {
  const report = {status:'running', step:'validate release environment', objects:[]};
  const run = execute || ((command, args) => execFileSync(command, args, {cwd, env, stdio:'inherit'}));
  try {
    const config = validateEnvironment(env, cwd);
    Object.assign(report, config, {tag:env.CI_COMMIT_TAG});
    report.step = 'verify pinned SDK';
    const sdk = JSON.parse(fs.readFileSync(path.join(cwd, 'internal/rumable/assets/sdk.json')));
    if (sha256(fs.readFileSync(path.join(cwd, 'internal/rumable/assets/dataflux-rum.js'))) !== sdk.sha256) throw Error('Pinned SDK hash mismatch');
    report.step = 'install release dependencies';
    run('npm', ['ci', '--prefix', 'scripts/ci', '--ignore-scripts', '--no-audit', '--no-fund']);
    report.step = 'test page and CI scripts';
    run('node', ['--test', 'web/generator_test.mjs', 'scripts/ci/ci_test.mjs', 'scripts/installer_test.mjs', 'scripts/release-oss_test.mjs']);
    report.step = 'build and test modern/legacy packages';
    // Sync SDK during version preparation, then commit it. A tag rebuild uses
    // those approved bytes, never whatever latest happens to mean on retry.
    run('go', ['run', 'scripts/release.go', '--legacy-go', '/opt/go-legacy/bin/go',
      '--skip-sdk-sync', '--version', config.version, '--revision', config.revision]);
    report.step = 'validate all release artifacts';
    const artifacts = artifactsFromDirectory(path.join(cwd, 'dist', config.version), config.version, config.revision);
    report.step = 'upload and read back OSS objects';
    await publishArtifacts(artifacts, client || createClient(env), entry => {
      report.objects.push(entry);
      fs.writeFileSync(path.join(cwd, 'release-report.json'), JSON.stringify(report, null, 2) + '\n');
    });
    report.status = 'verified';
    report.step = 'complete';
    return report;
  } catch (error) {
    report.status = 'failed';
    // These validation messages contain field names or fixed text only.
    if (['validate release environment', 'verify pinned SDK'].includes(report.step)) report.reason = error.message;
    throw Error(`Release failed at ${report.step}${report.reason ? ': ' + report.reason : ''}; see build output and release-report.json. OSS may contain partial files; retry the same tag after fixing the failure.`);
  } finally {
    fs.writeFileSync(path.join(cwd, 'release-report.json'), JSON.stringify(report, null, 2) + '\n');
  }
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  runRelease().then(report => console.log(`Release verified. Configuration page OSS key: ${report.setup}`))
    .catch(error => { console.error(error.message); process.exitCode = 1; });
}
