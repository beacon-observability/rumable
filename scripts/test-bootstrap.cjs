// Protect the browser boundary without adding a browser test framework.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
const source = fs.readFileSync(process.argv[2], 'utf8');
const replayRate = Number(process.argv[3] || 0);
const expected = Object.assign({
  sessionReplayOnErrorSampleRate: 0,
  defaultPrivacyLevel: 'mask',
  compressIntakeRequests: false,
  trackUserInteractions: true,
  allowedTracingUrls: []
}, process.argv[4] ? JSON.parse(fs.readFileSync(process.argv[4], 'utf8')) : {});
const startsReplay = expected._startsReplay ?? (replayRate > 0 || expected.sessionReplayOnErrorSampleRate > 0);
const sdk = expected._sdk;
delete expected._startsReplay;
delete expected._sdk;
function run(existing, fail) {
  let loads = 0, inits = 0, starts = 0, configuration;
  const calls = [];
  const w = existing ? { DATAFLUX_RUM: existing } : {};
  const d = { createElement() { return {}; }, head: { appendChild(script) {
    loads++;
    if (sdk) for (const [key,value] of Object.entries(sdk)) assert.equal(script[key],value,key);
    if (fail === 'load') { script.onerror(); return; }
    w.DATAFLUX_RUM = {
      init(config) { inits++; calls.push('init'); configuration = config; if (fail === 'init') throw Error('init failed'); },
      startSessionReplayRecording() { starts++; calls.push('replay'); if (fail === 'replay') throw Error('replay failed'); }
    };
    if (fail === 'missing-replay') delete w.DATAFLUX_RUM.startSessionReplayRecording;
    script.onload();
  } } };
  const context = vm.createContext({ window: w, document: d });
  vm.runInContext(source, context);
  vm.runInContext(source, context);
  return { w, loads, inits, starts, calls, configuration };
}
let r = run();
assert.equal(r.loads, 1); assert.equal(r.inits, 1);
assert.equal(r.configuration.sessionReplaySampleRate, replayRate);
assert.equal(r.starts, startsReplay ? 1 : 0);
assert.deepEqual(r.calls, startsReplay ? ['init', 'replay'] : ['init']);
assert.equal(r.w.__RUMABLE__.status, 'initialized');
const actual = JSON.parse(JSON.stringify(r.configuration));
for (const [key, value] of Object.entries(expected)) assert.deepEqual(actual[key], value, key);
r = run({ init() { throw Error('must not take over'); } });
assert.equal(r.loads, 0); assert.equal(r.inits, 0);
r = run(null, 'load');
assert.equal(r.w.__RUMABLE__.status, 'load-error'); assert.equal(r.inits, 0);
r = run(null, 'init');
assert.equal(r.w.__RUMABLE__.status, 'init-error'); assert.equal(r.starts, 0);
for (const fail of ['replay', 'missing-replay']) {
  r = run(null, fail);
  assert.equal(r.w.__RUMABLE__.status, 'initialized');
  assert.equal(r.w.__RUMABLE__.replayStatus, startsReplay ? 'start-error' : undefined);
}
console.log(`bootstrap: replay rate ${replayRate}, single initialization, existing SDK and failure isolation passed`);
