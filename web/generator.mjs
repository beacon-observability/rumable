import {SetupError} from './i18n.mjs';

// These fields map to internal/rumable/rum.go and the bundled Web SDK's init.
export const traceTypes = ['ddtrace', 'zipkin', 'zipkin_single_header', 'w3c_traceparent', 'w3c_traceparent_64bit', 'skywalking_v3', 'jaeger'];
export const officialSDKURL = 'https://static.guance.com/browser-sdk/v3/dataflux-rum.js';
const boolFields = ['trackUserInteractions', 'enablePrivacyForActionName', 'enableLongAnimationFrame', 'compressIntakeRequests', 'remoteConfiguration', 'trackSessionAcrossSubdomains', 'useSecureSessionCookie', 'allowFallbackToLocalStorage'];
const rateFields = ['sessionSampleRate', 'sessionOnErrorSampleRate', 'sessionReplaySampleRate', 'sessionReplayOnErrorSampleRate', 'tracingSampleRate'];
const stringFields = ['env', 'service', 'version', 'actionNameAttribute', 'workerUrl'];
const encode = (v) => JSON.stringify(v).replace(/\u007f/g, '\\u007f');
const lines = (v) => String(v || '').split(/\r?\n/).map(x => x.trim()).filter(Boolean);
export const shellQuote = v => "'" + String(v).replaceAll("'", "'\"'\"'") + "'";
export const psQuote = v => "'" + String(v).replaceAll("'", "''") + "'";

function url(value, label, {origin = false, https = false} = {}) {
  let u;
  try { u = new URL(value); } catch { throw new SetupError('error.urlRequired', {label}); }
  if (!['http:', 'https:'].includes(u.protocol) || u.username || u.password || String(value).includes('#') || /[\s\\]/u.test(value)) throw new SetupError('error.urlInvalid', {label});
  if (https && u.protocol !== 'https:') throw new SetupError('error.urlHTTPS', {label});
  if (origin && (String(value).includes('?') || (u.pathname !== '' && u.pathname !== '/'))) throw new SetupError('error.urlOrigin', {label});
  return u;
}

export function makeConfig(m) {
  const manual = !!m.manualTarget;
  const sdkSource = m.sdkSource || 'bundled';
  if (!['bundled', 'cdn'].includes(sdkSource)) throw new SetupError('error.sdkSource');
  const sdkLoading = m.sdkLoading || 'async';
  if (!['async', 'sync'].includes(sdkLoading)) throw new SetupError('error.sdkLoading');
  let sdkText = '';
  if (sdkSource === 'cdn' || sdkLoading === 'sync') {
    sdkText = `\n[${manual ? 'sites.sdk' : 'sdk'}]\nsource = ${encode(sdkSource)}\n`;
  }
  if (sdkSource === 'cdn') {
    const raw = String(m.sdkURL || '').trim();
    if (raw) {
      const remote = url(raw, {key:'field.sdkURL'}, {https: true});
      if (raw.includes('?') || remote.pathname === '/') throw new SetupError('error.cdnPath');
      sdkText += `url = ${encode(raw)}\n`;
    }
    // An omitted URL preserves the CLI's release-recorded SDK source.
    sdkText += `integrity = ${!!m.sdkIntegrity}\n`;
  }
  if (sdkLoading === 'sync') sdkText += 'loading = "sync"\n';
  let base;
  if (manual) {
    if (!/^[a-z][a-z0-9-]{0,47}$/.test(m.id)) throw new SetupError('error.siteId');
    base = url(m.publicURL, {key:'field.publicURL'});
    if (m.publicURL.includes('?') || base.pathname.includes('%') || !base.pathname.endsWith('/')) throw new SetupError('error.siteURL');
  }
  if (!['nginx', 'iis'].includes(m.server)) throw new SetupError('error.server');
  for (const key of ['applicationId', ...(m.reporting === 'openway' ? ['clientToken'] : [])]) {
    if (!m[key] || /[&#+%;\s\p{Cc}]/u.test(m[key])) throw new SetupError('error.token', {field:key});
  }
  if (!['openway', 'datakit'].includes(m.reporting)) throw new SetupError('error.reporting');
  const endpointKey = m.reporting === 'openway' ? 'site' : 'datakitOrigin';
  const endpoint = url(m.endpoint, {key:'field.endpoint'}, {origin: true});
  if (base?.protocol === 'https:' && endpoint.protocol !== 'https:') throw new SetupError('error.endpointHTTPS');
  const rum = {applicationId: m.applicationId, [endpointKey]: endpoint.origin};
  if (m.reporting === 'openway') rum.clientToken = m.clientToken;
  for (const key of stringFields) if (m[key]) rum[key] = String(m[key]);
  for (const key of rateFields) {
    if (m[key] === undefined || m[key] === '') continue;
    const n = Number(m[key]);
    if (!Number.isFinite(n) || n < 0 || n > 100) throw new SetupError('error.rate', {field:key});
    rum[key] = n;
  }
  for (const key of boolFields) if (m[key] !== undefined) rum[key] = !!m[key];
  rum.sessionReplayEnabled = !!m.sessionReplayEnabled;
  if (!rum.sessionReplayEnabled) { rum.sessionReplaySampleRate = 0; rum.sessionReplayOnErrorSampleRate = 0; }
  if (!['mask', 'mask-user-input', 'allow'].includes(m.defaultPrivacyLevel)) throw new SetupError('error.privacy');
  rum.defaultPrivacyLevel = m.defaultPrivacyLevel;
  if (!['auto', 'cookie', 'local-storage'].includes(m.sessionPersistence)) throw new SetupError('error.persistence');
  if (m.sessionPersistence !== 'auto') rum.sessionPersistence = m.sessionPersistence;
  if (rum.allowFallbackToLocalStorage && m.sessionPersistence !== 'auto') throw new SetupError('error.fallback');
  if (rum.useSecureSessionCookie && base && base.protocol !== 'https:') throw new SetupError('error.secureCookie');
  if (rum.workerUrl) {
    let workerURL;
    try { workerURL = new URL(rum.workerUrl, base || 'https://discovery.invalid/').href; }
    catch { throw new SetupError('error.workerURL'); }
    const worker = url(workerURL, {key:'field.workerURL'});
    if (rum.workerUrl.startsWith('//') || (!rum.workerUrl.startsWith('/') && !/^https?:\/\//.test(rum.workerUrl)) || (base && worker.origin !== base.origin)) throw new SetupError('error.workerURL');
  }
  rum.allowedTracingUrls = lines(m.allowedTracingUrls);
  for (const raw of rum.allowedTracingUrls) url(raw, {key:'field.tracingURLs'});
  if (rum.allowedTracingUrls.length) {
    if (!traceTypes.includes(m.traceType)) throw new SetupError('error.traceType');
    rum.traceType = m.traceType;
  }
  const rumText = Object.entries(rum).map(([k,v]) => `${k} = ${encode(v)}`).join('\n');
  if (!manual) {
    return `schemaVersion = 1\nserver = ${encode(m.server)}\n${m.server === 'iis' ? `allowCompressionChanges = ${!!m.allowCompressionChanges}\n` : ''}\n[rum]\n${rumText}\n${sdkText}`;
  }
  const paths = key => {
    const values = lines(m[key]);
    for (const path of values) if (!path.startsWith('/') || path.startsWith('//') || /[?#%\s\\]/u.test(path)) throw new SetupError('error.path', {field:key});
    return values;
  };
  const include = paths('includePaths'), exclude = paths('excludePaths'), probes = paths('probePaths');
  if (!include.length || !probes.length) throw new SetupError('error.pathsRequired');
  for (const p of probes) if (!include.some(v => p.startsWith(v)) || exclude.some(v => p.startsWith(v)) || p.startsWith(base.pathname + '__rumable__/')) throw new SetupError('error.probePath');
  let target;
  if (m.server === 'nginx') {
    if (!m.nginxBinary?.startsWith('/') || /[\r\n\0]/.test(m.nginxBinary)) throw new SetupError('error.nginxBinary');
    target = `[sites.nginx]\nbinary = ${encode(m.nginxBinary)}\nattachmentReviewed = false`;
    if (m.nginxConfig) {
      if (!m.nginxConfig.startsWith('/')) throw new SetupError('error.nginxConfig');
      target += `\nargs = ["-c", ${encode(m.nginxConfig)}]`;
    }
  } else {
    if (!m.iisSite?.trim() || !m.iisApplication?.startsWith('/') || /[\\\r\n]/.test(m.iisSite + m.iisApplication)) throw new SetupError('error.iisTarget');
    if ((m.iisApplication === '/' ? '/' : m.iisApplication + '/') !== base.pathname) throw new SetupError('error.iisPath');
    target = `[sites.iis]\nsite = ${encode(m.iisSite)}\napplication = ${encode(m.iisApplication)}\nallowCompressionChanges = ${!!m.allowCompressionChanges}`;
  }
  return `schemaVersion = 1\n\n[[sites]]\nid = ${encode(m.id)}\npublicURL = ${encode(m.publicURL)}\nprobePaths = ${encode(probes)}\nincludePaths = ${encode(include)}\nexcludePaths = ${encode(exclude)}\n\n${target}\n\n[sites.rum]\n${rumText}\n${sdkText}`;
}

export function validateManifest(m, manifestURL) {
  const source = url(manifestURL, {key:'field.manifest'}, {https: true});
  if (source.search) throw new SetupError('error.manifestQuery');
  if (!m || m.schemaVersion !== 1 || !/^\d+\.\d+\.\d+(?:-[a-z0-9]+(?:\.[a-z0-9]+)*)?$/.test(m.version) || !m.capabilities?.includes('rum-setup-v1') || !m.capabilities?.includes('rum-install-v1') || !Array.isArray(m.packages) || !m.sdk?.version) throw new SetupError('error.manifestUnsupported');
  const githubBase = /^https:\/\/github\.com\/[A-Za-z0-9][A-Za-z0-9-]*\/[A-Za-z0-9][A-Za-z0-9_.-]*\/releases\/download\/v([^/]+)\/$/.exec(m.artifactBase || '');
  if ((m.artifactBase !== undefined && m.artifactBase !== `../${m.version}/` && githubBase?.[1] !== m.version) || (/\/latest\/manifest\.json$/.test(source.pathname) && !m.artifactBase)) throw new SetupError('error.manifestBase');
  for (const [platform, file] of [['linux', 'install.sh'], ['windows', 'install.ps1']]) {
    const installer = m.installers?.[platform];
    if (installer?.file !== file || !/^[a-f0-9]{64}$/.test(installer.sha256)) throw new SetupError('error.installers');
  }
  const seen = new Set();
  for (const p of m.packages) {
    const key = `${p.os}/${p.arch}/${p.flavor}`;
    if (seen.has(key) || !['linux', 'windows'].includes(p.os) || !['386', 'amd64', 'arm64'].includes(p.arch) || !['modern', 'legacy'].includes(p.flavor)) throw new SetupError('error.platforms');
    seen.add(key);
    const file = `rumable_${m.version}_${p.os}_${p.arch}${p.flavor === 'legacy' ? '_legacy' : ''}${p.os === 'windows' ? '.zip' : '.tar.gz'}`;
    if (p.file !== file || !/^[a-f0-9]{64}$/.test(p.sha256)) throw new SetupError('error.package');
  }
  return m;
}

function base64(text) {
  return btoa(Array.from(new TextEncoder().encode(text), b => String.fromCharCode(b)).join(''));
}

// Installation logic is published with the release. Configuration stays local
// until the user runs this command; the base URL follows the selected manifest.
export function makeCommand(m, manifest, manifestURL) {
  validateManifest(manifest, manifestURL);
  if (!m.manualTarget && !manifest.capabilities.includes('rum-discovery-v1')) throw new SetupError('error.discoveryUnsupported');
  if (m.sdkLoading === 'sync' && !manifest.capabilities.includes('rum-sdk-loading-v1')) throw new SetupError('error.syncUnsupported');
  if (!m.manualTarget && (m.sdkSource === 'cdn' || m.sdkLoading === 'sync') && !manifest.capabilities.includes('rum-discovery-sdk-v1')) throw new SetupError('error.discoverySDKUnsupported');
  const config = base64(makeConfig(m));
  const base = new URL(manifest.artifactBase || '.', manifestURL).href;
  if (m.server === 'iis') {
    const script = new URL(manifest.installers.windows.file, base).href;
    return `[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12; & ([scriptblock]::Create((Invoke-WebRequest -UseBasicParsing ${psQuote(script)}).Content)) -BaseUrl ${psQuote(base)} -Config ${psQuote(config)}${m.manualTarget ? '' : ' -Discover'}`;
  }
  const script = new URL(manifest.installers.linux.file, base).href;
  const pipeline = `curl --proto '=https' --proto-redir '=https' -fsSL ${shellQuote(script)} | sudo sh -s -- --base-url ${shellQuote(base)} --config ${shellQuote(config)}${m.manualTarget ? '' : ' --discover'}`;
  return `bash -o pipefail -c ${shellQuote(pipeline)}`;
}
