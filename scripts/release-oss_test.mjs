import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import {execFileSync, spawnSync} from 'node:child_process';

const source = fs.readFileSync(new URL('./release-oss.sh', import.meta.url));
const env = {...process.env, GIT_CONFIG_GLOBAL:'/dev/null', GIT_CONFIG_NOSYSTEM:'1', GIT_TERMINAL_PROMPT:'0'};
const tag = 'refs/tags/release-oss-v0.1.0';

function fixture(t) {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'rumable-release-tag-'));
  t.after(() => fs.rmSync(root, {recursive:true, force:true}));
  const cwd = path.join(root, 'work'), remote = path.join(root, 'remote.git');
  fs.mkdirSync(cwd);
  const git = (...args) => execFileSync('git', args, {cwd, env, encoding:'utf8', stdio:['ignore','pipe','pipe']}).trim();
  git('init', '-b', 'main'); git('init', '--bare', remote);
  git('config', 'user.name', 'Release test'); git('config', 'user.email', 'release-test@localhost');
  git('remote', 'add', 'origin', remote);
  const write = (name, content) => {const file = path.join(cwd, name); fs.mkdirSync(path.dirname(file), {recursive:true}); fs.writeFileSync(file, content);};
  write('scripts/release-oss.sh', source);
  write('internal/rumable/config.go', 'package rumable\nvar Version = "0.1.0"\n');
  write('.gitlab-ci.yml', 'stages: [release]\n');
  git('add', '.'); git('commit', '-m', 'release fixture');
  const run = (...args) => spawnSync('bash', ['scripts/release-oss.sh', ...args], {cwd, env, encoding:'utf8'});
  const refs = target => git('--git-dir', target, 'for-each-ref', '--format=%(refname)');
  return {root,cwd,remote,git,write,run,refs};
}
function succeeds(result) {assert.equal(result.status, 0, result.stdout + result.stderr);}
function rejects(result, message) {assert.notEqual(result.status, 0); assert.match(result.stdout + result.stderr, message);}

test('dry run is read-only; actual release pushes only the annotated tag and can be repeated', t => {
  const f = fixture(t), head = f.git('rev-parse', 'HEAD');
  succeeds(f.run('0.1.0', '--dry-run'));
  assert.equal(f.git('tag', '--list'), ''); assert.equal(f.refs(f.remote), '');
  f.git('config', 'push.followTags', 'true'); f.git('tag', '-a', 'unrelated', '-m', 'not a release');
  succeeds(f.run());
  assert.equal(f.refs(f.remote), tag);
  assert.equal(f.git('cat-file', '-t', tag), 'tag');
  assert.equal(f.git('--git-dir', f.remote, 'rev-parse', `${tag}^{commit}`), head);
  const object = f.git('--git-dir', f.remote, 'rev-parse', tag);
  assert.match(f.run('0.1.0').stdout, /already points to/);
  assert.equal(f.git('--git-dir', f.remote, 'rev-parse', tag), object);
});

test('version mismatch and pending tracked/untracked work cannot create or push release tags', t => {
  const f = fixture(t);
  rejects(f.run('0.2.0'), /Version must match/);
  rejects(f.run('--remote'), /requires a name/);
  rejects(f.run('v0.1.0'), /Invalid release version/);
  f.write('pending.txt', 'pending');
  rejects(f.run('0.1.0'), /pending changes/);
  fs.unlinkSync(path.join(f.cwd, 'pending.txt'));
  f.write('internal/rumable/config.go', 'modified');
  rejects(f.run('0.1.0'), /pending changes/);
  assert.equal(f.git('tag', '--list'), ''); assert.equal(f.refs(f.remote), '');
});

test('different local or remote release commits are refused without replacement', t => {
  const f = fixture(t);
  f.git('tag', '-a', 'release-oss-v0.1.0', '-m', 'first');
  f.write('next.txt', 'next'); f.git('add', '.'); f.git('commit', '-m', 'next');
  rejects(f.run('0.1.0'), /Local release tag points/);
  assert.equal(f.refs(f.remote), '');
  f.git('push', 'origin', `${tag}:${tag}`);
  const before = f.git('--git-dir', f.remote, 'rev-parse', tag);
  f.git('tag', '-d', 'release-oss-v0.1.0');
  rejects(f.run('0.1.0'), /Remote release tag points/);
  assert.equal(f.git('tag', '--list'), '');
  assert.equal(f.git('--git-dir', f.remote, 'rev-parse', tag), before);
});

test('rejected pushes retain the local tag and can be retried without recreating it', t => {
  const f = fixture(t), hook = path.join(f.remote, 'hooks/pre-receive');
  fs.writeFileSync(hook, '#!/bin/sh\nexit 1\n', {mode:0o755});
  rejects(f.run('0.1.0'), /local tag is retained/);
  assert.equal(f.refs(f.remote), '');
  const before = f.git('rev-parse', tag);
  fs.unlinkSync(hook);
  succeeds(f.run('0.1.0'));
  assert.equal(f.git('rev-parse', tag), before);
  assert.equal(f.git('--git-dir', f.remote, 'rev-parse', tag), before);
});

test('preflight checks the push destination and refuses multiple push destinations', t => {
  const f = fixture(t), pushRemote = path.join(f.root, 'push.git');
  f.git('init', '--bare', pushRemote);
  f.git('remote', 'set-url', '--push', 'origin', pushRemote);
  f.git('tag', '-a', 'release-oss-v0.1.0', '-m', 'first');
  f.git('push', 'origin', `${tag}:${tag}`);
  f.git('tag', '-d', 'release-oss-v0.1.0');
  f.write('next.txt', 'next'); f.git('add', '.'); f.git('commit', '-m', 'next');
  rejects(f.run('0.1.0'), /Remote release tag points/);
  assert.equal(f.refs(f.remote), ''); assert.equal(f.git('tag', '--list'), '');
  f.git('remote', 'set-url', '--add', '--push', 'origin', f.remote);
  rejects(f.run('0.1.0'), /exactly one push URL/);
});
