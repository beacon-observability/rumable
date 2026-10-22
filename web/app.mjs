import {makeConfig, makeCommand, validateManifest} from './generator.mjs';
const form = document.querySelector('#config-form');
const $ = id => document.getElementById(id);
let manifest = null, source = '', tab = 'config', config = '', command = '', generation = 0;
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
  $('sdk-summary').textContent = cdn ? 'CDN' : '网站同域';
  const sync = m.sdkLoading === 'sync';
  $('loading-summary').textContent = sync ? '同步' : '异步';
  $('sdk-loading-help').textContent = sync
    ? '先加载并执行 SDK，再初始化，之后继续解析页面，便于采集后续业务脚本的早期事件。两次脚本请求会阻塞 HTML 解析；网络慢或 CDN 故障可能延迟首屏。'
    : '初始化脚本和 SDK 均异步加载，下载不阻塞 HTML 解析；初始化完成前的错误、请求和交互可能无法采集。';
  $('sdk-source-help').textContent = cdn
    ? '初始化脚本仍由网站同域提供，SDK 从 CDN 下载。网站的 CSP 需允许该脚本地址。'
    : 'SDK 随 Rumable 安装包提供，由你的网站同域托管；浏览器无需访问外部 CDN。';
  $('sdk-update-help').textContent = cdn
    ? '官方 v3 是可变地址，CDN 更新后浏览器可能在缓存过期后加载新版，无需重新部署。启用和探测时仍要求文件与安装包一致；CDN 先更新时需升级 Rumable 或改用匹配的固定地址。回滚只能恢复 URL，无法恢复远程文件；加载失败不会自动改用同域副本。'
    : '同域 SDK 固定在安装包版本。安装新版 Rumable 后，需对原站点重新执行接入命令或 enable 才会升级；仅下载或替换程序不会更新站点。保留旧资源以便回滚。';
  $('token-field').hidden = m.reporting !== 'openway';
  form.elements.clientToken.required = m.reporting === 'openway';
  $('endpoint-help').textContent = `对应 ${m.reporting === 'openway' ? 'site' : 'datakitOrigin'}，仅填写 origin`;
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
  $('replay-summary').textContent = m.sessionReplayEnabled ? `${m.sessionReplaySampleRate}%` : '关闭';
  config = ''; command = '';
  try {
    config = makeConfig(m);
    if (manifest) command = makeCommand(m, manifest, source);
    $('form-error').textContent = '';
  } catch (error) { $('form-error').textContent = error.message; }
  $('output-text').textContent = tab === 'config' ? (config || '填写必填项后生成配置。') : (command || '读取有效发布清单并完成配置后生成命令。');
  $('copy-command').disabled = !command;
  $('download-config').disabled = !config;
  $('tab-config').classList.toggle('selected', tab === 'config');
  $('tab-command').classList.toggle('selected', tab === 'command');
}
form.addEventListener('input', () => { $('action-status').textContent = ''; render(); });
form.addEventListener('change', render);
form.addEventListener('submit', e => e.preventDefault());
$('tab-config').onclick = () => { tab = 'config'; render(); };
$('tab-command').onclick = () => { tab = 'command'; render(); };
$('manifest-url').oninput = () => {
  generation++; manifest = null; source = '';
  $('release-status').textContent = '发布地址已更改，请重新读取版本。'; render();
};
$('load-release').onclick = async () => {
  const id = ++generation, raw = $('manifest-url').value.trim();
  manifest = null; source = ''; render();
  $('release-status').textContent = '正在读取发布版本…';
  try {
    const parsed = new URL(raw);
    if (parsed.protocol !== 'https:' || parsed.username || parsed.password || parsed.hash || parsed.search) throw Error('请使用不含凭据和查询参数的 HTTPS 发布清单地址');
    const response = await fetch(raw, {credentials:'omit', redirect:'error', cache:'no-store', signal:AbortSignal.timeout(15000)});
    if (!response.ok) throw Error(`下载清单失败（HTTP ${response.status}）`);
    const bytes = await response.text();
    if (bytes.length > 128 * 1024) throw Error('发布清单超过大小限制');
    const candidate = validateManifest(JSON.parse(bytes), raw);
    if (id !== generation) return;
    manifest = candidate; source = raw;
    $('release-status').textContent = `Rumable ${candidate.version} · Web SDK ${candidate.sdk.version}`;
  } catch (error) {
    if (id !== generation) return;
    $('release-status').textContent = `无法读取发布版本：${error.message}。请检查地址和 OSS 跨域配置。`;
  }
  render();
};
$('copy-command').onclick = async () => {
  if (!command) return;
  try { await navigator.clipboard.writeText(command); $('action-status').textContent = '命令已复制。请在目标服务器终端执行。'; }
  catch { tab = 'command'; render(); $('action-status').textContent = '无法访问剪贴板，请手动选择并复制命令。'; }
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
render();
if ($('manifest-url').value) $('load-release').click();
