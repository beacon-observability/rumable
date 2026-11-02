import assert from 'node:assert/strict';
import test from 'node:test';
import fs from 'node:fs';
import {createTranslator, messages, SetupError, errorMessage, translatePage, locales, resolveLocale, readLanguagePreference, saveLanguagePreference, languageStorageKey, catalogs} from './i18n.mjs';
import {makeConfig, makeCommand, validateManifest} from './generator.mjs';

test('setup HTML and catalog provide complete English copy', () => {
  const html = fs.readFileSync(new URL('./index.html', import.meta.url), 'utf8');
  const generator = fs.readFileSync(new URL('./generator.mjs', import.meta.url), 'utf8');
  assert.match(html, /<html lang="en">/);
  assert.match(html, /for="language-select"/);
  assert.match(html, /<select id="language-select">/);
  const keys = [...html.matchAll(/data-i18n(?:-placeholder|-aria-label)?="([^"]+)"/g), ...generator.matchAll(/new SetupError\('([^']+)'/g)].map(m => m[1]);
  for (const key of keys) assert.ok(messages[key], 'Missing English copy: ' + key);
  for (const [key, text] of Object.entries(messages)) {
    assert.equal(typeof text, 'string', key);
    assert.ok(text.length, key);
    assert.doesNotMatch(text, /[\u4e00-\u9fff]/u, key);
  }
});

test('English copy fills values and nested field labels', () => {
  const t = createTranslator();
  assert.equal(t('app.endpointHelp', {field:'site'}), 'Maps to site; enter the origin only');
  assert.equal(t('error.urlHTTPS', {label:{key:'field.sdkURL'}}), 'CDN URL must use HTTPS');
  assert.equal(t('release.loaded', {version:'0.1.8', sdk:'3.3.21'}), 'Rumable 0.1.8 · Web SDK 3.3.21');
});

test('page bindings update text, placeholders, and accessible labels', () => {
  const node = key => ({dataset:{i18n:key}, attributes:{}, getAttribute:() => key, setAttribute(name,value) { this.attributes[name] = value; }});
  const text = node('page.title'), placeholder = node('placeholder.applicationId'), aria = node('output.accessibleName');
  const root = {querySelectorAll(selector) {
    return selector === '[data-i18n]' ? [text] : selector === '[data-i18n-placeholder]' ? [placeholder] : [aria];
  }};
  translatePage(root, createTranslator());
  assert.equal(text.textContent, messages['page.title']);
  assert.equal(placeholder.attributes.placeholder, messages['placeholder.applicationId']);
  assert.equal(aria.attributes['aria-label'], messages['output.accessibleName']);
});

test('English errors preserve configuration, commands, and operator Unicode values', () => {
  const t = createTranslator();
  const model = {server:'nginx', applicationId:'test-app', reporting:'openway', clientToken:'test-token', endpoint:'https://intake.example.com', defaultPrivacyLevel:'mask', sessionPersistence:'cookie', service:'我的商店', sdkSource:'cdn', sdkLoading:'sync'};
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
    assert.equal(error.message, errorMessage(error, t));
    assert.match(errorMessage(error, t), english);
  }
  assert.throws(() => makeCommand(model, {...manifest,capabilities:manifest.capabilities.filter(c => c !== 'rum-sdk-loading-v1')}, url), error => {
    assert.match(errorMessage(error, t), /does not support synchronous loading/);
    return true;
  });
  assert.throws(() => validateManifest(null, url), error => error.key === 'error.manifestUnsupported');
  assert.equal(makeConfig(model), config);
  assert.equal(makeCommand(model, manifest, url), command);
  assert.ok(config.includes('我的商店'));
  assert.doesNotMatch(config, /^url =/m);
  assert.equal(errorMessage(Error('browser-specific detail'), t, 'error.network'), 'The network request failed or was blocked by the browser');
});


test('every supported locale has complete copy and matching interpolation parameters', () => {
  assert.deepEqual(Object.keys(locales), ['en', 'zh-CN', 'zh-TW', 'id', 'ja', 'ko']);
  const parameters = text => [...text.matchAll(/\{(\w+)\}/g)].map(m => m[1]).sort();
  for (const [locale, catalog] of Object.entries(catalogs)) {
    assert.deepEqual(Object.keys(catalog).sort(), Object.keys(messages).sort(), locale);
    for (const [key, text] of Object.entries(catalog)) {
      assert.equal(typeof text, 'string', `${locale}: ${key}`);
      assert.ok(text.trim(), `${locale}: ${key}`);
      assert.deepEqual(parameters(text), parameters(messages[key]), `${locale}: ${key}`);
      assert.equal(createTranslator(locale)(key), text, `${locale}: ${key}`);
    }
  }
});

test('locale negotiation follows explicit preference, browser order, scripts and regions', () => {
  for (const [preference, languages, expected] of [
    ['id', ['ja'], 'id'], ['en', ['zh-CN'], 'en'],
    [null, ['fr-FR', 'ja-JP', 'ko-KR'], 'ja'],
    [null, ['ko-KR'], 'ko'], [null, ['id-ID'], 'id'],
    [null, ['zh'], 'zh-CN'], [null, ['ZH-cn'], 'zh-CN'],
    [null, ['zh-SG'], 'zh-CN'], [null, ['zh-TW'], 'zh-TW'],
    [null, ['zh-HK'], 'zh-TW'], [null, ['zh-MO'], 'zh-TW'],
    [null, ['zh-Hant-CN'], 'zh-TW'], [null, ['zh-Hans-HK'], 'zh-CN'],
    ['invalid', ['en-GB', 'id'], 'en'], [null, ['fr', null], 'en'],
  ]) assert.equal(resolveLocale(preference, languages), expected);
});

test('explicit preferences survive reload and storage restrictions never break switching', () => {
  const values = new Map();
  const storage = () => ({getItem: key => values.get(key), setItem: (key, value) => values.set(key, value)});
  for (const locale of Object.keys(locales)) {
    saveLanguagePreference(storage, locale);
    assert.equal(values.get(languageStorageKey), locale);
    assert.equal(resolveLocale(readLanguagePreference(storage), ['fr']), locale);
  }
  values.set(languageStorageKey, 'invalid');
  assert.equal(readLanguagePreference(storage), null);
  for (const blocked of [() => { throw Error('blocked getter'); }, () => ({
    getItem() { throw Error('blocked read'); }, setItem() { throw Error('quota'); },
  }), () => null]) {
    assert.equal(readLanguagePreference(blocked), null);
    assert.doesNotThrow(() => saveLanguagePreference(blocked, 'ja'));
  }
});

test('translations fall back to English and localize nested errors', () => {
  assert.equal(createTranslator('unsupported')('page.title'), messages['page.title']);
  assert.equal(createTranslator('ja')('unknown.key'), 'unknown.key');
  const original = catalogs.ja['error.network'];
  try {
    delete catalogs.ja['error.network'];
    assert.equal(createTranslator('ja')('error.network'), messages['error.network']);
  } finally { catalogs.ja['error.network'] = original; }
  for (const locale of Object.keys(locales)) {
    const t = createTranslator(locale);
    const error = new SetupError('error.urlHTTPS', {label:{key:'field.sdkURL'}});
    assert.ok(errorMessage(error, t).includes(t('field.sdkURL')));
    assert.ok(!errorMessage(error, t).includes('{label}'));
    assert.equal(errorMessage(Error('network detail'), t, 'error.network'), t('error.network'));
    assert.equal(t('release.loaded', {version:'1.2.3', sdk:'3.3.21'}), 'Rumable 1.2.3 · Web SDK 3.3.21');
  }
});
