import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import {spawnSync} from 'node:child_process';
import {fileURLToPath} from 'node:url';

test('installer detects x86/x64/ARM64 and rejects invalid input before download', t => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'rumable-installer-test-'));
  t.after(() => fs.rmSync(dir, {recursive:true, force:true}));
  const bin = path.join(dir, 'bin'); fs.mkdirSync(bin);
  const command = (name, body) => fs.writeFileSync(path.join(bin, name), '#!/bin/sh\n' + body, {mode:0o755});
  command('id', 'echo "${FIXTURE_UID:-0}"\n');
  command('uname', 'if [ "$1" = -s ]; then echo Linux; else echo "$FIXTURE_ARCH"; fi\n');
  // Stop at the download boundary: no binary is installed on the host.
  command('curl', 'printf "%s\\n" "$@" > "$FIXTURE_DOWNLOAD"\nexit 77\n');
  const template = fs.readFileSync(fileURLToPath(new URL('./install.sh', import.meta.url)), 'utf8');
  const script = path.join(dir, 'install.sh');
  fs.writeFileSync(script, template.replaceAll('@@VERSION@@','0.1.0')
    .replaceAll('@@LINUX_386_SHA256@@','a'.repeat(64))
    .replaceAll('@@LINUX_AMD64_SHA256@@','b'.repeat(64))
    .replaceAll('@@LINUX_ARM64_SHA256@@','c'.repeat(64)));
  const download = path.join(dir, 'download.txt');
  const config = Buffer.from('schemaVersion = 1\n').toString('base64');
  const args = ['--base-url', 'https://downloads.example.com/rumable/0.1.0/', '--config', config];
  const execute = (arch, extraEnv = {}, values = args) => {
    fs.rmSync(download, {force:true});
    return spawnSync('sh', [script, ...values], {encoding:'utf8', env:{...process.env, PATH:bin + path.delimiter + process.env.PATH,
      FIXTURE_ARCH:arch, FIXTURE_DOWNLOAD:download, ...extraEnv}});
  };
  for (const [machine, arch] of [['i386','386'], ['i686','386'], ['x86_64','amd64'], ['aarch64','arm64'], ['arm64','arm64']]) {
    const result = execute(machine);
    assert.equal(result.status, 77, result.stderr);
    assert(fs.readFileSync(download, 'utf8').includes(`/rumable_0.1.0_linux_${arch}.tar.gz`));
  }
  for (const base of ['https://downloads.example.com/rumable/latest','https://downloads.example.com/rumable/latest/']) {
    const result = execute('x86_64', {}, ['--base-url',base,'--config',config]);
    assert.equal(result.status, 77, result.stderr);
    const request = fs.readFileSync(download, 'utf8');
    assert(request.includes('https://downloads.example.com/rumable/0.1.0/rumable_0.1.0_linux_amd64.tar.gz'));
    assert(!request.includes('/latest/'));
  }
  for (const base of ['https://github.com/beacon-observability/rumable/releases/latest/download', 'https://github.com/beacon-observability/rumable/releases/latest/download/']) {
    const result = execute('x86_64', {}, ['--base-url',base,'--config',config]);
    assert.equal(result.status,77,result.stderr);
    assert(fs.readFileSync(download,'utf8').includes('https://github.com/beacon-observability/rumable/releases/download/v0.1.0/rumable_0.1.0_linux_amd64.tar.gz'));
  }
  for (const [machine, overrides, values] of [
    ['riscv64', {}, args], ['x86_64', {FIXTURE_UID:'1000'}, args],
    ['x86_64', {}, ['--base-url','http://downloads.example.com','--config',config]],
    ['x86_64', {}, ['--base-url','https://user:password@example.com','--config',config]],
    ['x86_64', {}, ['--base-url','https://downloads.example.com','--config','!invalid!']],
    ['x86_64', {}, ['--base-url']]
  ]) {
    const result = execute(machine, overrides, values);
    assert.notEqual(result.status, 0);
    assert.notEqual(result.status, 77);
    assert(!fs.existsSync(download), 'invalid input reached package download');
  }
});
