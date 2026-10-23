import assert from 'node:assert/strict';
import {execFileSync, spawnSync} from 'node:child_process';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import {makeConfig, makeCommand, validateManifest, officialSDKURL} from './generator.mjs';
export const model = {
  manualTarget:true,
  id:'website', applicationId:'app-test', reporting:'openway', endpoint:'https://openway.example.com', clientToken:'browser-only-token',
  service:'网站 "quoted" \\ test', env:'production', version:'1.0.0', sessionSampleRate:'35.5', sessionOnErrorSampleRate:'0',
  sessionReplayEnabled:false, sessionReplaySampleRate:'20', sessionReplayOnErrorSampleRate:'10', trackUserInteractions:true,
  defaultPrivacyLevel:'mask', sessionPersistence:'cookie', publicURL:'https://www.example.com/', server:'nginx', arch:'amd64',
  nginxBinary:'/usr/sbin/nginx', nginxConfig:'/etc/nginx/nginx.conf', includePaths:'/', excludePaths:'/api/', probePaths:'/',
  allowedTracingUrls:'https://api.example.com/', traceType:'w3c_traceparent', tracingSampleRate:'50',
  iisSite:'Default Web Site', iisApplication:'/', allowCompressionChanges:true,
};
export const manifest = {schemaVersion:1, version:'0.1.0-test.1', capabilities:['rum-setup-v1','rum-install-v1'], installers:{linux:{file:'install.sh',sha256:'c'.repeat(64)},windows:{file:'install.ps1',sha256:'d'.repeat(64)}}, sdk:{version:'3.3.21'}, packages:[
  ...['amd64','arm64'].map(arch => ({os:'linux', arch, flavor:'modern', file:`rumable_0.1.0-test.1_linux_${arch}.tar.gz`, sha256:'a'.repeat(64)})),
  {os:'windows', arch:'amd64', flavor:'modern', file:'rumable_0.1.0-test.1_windows_amd64.zip', sha256:'b'.repeat(64)},
]};
const source = 'https://downloads.example.com/rumable/0.1.0-test.1/manifest.json';
const fixtures = [model, {...model, reporting:'datakit', sessionReplayEnabled:true, compressIntakeRequests:true, workerUrl:'/worker.js'}, {...model, server:'iis', publicURL:'https://www.example.com/app/', iisApplication:'/app', includePaths:'/app/', probePaths:'/app/'}];
fixtures.push(...['nginx','iis'].flatMap(server => [
  {...model, server, sdkSource:'cdn', sdkURL:officialSDKURL},
  {...model, server, sdkSource:'cdn', sdkURL:'https://cdn.example.com/rum/3.3.21.js', sdkIntegrity:true},
  {...model, server, sdkSource:'bundled', sdkLoading:'sync'},
  {...model, server, sdkSource:'cdn', sdkURL:officialSDKURL, sdkLoading:'sync'},
  {...model, server, sdkSource:'cdn', sdkURL:'https://cdn.example.com/rum/3.3.21.js', sdkLoading:'sync', sdkIntegrity:true},
]));
if (process.argv.includes('--fixtures')) {
  process.stdout.write(JSON.stringify(fixtures.map(makeConfig)));
} else {
  const {test} = await import('node:test');
  test('reporting modes and replay switch serialize SDK values', () => {
    const open = makeConfig(model), kit = makeConfig(fixtures[1]);
    assert.match(open, /sessionReplaySampleRate = 0/); assert.match(open, /sessionReplayOnErrorSampleRate = 0/);
    assert.match(kit, /datakitOrigin =/); assert.doesNotMatch(kit, /clientToken =|^site =/m);
    assert.match(kit, /sessionReplaySampleRate = 20/); assert.match(kit, /sessionSampleRate = 35.5/);
    const fallback = makeConfig({...model,sessionPersistence:'auto',allowFallbackToLocalStorage:true});
    assert.doesNotMatch(fallback, /sessionPersistence =/); assert.match(fallback, /allowFallbackToLocalStorage = true/);
    assert.throws(() => makeConfig({...model,allowFallbackToLocalStorage:true}));
  });
  test('reject invalid SDK, server and URL combinations', () => {
    for (const patch of [{sessionSampleRate:'101'},{sessionSampleRate:'NaN'},{clientToken:'a&b'},{applicationId:''},{endpoint:'https://example.com/path'},{endpoint:'https://example.com?'},{endpoint:'http://example.com'},{defaultPrivacyLevel:'unknown'},{probePaths:'/api/'},{workerUrl:'//www.example.com/x'},{workerUrl:'https://other.example/x'},{publicURL:'https://example.com/?'},{server:'iis',iisApplication:'/app'}]) assert.throws(() => makeConfig({...model,...patch}), JSON.stringify(patch));
  });
  test('SDK source round trips through config and commands in both setup modes', () => {
    const current = {...manifest, capabilities:[...manifest.capabilities,'rum-discovery-v1','rum-discovery-sdk-v1']};
    for (const server of ['nginx','iis']) for (const manualTarget of [false,true]) {
      const m = {...model, server, manualTarget, sdkSource:'cdn', sdkURL:'https://cdn.example.com/pinned.js', sdkIntegrity:true};
      const config = makeConfig(m);
      assert.ok(config.includes(manualTarget ? '[sites.sdk]' : '[sdk]'));
      assert.match(config, /source = "cdn"\nurl = "https:\/\/cdn.example.com\/pinned.js"\nintegrity = true/);
      assert.ok(makeCommand(m,current,source).includes(Buffer.from(config).toString('base64')));
      const bundled = makeConfig({...m,sdkSource:'bundled'});
      assert.doesNotMatch(bundled, /\[(?:sites\.)?sdk\]|pinned.js|integrity/);
      assert.equal(bundled,makeConfig({...model,server,manualTarget}));
      const official = makeConfig({...m,sdkURL:'',sdkIntegrity:false});
      assert.doesNotMatch(official, /^url =/m);
      assert.match(official, /integrity = false/);
    }
  });
  test('CDN discovery refuses incompatible packages and malformed URLs', () => {
    const old = {...manifest,capabilities:[...manifest.capabilities,'rum-discovery-v1']};
    const m = {...model,manualTarget:false,sdkSource:'cdn'};
    for (const server of ['nginx','iis']) {
      assert.throws(() => makeCommand({...m,server},old,source), /cannot configure CDN or loading mode during discovery/);
      assert.doesNotThrow(() => makeCommand({...m,server,manualTarget:true},old,source));
      assert.doesNotThrow(() => makeCommand({...m,server,sdkSource:'bundled'},old,source));
    }
    for (const sdkURL of ['http://cdn.example.com/sdk.js','https://cdn.example.com/','https://cdn.example.com/sdk.js?','https://cdn.example.com/sdk.js?v=1','https://cdn.example.com/sdk.js#v','https://user:pass@cdn.example.com/sdk.js','//cdn.example.com/sdk.js']) {
      assert.throws(() => makeConfig({...m,sdkURL}), sdkURL);
    }
    assert.throws(() => makeConfig({...m,sdkSource:'unknown'}));
  });
  test('synchronous loading is explicit and requires matching release capabilities', () => {
    const current = {...manifest, capabilities:[...manifest.capabilities,'rum-discovery-v1','rum-discovery-sdk-v1','rum-sdk-loading-v1']};
    for (const server of ['nginx','iis']) for (const manualTarget of [false,true]) for (const sdkSource of ['bundled','cdn']) {
      const m = {...model,server,manualTarget,sdkSource,sdkLoading:'sync'};
      const config = makeConfig(m);
      assert.match(config, /loading = "sync"/);
      assert.ok(config.includes(manualTarget ? '[sites.sdk]' : '[sdk]'));
      assert.ok(makeCommand(m,current,source).includes(Buffer.from(config).toString('base64')));
      assert.throws(() => makeCommand(m,{...current,capabilities:current.capabilities.filter(c => c !== 'rum-sdk-loading-v1')},source), /does not support synchronous loading/);
      if (!manualTarget) assert.throws(() => makeCommand(m,{...current,capabilities:current.capabilities.filter(c => c !== 'rum-discovery-sdk-v1')},source), /cannot configure CDN or loading mode during discovery/);
      assert.doesNotMatch(makeConfig({...m,sdkLoading:'async'}), /loading =/);
    }
    assert.throws(() => makeConfig({...model,sdkLoading:'defer'}), /asynchronous or synchronous/);
  });
  test('manifest rejects changed capabilities, paths, hashes and duplicate platforms', () => {
    validateManifest(manifest, source);
    for (const patch of [{artifactBase:'https://other.example/'},{artifactBase:'../9.9.9/'},{capabilities:[]},{installers:{}},{installers:{...manifest.installers,linux:{file:'../install.sh',sha256:'c'.repeat(64)}}},{version:'../../x'},{packages:[{...manifest.packages[0],file:'../../rumable'}]},{packages:[{...manifest.packages[0],sha256:'bad'}]},{packages:[manifest.packages[0],manifest.packages[0]]}]) assert.throws(() => validateManifest({...manifest,...patch},source));
    assert.throws(() => validateManifest(manifest,'http://example.com/manifest.json'));
  });
  test('one-line command delegates installation and keeps SDK configuration as data', () => {
    const m = {...model,service:"中文'; $(touch /tmp/rumable-must-not-exist); `false`"};
    const command = makeCommand(m,manifest,source);
    execFileSync('sh',['-n'],{input:command});
    assert.doesNotMatch(command,/touch \/tmp/);
    assert.match(command,/^bash -o pipefail -c /);
    assert.doesNotMatch(command,/\n|sha256sum|tar -xzf| setup --config /);
    assert.ok(command.includes('https://downloads.example.com/rumable/0.1.0-test.1/'));
    assert.equal(command, makeCommand({...m,arch:'arm64'},manifest,source));
    const encoded = Buffer.from(makeConfig(m)).toString('base64'); assert.ok(command.includes(encoded));
    const ps = makeCommand({...m,server:'iis'},manifest,source);
    assert.ok(ps.includes(Buffer.from(makeConfig({...m,server:'iis'})).toString('base64')));
    assert.match(ps,/SecurityProtocol.*Tls12;.*Invoke-WebRequest.*install\.ps1/);
    assert.doesNotMatch(ps,/\n|Get-FileHash|ExtractToFile|LASTEXITCODE/);
  });
  test('latest discovery produces immutable commands for both operating systems', () => {
    const latestURL = 'https://downloads.example.com/rumable/latest/manifest.json';
    assert.throws(() => validateManifest(manifest, latestURL), /latest/);
    const latest = {...manifest,artifactBase:`../${manifest.version}/`};
    for (const server of ['nginx','iis']) {
      assert.equal(makeCommand({...model,server},latest,latestURL), makeCommand({...model,server},manifest,source));
      assert.doesNotMatch(makeCommand({...model,server},latest,latestURL), /\/latest\//);
    }
  });
  test('automatic setup needs only RUM fields and an environment; old releases fail closed', () => {
    for (const server of ['nginx','iis']) {
      const m = {...model, manualTarget:false, server, id:'', publicURL:'', nginxBinary:'', iisSite:'', includePaths:'', probePaths:''};
      const config = makeConfig(m);
      assert.match(config, /\[rum\]/);
      assert.match(config, new RegExp(`server = "${server}"`));
      assert.doesNotMatch(config, /\[\[sites\]\]|publicURL|probePaths|nginxBinary|website/);
      assert.throws(() => makeCommand(m,manifest,source), /site discovery/);
      const current = {...manifest, capabilities:[...manifest.capabilities,'rum-discovery-v1']};
      const command = makeCommand(m,current,source);
      assert.match(command, server === 'iis' ? / -Discover$/ : / --discover'$/);
      assert.ok(command.includes(Buffer.from(config).toString('base64')));
      if (server === 'nginx') execFileSync('sh',['-n'],{input:command});
      // Host-dependent checks happen after site discovery in the CLI.
      assert.doesNotThrow(() => makeConfig({...m, workerUrl:'https://www.example.com/worker.js',useSecureSessionCookie:true}));
      assert.throws(() => makeConfig({...m,workerUrl:'//evil.example/worker.js'}));
    }
  });
  test('generated Linux command propagates curl and installer failure even from a POSIX shell', t => {
    const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'rumable-command-'));
    t.after(() => fs.rmSync(dir, {recursive:true, force:true}));
    fs.writeFileSync(path.join(dir,'sudo'), '#!/bin/sh\nexec "$@"\n', {mode:0o755});
    const cmd = makeCommand(model,manifest,source);
    for (const [body,status] of [['exit 22',22], ['exit 60',60], ['printf "exit 17\\n"',17], ['printf "exit 0\\n"',0]]) {
      fs.writeFileSync(path.join(dir,'curl'), '#!/bin/sh\n'+body+'\n',{mode:0o755});
      const result=spawnSync('sh',['-c',cmd],{env:{...process.env,PATH:dir+path.delimiter+process.env.PATH},encoding:'utf8'});
      assert.equal(result.status,status,result.stderr);
    }
  });
}
