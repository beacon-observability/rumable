import {makeConfig, makeCommand, validateManifest} from './generator.mjs';
import {resolveLocale, readLanguagePreference, saveLanguagePreference, createTranslator, SetupError, errorMessage, translatePage} from './i18n.mjs';
const form = document.querySelector('#config-form');
const $ = id => document.getElementById(id);
let manifest = null, source = '', tab = 'config', config = '', command = '', generation = 0;
let storage;
try { storage = window.localStorage; } catch { /* Storage is optional. */ }
let locale = resolveLocale(readLanguagePreference(storage), [...(navigator.languages || []), navigator.language]), t;
let releaseKey = 'release.initial', releaseValues = {}, releaseError = null, actionKey = '';
function updateLanguage() {
  t = createTranslator(locale);
  document.documentElement.lang = locale;
  translatePage(document, t);
  $('language-select').value = locale;
  render();
}
$('language-select').addEventListener('change', event => {
  locale = event.target.value;
  saveLanguagePreference(storage, locale);
  updateLanguage();
});
function model() {
  const m = Object.fromEntries(new FormData(form));
  for (const input of form.querySelectorAll('input[type=checkbox]')) m[input.name] = input.checked;
  return m;
}
function render() {
  const fallback = form.elements.allowFallbackToLocalStorage;
  fallback.disabled = form.elements.sessionPersistence.value !== 'auto';
  if (fallback.disabled) fallback.checked = false;
  const cdn = form.elements.sdkSource.value === 'cdn';
  $('cdn-fields').hidden = !cdn;
  form.elements.sdkURL.disabled = !cdn;
  form.elements.sdkIntegrity.disabled = !cdn;
  const m = model();
  $('sdk-summary').textContent = cdn ? 'CDN' : t('sdk.bundled');
  const sync = m.sdkLoading === 'sync';
  $('loading-summary').textContent = t(sync ? 'sdk.sync' : 'sdk.async');
  $('sdk-loading-help').textContent = t(sync ? 'sdk.syncHelp' : 'sdk.asyncHelp');
  $('sdk-source-help').textContent = t(cdn ? 'sdk.cdnHelp' : 'sdk.bundledHelp');
  $('sdk-update-help').textContent = t(cdn ? 'sdk.cdnUpdate' : 'sdk.bundledUpdate');
  $('token-field').hidden = m.reporting !== 'openway';
  form.elements.clientToken.required = m.reporting === 'openway';
  $('endpoint-help').textContent = t('app.endpointHelp', {field:m.reporting === 'openway' ? 'site' : 'datakitOrigin'});
  $('replay-fields').hidden = !m.sessionReplayEnabled;
  $('nginx-fields').hidden = m.server !== 'nginx';
  $('iis-fields').hidden = m.server !== 'iis';
  $('iis-options').hidden = m.server !== 'iis';
  $('windows-help').hidden = m.server !== 'iis';
  $('manual-fields').hidden = !m.manualTarget;
  $('discovery-help').hidden = m.manualTarget;
  form.elements.publicURL.required = m.manualTarget;
  form.elements.id.required = m.manualTarget;
  $('arch-field').hidden = m.server === 'iis';
  $('sample-summary').textContent = `${m.sessionSampleRate}%`;
  $('replay-summary').textContent = m.sessionReplayEnabled ? `${m.sessionReplaySampleRate}%` : t('output.off');
  config = ''; command = '';
  try {
    config = makeConfig(m);
    if (manifest) command = makeCommand(m, manifest, source);
    $('form-error').textContent = '';
  } catch (error) { $('form-error').textContent = errorMessage(error, t); }
  $('output-text').textContent = tab === 'config' ? (config || t('output.emptyConfig')) : (command || t('output.emptyCommand'));
  $('release-status').textContent = t(releaseKey, releaseError ? {reason:errorMessage(releaseError, t, 'error.network')} : releaseValues);
  $('action-status').textContent = actionKey ? t(actionKey) : '';
  $('copy-command').disabled = !command;
  $('download-config').disabled = !config;
  $('tab-config').classList.toggle('selected', tab === 'config');
  $('tab-command').classList.toggle('selected', tab === 'command');
}
form.addEventListener('input', () => { actionKey = ''; render(); });
form.addEventListener('change', render);
form.addEventListener('submit', e => e.preventDefault());
$('tab-config').onclick = () => { tab = 'config'; render(); };
$('tab-command').onclick = () => { tab = 'command'; render(); };
$('manifest-url').oninput = () => {
  generation++; manifest = null; source = '';
  releaseKey = 'release.changed'; releaseError = null; render();
};
$('load-release').onclick = async () => {
  const id = ++generation, raw = $('manifest-url').value.trim();
  manifest = null; source = ''; releaseKey = 'release.loading'; releaseError = null; render();
  try {
    let parsed;
    try { parsed = new URL(raw); } catch { throw new SetupError('error.manifestURL'); }
    if (parsed.protocol !== 'https:' || parsed.username || parsed.password || parsed.hash || parsed.search) throw new SetupError('error.manifestURL');
    const response = await fetch(raw, {credentials:'omit', redirect:'error', cache:'no-store', signal:AbortSignal.timeout(15000)});
    if (!response.ok) throw new SetupError('error.http', {status:response.status});
    const bytes = await response.text();
    if (bytes.length > 128 * 1024) throw new SetupError('error.manifestSize');
    let data;
    try { data = JSON.parse(bytes); } catch { throw new SetupError('error.manifestJSON'); }
    const candidate = validateManifest(data, raw);
    if (id !== generation) return;
    manifest = candidate; source = raw;
    releaseKey = 'release.loaded'; releaseValues = {version:candidate.version, sdk:candidate.sdk.version};
  } catch (error) {
    if (id !== generation) return;
    releaseKey = 'release.failed';
    releaseError = error.name === 'TimeoutError' ? new SetupError('error.timeout') : error;
  }
  render();
};
$('copy-command').onclick = async () => {
  if (!command) return;
  try { await navigator.clipboard.writeText(command); actionKey = 'action.copied'; }
  catch { tab = 'command'; actionKey = 'action.copyFailed'; }
  render();
};
$('download-config').onclick = () => {
  if (!config) return;
  const link = document.createElement('a');
  link.href = URL.createObjectURL(new Blob([config], {type:'text/plain;charset=utf-8'}));
  link.download = 'rumable.toml'; link.click(); setTimeout(() => URL.revokeObjectURL(link.href), 1000);
};
const initial = new URLSearchParams(location.search).get('manifest');
if (initial) $('manifest-url').value = initial;
else if (location.protocol === 'https:' && /\/(?:latest|\d+\.\d+\.\d+(?:-[a-z0-9.]+)?)\/setup\/(?:index\.html)?$/.test(location.pathname)) {
  $('manifest-url').value = new URL('../manifest.json', location.href).href;
}
updateLanguage();
if ($('manifest-url').value) $('load-release').click();
