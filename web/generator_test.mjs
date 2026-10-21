import assert from 'node:assert/strict';
import {execFileSync} from 'node:child_process';
import {makeConfig, makeCommand, validateManifest} from './generator.mjs';
export const model = {
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
    assert.match(command,/^curl .*install\.sh' \| sudo sh -s -- --base-url /);
    assert.doesNotMatch(command,/\n|sha256sum|tar -xzf| setup --config /);
    assert.ok(command.includes("--base-url 'https://downloads.example.com/rumable/0.1.0-test.1/'"));
    assert.equal(command, makeCommand({...m,arch:'arm64'},manifest,source));
    const encoded = Buffer.from(makeConfig(m)).toString('base64'); assert.ok(command.includes(encoded));
    const ps = makeCommand({...m,server:'iis'},manifest,source);
    assert.ok(ps.includes(Buffer.from(makeConfig({...m,server:'iis'})).toString('base64')));
    assert.match(ps,/install\.ps1/);
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
}
