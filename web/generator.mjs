// These fields map to internal/rumable/rum.go and the bundled Web SDK's init.
export const traceTypes = ['ddtrace', 'zipkin', 'zipkin_single_header', 'w3c_traceparent', 'w3c_traceparent_64bit', 'skywalking_v3', 'jaeger'];
const boolFields = ['trackUserInteractions', 'enablePrivacyForActionName', 'enableLongAnimationFrame', 'compressIntakeRequests', 'remoteConfiguration', 'trackSessionAcrossSubdomains', 'useSecureSessionCookie', 'allowFallbackToLocalStorage'];
const rateFields = ['sessionSampleRate', 'sessionOnErrorSampleRate', 'sessionReplaySampleRate', 'sessionReplayOnErrorSampleRate', 'tracingSampleRate'];
const stringFields = ['env', 'service', 'version', 'actionNameAttribute', 'workerUrl'];
const encode = (v) => JSON.stringify(v).replace(/\u007f/g, '\\u007f');
const lines = (v) => String(v || '').split(/\r?\n/).map(x => x.trim()).filter(Boolean);
export const shellQuote = v => "'" + String(v).replaceAll("'", "'\"'\"'") + "'";
export const psQuote = v => "'" + String(v).replaceAll("'", "''") + "'";

function url(value, label, {origin = false, https = false} = {}) {
  let u;
  try { u = new URL(value); } catch { throw Error(`${label}需要完整的 HTTP(S) 地址`); }
  if (!['http:', 'https:'].includes(u.protocol) || u.username || u.password || String(value).includes('#') || /[\s\\]/u.test(value)) throw Error(`${label}格式不正确`);
  if (https && u.protocol !== 'https:') throw Error(`${label}必须使用 HTTPS`);
  if (origin && (String(value).includes('?') || (u.pathname !== '' && u.pathname !== '/'))) throw Error(`${label}只填写域名和端口，不带路径或查询参数`);
  return u;
}

export function makeConfig(m) {
  if (!/^[a-z][a-z0-9-]{0,47}$/.test(m.id)) throw Error('接入标识需以小写字母开头，使用小写字母、数字或短横线，最多 48 位');
  const base = url(m.publicURL, '网站地址');
  if (m.publicURL.includes('?') || base.pathname.includes('%') || !base.pathname.endsWith('/')) throw Error('网站地址必须以 / 结尾，不带查询参数或编码路径');
  if (!['nginx', 'iis'].includes(m.server)) throw Error('请选择 Nginx 或 IIS');
  for (const key of ['applicationId', ...(m.reporting === 'openway' ? ['clientToken'] : [])]) {
    if (!m[key] || /[&#+%;\s\p{Cc}]/u.test(m[key])) throw Error(`${key}必填，且不能包含空白或 & # + % ; 字符`);
  }
  if (!['openway', 'datakit'].includes(m.reporting)) throw Error('请选择上报方式');
  const endpointKey = m.reporting === 'openway' ? 'site' : 'datakitOrigin';
  const endpoint = url(m.endpoint, '上报地址', {origin: true});
  if (base.protocol === 'https:' && endpoint.protocol !== 'https:') throw Error('HTTPS 网站需要 HTTPS 上报地址');
  const rum = {applicationId: m.applicationId, [endpointKey]: endpoint.origin};
  if (m.reporting === 'openway') rum.clientToken = m.clientToken;
  for (const key of stringFields) if (m[key]) rum[key] = String(m[key]);
  for (const key of rateFields) {
    if (m[key] === undefined || m[key] === '') continue;
    const n = Number(m[key]);
    if (!Number.isFinite(n) || n < 0 || n > 100) throw Error(`${key}必须在 0–100 之间`);
    rum[key] = n;
  }
  for (const key of boolFields) if (m[key] !== undefined) rum[key] = !!m[key];
  rum.sessionReplayEnabled = !!m.sessionReplayEnabled;
  if (!rum.sessionReplayEnabled) { rum.sessionReplaySampleRate = 0; rum.sessionReplayOnErrorSampleRate = 0; }
  if (!['mask', 'mask-user-input', 'allow'].includes(m.defaultPrivacyLevel)) throw Error('请选择隐私等级');
  rum.defaultPrivacyLevel = m.defaultPrivacyLevel;
  if (!['auto', 'cookie', 'local-storage'].includes(m.sessionPersistence)) throw Error('请选择会话存储方式');
  if (m.sessionPersistence !== 'auto') rum.sessionPersistence = m.sessionPersistence;
  if (rum.allowFallbackToLocalStorage && m.sessionPersistence !== 'auto') throw Error('存储回退需要将会话存储设为自动选择');
  if (rum.useSecureSessionCookie && base.protocol !== 'https:') throw Error('安全 Cookie 需要 HTTPS 网站');
  if (rum.workerUrl) {
    const worker = new URL(rum.workerUrl, base);
    if (rum.workerUrl.startsWith('//') || (!rum.workerUrl.startsWith('/') && !/^https?:\/\//.test(rum.workerUrl)) || worker.origin !== base.origin || worker.hash || worker.username || worker.password) throw Error('Worker 地址必须是同源的绝对地址或 / 开头的路径');
  }
  rum.allowedTracingUrls = lines(m.allowedTracingUrls);
  for (const raw of rum.allowedTracingUrls) url(raw, '链路匹配地址');
  if (rum.allowedTracingUrls.length) {
    if (!traceTypes.includes(m.traceType)) throw Error('请选择链路协议');
    rum.traceType = m.traceType;
  }
  const paths = key => {
    const values = lines(m[key]);
    for (const path of values) if (!path.startsWith('/') || path.startsWith('//') || /[?#%\s\\]/u.test(path)) throw Error(`${key}需要 / 开头的路径，不带查询、空白或编码字符`);
    return values;
  };
  const include = paths('includePaths'), exclude = paths('excludePaths'), probes = paths('probePaths');
  if (!include.length || !probes.length) throw Error('至少填写一个注入路径和一个验收路径');
  for (const p of probes) if (!include.some(v => p.startsWith(v)) || exclude.some(v => p.startsWith(v)) || p.startsWith(base.pathname + '__rumable__/')) throw Error('验收路径必须位于注入范围内且不能被排除');
  let target;
  if (m.server === 'nginx') {
    if (!m.nginxBinary?.startsWith('/') || /[\r\n\0]/.test(m.nginxBinary)) throw Error('Nginx 程序需要绝对路径');
    target = `[sites.nginx]\nbinary = ${encode(m.nginxBinary)}\nattachmentReviewed = false`;
    if (m.nginxConfig) {
      if (!m.nginxConfig.startsWith('/')) throw Error('Nginx 配置需要绝对路径');
      target += `\nargs = ["-c", ${encode(m.nginxConfig)}]`;
    }
  } else {
    if (!m.iisSite?.trim() || !m.iisApplication?.startsWith('/') || /[\\\r\n]/.test(m.iisSite + m.iisApplication)) throw Error('请填写 IIS 站点名及 / 开头的应用路径');
    if ((m.iisApplication === '/' ? '/' : m.iisApplication + '/') !== base.pathname) throw Error('网站地址的路径需与 IIS 应用路径一致');
    target = `[sites.iis]\nsite = ${encode(m.iisSite)}\napplication = ${encode(m.iisApplication)}\nallowCompressionChanges = ${!!m.allowCompressionChanges}`;
  }
  return `schemaVersion = 1\n\n[[sites]]\nid = ${encode(m.id)}\npublicURL = ${encode(m.publicURL)}\nprobePaths = ${encode(probes)}\nincludePaths = ${encode(include)}\nexcludePaths = ${encode(exclude)}\n\n${target}\n\n[sites.rum]\n${Object.entries(rum).map(([k,v]) => `${k} = ${encode(v)}`).join('\n')}\n`;
}

export function validateManifest(m, manifestURL) {
  const source = url(manifestURL, '发布清单', {https: true});
  if (source.search) throw Error('发布清单使用固定的版本地址，不带查询参数');
  if (m.schemaVersion !== 1 || !/^\d+\.\d+\.\d+(?:-[a-z0-9]+(?:\.[a-z0-9]+)*)?$/.test(m.version) || !m.capabilities?.includes('rum-setup-v1') || !m.capabilities?.includes('rum-install-v1') || !Array.isArray(m.packages) || !m.sdk?.version) throw Error('清单不包含支持一键接入的版本');
  if ((m.artifactBase !== undefined && m.artifactBase !== `../${m.version}/`) || (/\/latest\/manifest\.json$/.test(source.pathname) && !m.artifactBase)) throw Error('latest 清单必须指向对应的固定版本目录');
  for (const [platform, file] of [['linux', 'install.sh'], ['windows', 'install.ps1']]) {
    const installer = m.installers?.[platform];
    if (installer?.file !== file || !/^[a-f0-9]{64}$/.test(installer.sha256)) throw Error('清单缺少有效的一键安装脚本，请选择 CI 发布的新版本');
  }
  const seen = new Set();
  for (const p of m.packages) {
    const key = `${p.os}/${p.arch}/${p.flavor}`;
    if (seen.has(key) || !['linux', 'windows'].includes(p.os) || !['386', 'amd64', 'arm64'].includes(p.arch) || !['modern', 'legacy'].includes(p.flavor)) throw Error('发布清单存在重复或无效平台');
    seen.add(key);
    const file = `rumable_${m.version}_${p.os}_${p.arch}${p.flavor === 'legacy' ? '_legacy' : ''}${p.os === 'windows' ? '.zip' : '.tar.gz'}`;
    if (p.file !== file || !/^[a-f0-9]{64}$/.test(p.sha256)) throw Error('发布包名称或 SHA-256 无效');
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
  const config = base64(makeConfig(m));
  const base = new URL(manifest.artifactBase || '.', manifestURL).href;
  if (m.server === 'iis') {
    const script = new URL(manifest.installers.windows.file, base).href;
    return `& ([scriptblock]::Create((Invoke-WebRequest -UseBasicParsing ${psQuote(script)}).Content)) -BaseUrl ${psQuote(base)} -Config ${psQuote(config)}`;
  }
  const script = new URL(manifest.installers.linux.file, base).href;
  return `curl --proto '=https' --proto-redir '=https' -fsSL ${shellQuote(script)} | sudo sh -s -- --base-url ${shellQuote(base)} --config ${shellQuote(config)}`;
}
