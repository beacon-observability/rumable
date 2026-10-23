import assert from 'node:assert/strict';
import test from 'node:test';
import fs from 'node:fs';
import {resolveLocale, languageStorageKey, readLanguagePreference, saveLanguagePreference, createTranslator, messages, SetupError, errorMessage} from './i18n.mjs';
import {makeConfig, makeCommand, validateManifest} from './generator.mjs';

test('browser preference order selects Chinese or English with an English fallback', () => {
  for (const language of ['zh', 'zh-CN', 'zh-TW', 'zh-Hant-HK', 'ZH-sg']) {
    assert.equal(resolveLocale(null, [language, 'en-US']), 'zh-CN');
  }
  assert.equal(resolveLocale(null, ['en-GB', 'zh-CN']), 'en');
  assert.equal(resolveLocale(null, ['fr-FR', 'zh-CN', 'en']), 'zh-CN');
  assert.equal(resolveLocale(null, ['ja-JP', 'de-DE']), 'en');
  assert.equal(resolveLocale(null, []), 'en');
  assert.equal(resolveLocale(null, ['zhfake']), 'en');
  assert.equal(resolveLocale(), 'en');
  assert.equal(resolveLocale('en', ['zh-CN']), 'en');
  assert.equal(resolveLocale('zh-CN', ['en-US']), 'zh-CN');
  assert.equal(resolveLocale('invalid', ['zh-CN']), 'zh-CN');
});

test('manual language persists; missing, legacy or blocked storage uses browser detection', () => {
  const entries = new Map();
  const storage = {getItem:key => entries.get(key), setItem:(key,value) => entries.set(key,value), removeItem:key => entries.delete(key)};
  assert.equal(readLanguagePreference(storage), null);
  saveLanguagePreference(storage, 'en');
  assert.equal(readLanguagePreference(storage), 'en');
  saveLanguagePreference(storage, 'zh-CN');
  assert.equal(resolveLocale(readLanguagePreference(storage), ['en-US']), 'zh-CN');
  saveLanguagePreference(storage, null);
  assert.equal(readLanguagePreference(storage), null);
  assert.equal(entries.has(languageStorageKey), false);
  for (const saved of ['auto', 'unsupported']) {
    entries.set(languageStorageKey, saved);
    assert.equal(readLanguagePreference(storage), null);
    assert.equal(resolveLocale(readLanguagePreference(storage), ['zh-CN']), 'zh-CN');
    assert.equal(resolveLocale(readLanguagePreference(storage), ['fr-FR']), 'en');
  }
  const blocked = {getItem() { throw Error('blocked'); }, setItem() { throw Error('blocked'); }, removeItem() { throw Error('blocked'); }};
  assert.equal(readLanguagePreference(blocked), null);
  assert.doesNotThrow(() => saveLanguagePreference(blocked, 'en'));
  assert.doesNotThrow(() => saveLanguagePreference(blocked, null));
  assert.equal(readLanguagePreference(undefined), null);
  assert.doesNotThrow(() => saveLanguagePreference(undefined, 'en'));
});

test('all page bindings and generator errors have paired translations and matching placeholders', () => {
  const html = fs.readFileSync(new URL('./index.html', import.meta.url), 'utf8');
  const generator = fs.readFileSync(new URL('./generator.mjs', import.meta.url), 'utf8');
  const keys = [...html.matchAll(/data-i18n(?:-placeholder|-aria-label)?="([^"]+)"/g), ...generator.matchAll(/new SetupError\('([^']+)'/g)].map(m => m[1]);
  for (const key of keys) assert.ok(messages[key], `Missing translation: ${key}`);
  const placeholders = text => [...text.matchAll(/\{(\w+)\}/g)].map(m => m[1]).sort();
  for (const [key, [zh, en]] of Object.entries(messages)) {
    assert.ok(zh && en, key);
    assert.doesNotMatch(en, /[\u4e00-\u9fff]/u, key);
    assert.deepEqual(placeholders(en), placeholders(zh), key);
  }
});

test('generator errors can be retranslated without changing configuration or commands', () => {
  const zh = createTranslator('zh-CN'), en = createTranslator('en');
  const model = {server:'nginx', applicationId:'test-app', reporting:'openway', clientToken:'test-token', endpoint:'https://openway.example.com', defaultPrivacyLevel:'mask', sessionPersistence:'cookie', service:'我的商店', sdkSource:'cdn', sdkLoading:'sync'};
  const manifest = {schemaVersion:1, version:'1.2.3', artifactBase:'../1.2.3/', capabilities:['rum-setup-v1','rum-install-v1','rum-discovery-v1','rum-discovery-sdk-v1','rum-sdk-loading-v1'], sdk:{version:'3.3.21'}, packages:[], installers:{linux:{file:'install.sh',sha256:'a'.repeat(64)}, windows:{file:'install.ps1',sha256:'b'.repeat(64)}}};
  const url = 'https://downloads.example.com/rumable/latest/manifest.json';
  const config = makeConfig(model), command = makeCommand(model, manifest, url);
  for (const [change, key, english] of [
    [{applicationId:''}, 'error.token', /applicationId is required/],
    [{sdkURL:'http://cdn.example.com/rum.js'}, 'error.urlHTTPS', /CDN URL must use HTTPS/],
    [{sessionSampleRate:101}, 'error.rate', /sessionSampleRate must be between/],
    [{workerUrl:'https://['}, 'error.workerURL', /Worker URL/],
  ]) {
    let error;
    try { makeConfig({...model,...change}); } catch (caught) { error = caught; }
    assert.ok(error instanceof SetupError);
    assert.equal(error.key, key);
    assert.equal(error.message, errorMessage(error, zh));
    assert.match(errorMessage(error, en), english);
    assert.doesNotMatch(errorMessage(error, en), /[\u4e00-\u9fff]/u);
  }
  assert.throws(() => makeCommand(model, {...manifest,capabilities:manifest.capabilities.filter(c => c !== 'rum-sdk-loading-v1')}, url), error => {
    assert.match(errorMessage(error, en), /does not support synchronous loading/);
    return true;
  });
  assert.throws(() => validateManifest(null, url), error => error.key === 'error.manifestUnsupported');
  assert.equal(makeConfig(model), config);
  assert.equal(makeCommand(model, manifest, url), command);
  assert.ok(config.includes('我的商店'));
  assert.equal(errorMessage(Error('browser-specific detail'), en, 'error.network'), 'The network request failed or was blocked by the browser');
});
