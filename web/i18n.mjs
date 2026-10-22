// Paired Chinese / English strings keep setup translations together.
export const messages = {
  "page.title": ["RUM 一键接入 · Rumable", "One-click RUM setup · Rumable"],
  "nav.category": ["应用可观测性", "Application observability"],
  "nav.rum": ["浏览器 RUM 接入", "Browser RUM setup"],
  "nav.experience": ["从用户体验出发", "Start with user experience"],
  "nav.noCode": ["无需修改业务代码", "No application code changes"],
  "header.monitoring": ["应用监测", "Application monitoring"],
  "header.newApp": ["接入新应用", "Connect an application"],
  "header.injection": ["Web SDK · 自动注入", "Web SDK · Automatic injection"],
  "page.heading": ["让每一次用户体验，都有迹可循", "Understand every user experience"],
  "page.description": ["配置 Web SDK，在服务器执行生成的命令，即可接入浏览器监测。", "Configure the Web SDK and run the generated command on your server to start browser monitoring."],
  "app.heading": ["连接你的应用", "Connect your application"],
  "app.description": ["使用应用管理中创建的 RUM 应用信息。", "Use the details of the RUM application you created in application management."],
  "field.applicationId": ["应用 ID", "Application ID"],
  "app.idHelp": ["对应 Web SDK 的 applicationId", "The Web SDK applicationId"],
  "field.reporting": ["上报方式", "Reporting method"],
  "field.endpoint": ["上报地址", "Reporting endpoint"],
  "app.endpointInitial": ["对应 site，仅填写 origin", "Maps to site; enter the origin only"],
  "field.clientToken": ["浏览器令牌", "Browser token"],
  "app.tokenHelp": ["使用浏览器 Client Token，不是管理 API Key。", "Use a browser Client Token, not a management API Key."],
  "field.service": ["服务名称", "Service name"],
  "field.env": ["环境", "Environment"],
  "field.version": ["应用版本", "Application version"],
  "app.versionHelp": ["业务应用版本，与 Rumable 安装包版本独立。", "Your application version, independent of the Rumable package version."],
  "sdk.heading": ["选择 SDK 来源与加载方式", "Choose how to load the SDK"],
  "sdk.description": ["服务器在 HTML 中加入脚本，浏览器按所选方式加载并初始化 SDK。", "The server injects scripts into HTML. The browser loads and initializes the SDK using your selected method."],
  "field.sdkSource": ["SDK 来源", "SDK source"],
  "sdk.bundledDefault": ["网站同域（默认）", "Same origin (default)"],
  "sdk.bundledHelp": ["SDK 随 Rumable 安装包提供，由你的网站同域托管；浏览器无需访问外部 CDN。", "The SDK is bundled with Rumable and served from your website's origin. Browsers do not need an external CDN."],
  "field.sdkURL": ["CDN 地址", "CDN URL"],
  "sdk.urlHelp": ["使用完整 HTTPS 脚本地址，不带查询参数、片段或账号密码。自建 CDN 需提供与所选安装包一致的 SDK 文件。", "Use a full HTTPS script URL without query parameters, fragments or credentials. Your CDN must serve the same SDK file as the selected package."],
  "sdk.integrity": ["校验脚本完整性（SRI）", "Verify script integrity (SRI)"],
  "sdk.integrityHelp": ["跨域 SRI 需要 CDN 允许 CORS。官方 v3 地址默认关闭此项；自建固定版本地址可开启，安装时会检查。", "Cross-origin SRI requires CORS support on the CDN. It is off by default for the official v3 URL. You can enable it for a pinned URL on your CDN; installation checks compatibility."],
  "field.sdkLoading": ["加载方式", "Loading mode"],
  "sdk.asyncDefault": ["异步（默认）", "Asynchronous (default)"],
  "sdk.sync": ["同步", "Synchronous"],
  "sdk.asyncHelp": ["初始化脚本和 SDK 均异步加载，下载不阻塞 HTML 解析；初始化完成前的错误、请求和交互可能无法采集。", "The initialization script and SDK load asynchronously without blocking HTML parsing. Errors, requests and interactions before initialization may not be captured."],
  "sdk.updates": ["SDK 如何更新", "How SDK updates work"],
  "sdk.bundledUpdate": ["同域 SDK 固定在安装包版本。安装新版 Rumable 后，需对原站点重新执行接入命令或 enable 才会升级；仅下载或替换程序不会更新站点。保留旧资源以便回滚。", "The same-origin SDK is pinned to the package. After installing a newer Rumable package, run setup or enable again for the original site to update it. Downloading or replacing the CLI alone does not update the site. Keep old assets for rollback."],
  "sdk.versionHelp": ["“应用版本”用于标记业务版本，“SDK 远程配置”用于调整采集参数，两者都不会升级 SDK 文件。", "Application version labels your application release. SDK remote configuration adjusts collection settings. Neither updates the SDK files."],
  "collection.heading": ["决定采集什么", "Choose what to collect"],
  "collection.description": ["性能、资源和错误由 SDK 自动采集，以下控制采样与录制。", "The SDK automatically collects performance, resource and error data. Configure sampling and recording below."],
  "field.sessionSampleRate": ["会话采样率", "Session sample rate"],
  "field.sessionOnErrorSampleRate": ["错误会话补偿采样率", "Additional error session sample rate"],
  "collection.interactions": ["采集用户交互", "Track user interactions"],
  "collection.interactionsHelp": ["自动记录点击等用户操作 · trackUserInteractions", "Automatically record clicks and other actions · trackUserInteractions"],
  "collection.replayHelp": ["录制会话回放，帮助还原用户遇到的问题。", "Record sessions to help reproduce the issues users encounter."],
  "field.sessionReplaySampleRate": ["回放采样率", "Replay sample rate"],
  "field.sessionReplayOnErrorSampleRate": ["错误回放补偿采样率", "Additional error replay sample rate"],
  "collection.replaySamplingHelp": ["先采样 RUM 会话，再决定是否录制。回放需要站点 CSP 允许相应 Worker。", "RUM sessions are sampled before replay recording is selected. Your site's CSP must allow the required Worker."],
  "field.privacy": ["默认隐私等级", "Default privacy level"],
  "privacy.mask": ["遮罩所有文本（推荐）", "Mask all text (recommended)"],
  "privacy.maskInput": ["仅遮罩用户输入", "Mask user input only"],
  "privacy.allow": ["显示允许采集的内容", "Show permitted content"],
  "field.persistence": ["会话存储", "Session storage"],
  "persistence.auto": ["自动选择（支持存储回退）", "Automatic (supports storage fallback)"],
  "advanced.heading": ["高级 Web SDK 配置", "Advanced Web SDK settings"],
  "field.actionName": ["交互名称属性", "Action name attribute"],
  "field.workerURL": ["Worker 地址", "Worker URL"],
  "advanced.workerHelp": ["同源地址 · workerUrl", "Same-origin URL · workerUrl"],
  "field.tracingURLs": ["链路匹配地址", "Tracing URL patterns"],
  "advanced.tracingHelp": ["allowedTracingUrls · 未填写时不注入链路请求头", "allowedTracingUrls · No tracing headers are injected when empty"],
  "field.traceType": ["链路协议", "Tracing protocol"],
  "field.tracingSampleRate": ["链路采样率", "Trace sample rate"],
  "advanced.actionPrivacy": ["遮罩交互名称", "Mask action names"],
  "advanced.longFrames": ["采集长动画帧", "Collect long animation frames"],
  "advanced.compression": ["压缩上报请求（需要 Worker）", "Compress reporting requests (requires a Worker)"],
  "advanced.remoteConfig": ["启用 SDK 远程配置", "Enable SDK remote configuration"],
  "advanced.subdomains": ["跨子域共享会话", "Share sessions across subdomains"],
  "advanced.secureCookie": ["仅 HTTPS 传输会话 Cookie", "Send session cookies over HTTPS only"],
  "advanced.storageFallback": ["自动选择时允许回退 Local Storage", "Allow Local Storage fallback in automatic mode"],
  "advanced.help": ["远程配置可能覆盖静态参数。setUser、自定义 startView、beforeSend 等需要业务调用，不通过服务器注入自动实现。", "Remote configuration may override static settings. setUser, custom startView and beforeSend require application code and are not implemented by server injection."],
  "server.heading": ["选择安装环境", "Choose your server environment"],
  "server.description": ["网站信息由安装命令在服务器上自动发现。", "The installation command discovers websites on your server."],
  "field.server": ["安装环境", "Server environment"],
  "server.archHelp": ["需要 Bash、curl 和 sudo；安装时自动识别 Linux x86（32 位）/ x64 / ARM64 架构。", "Requires Bash, curl and sudo. Installation detects Linux x86 (32-bit), x64 or ARM64 automatically."],
  "server.discoveryHelp": ["只有一个站点时自动选择；多个站点或访问地址时在终端选择。无法确定域名时，命令会提示输入网站地址。默认接入所选站点或应用的全部路径。", "A single site is selected automatically. Choose in the terminal when there are multiple sites or URLs. If the hostname cannot be determined, you will be prompted for the URL. All paths of the selected site or application are included by default."],
  "server.advanced": ["高级接入设置（可选）", "Advanced setup (optional)"],
  "server.manual": ["手动指定站点与路径", "Specify the site and paths manually"],
  "field.publicURL": ["网站地址", "Website URL"],
  "server.urlHelp": ["以 / 结尾，需可访问验收页面。", "End with / and provide an accessible verification page."],
  "field.id": ["接入标识", "Site ID"],
  "server.idHelp": ["用于后续启停和回滚，同一站点保持不变。", "Used to enable, disable and roll back. Keep it unchanged for the same site."],
  "field.includePaths": ["注入路径", "Included paths"],
  "server.includeHelp": ["每行一个路径前缀，区分大小写。", "One case-sensitive path prefix per line."],
  "field.excludePaths": ["排除路径", "Excluded paths"],
  "server.excludeHelp": ["排除优先。", "Exclusions take priority."],
  "field.probePaths": ["验收路径", "Verification paths"],
  "server.probeHelp": ["需要匿名返回完整 HTML，覆盖每种页面模板。", "Must return full HTML anonymously and cover each page template."],
  "field.nginxBinary": ["Nginx 程序", "Nginx binary"],
  "field.nginxConfig": ["Nginx 配置文件", "Nginx configuration file"],
  "field.iisSite": ["IIS 站点名称", "IIS site name"],
  "field.iisApplication": ["IIS 应用路径", "IIS application path"],
  "server.compression": ["允许调整所选应用的静态和动态压缩设置", "Allow changes to static and dynamic compression for the selected application"],
  "server.compressionHelp": ["未勾选时，自动发现流程会在需要调整时询问。停用或卸载接入会恢复原设置。", "If unchecked, discovery asks for permission when changes are needed. Disabling or uninstalling restores the original settings."],
  "server.htmlHelp": ["页面需含标准 <head> 开始标签。部署会检查 CSP、SDK 冲突和验收页面；不兼容时停止并保留恢复记录。", "Pages need a standard <head> opening tag. Deployment checks CSP, SDK conflicts and verification pages. On incompatibility, it stops and keeps recovery records."],
  "server.windowsHelp": ["IIS 需要 URL Rewrite 和静态资源读取权限。以管理员身份运行 PowerShell 5.1+，安装器自动识别 x64 / ARM64 和 Windows 版本，旧系统自动选择 legacy 包；32 位 Windows 不支持 IIS 一键接入。", "IIS requires URL Rewrite and static resource read permissions. Run PowerShell 5.1+ as administrator. The installer detects x64 / ARM64 and the Windows version, selecting a legacy package for older systems. One-click IIS setup does not support 32-bit Windows."],
  "output.heading": ["生成接入命令", "Generate your setup command"],
  "output.description": ["复制一条命令 → 自动接入", "Copy one command → Automatic setup"],
  "sdk.bundled": ["网站同域", "Same origin"],
  "sdk.async": ["异步", "Asynchronous"],
  "output.sessions": ["会话采样", "Session sampling"],
  "output.replay": ["会话回放", "Session replay"],
  "output.off": ["关闭", "Off"],
  "field.manifest": ["发布清单地址", "Release manifest URL"],
  "release.help": ["支持 latest 固定入口或指定版本，包含 SDK 版本和安装包摘要。", "Use the stable latest entry or a specific release. The manifest includes the SDK version and package checksums."],
  "release.load": ["读取发布版本", "Load release"],
  "release.initial": ["尚未连接 OSS，可先配置并下载 TOML。", "No release loaded yet. You can still configure and download TOML."],
  "release.sdkHelp": ["这里显示安装包内的 SDK 版本。可变 CDN 后续提供的版本可能不同；读取 latest 不会自动升级已接入站点。", "This shows the SDK version in the package. A mutable CDN may later serve a different version. Loading latest does not automatically upgrade existing sites."],
  "output.config": ["配置预览", "Configuration"],
  "output.command": ["接入命令", "Setup command"],
  "output.initial": ["填写应用 ID、上报地址和浏览器令牌后生成配置。", "Enter an application ID, reporting endpoint and browser token to generate the configuration."],
  "output.copy": ["复制接入命令", "Copy setup command"],
  "output.download": ["下载 rumable.toml", "Download rumable.toml"],
  "output.next": ["执行之后", "After running the command"],
  "output.nextHelp": ["在服务器查看接入结果，再打开网站确认 SDK 运行，并在观测云确认收到数据。", "Check the setup result on your server, open your website to verify the SDK runs, then confirm data arrives in Guance."],
  "output.private": ["配置仅在当前浏览器生成。命令包含浏览器令牌，Base64 不是加密。", "Configuration is generated only in this browser. The command contains the browser token. Base64 is not encryption."],
  "page.footer": ["Rumable 自动接入 · 执行命令会修改目标站点配置，保留备份供启停和回滚。", "Rumable automatic setup · Running the command changes the target site's configuration and keeps backups for disabling or rollback."],
  "placeholder.applicationId": ["填写 applicationId", "Enter applicationId"],
  "placeholder.clientToken": ["填写 clientToken", "Enter clientToken"],
  "placeholder.service": ["如 web-store", "e.g. web-store"],
  "placeholder.env": ["如 production", "e.g. production"],
  "placeholder.version": ["如 1.0.0", "e.g. 1.0.0"],
  "placeholder.actionName": ["如 data-rum-action", "e.g. data-rum-action"],
  "placeholder.workerURL": ["留空使用内置 Blob Worker", "Leave empty to use the built-in Blob Worker"],
  "placeholder.tracingURLs": ["每行一个完整 URL，如 https://api.example.com/", "One full URL per line, e.g. https://api.example.com/"],
  "placeholder.excludePaths": ["如 /api/", "e.g. /api/"],
  "placeholder.nginxConfig": ["留空使用 Nginx 默认配置", "Leave empty to use the default Nginx configuration"],
  "placeholder.manifest": ["https://OSS域名/rumable/版本/manifest.json", "https://static.guance.com/rumable/latest/manifest.json"],
  "output.accessibleName": ["生成内容", "Generated output"],
  "language.label": ["语言", "Language"],
  "language.auto": ["跟随浏览器", "Browser language"],
  "sdk.syncHelp": ["先加载并执行 SDK，再初始化，之后继续解析页面，便于采集后续业务脚本的早期事件。两次脚本请求会阻塞 HTML 解析；网络慢或 CDN 故障可能延迟首屏。", "The SDK loads and initializes before HTML parsing resumes, helping capture early events from subsequent application scripts. Both script requests block HTML parsing; slow networks or CDN failures may delay the first render."],
  "sdk.cdnHelp": ["初始化脚本仍由网站同域提供，SDK 从 CDN 下载。网站的 CSP 需允许该脚本地址。", "The initialization script is still served from your website's origin. The SDK is downloaded from the CDN, which must be allowed by your site's CSP."],
  "sdk.cdnUpdate": ["官方 v3 是可变地址，CDN 更新后浏览器可能在缓存过期后加载新版，无需重新部署。启用和探测时仍要求文件与安装包一致；CDN 先更新时需升级 Rumable 或改用匹配的固定地址。回滚只能恢复 URL，无法恢复远程文件；加载失败不会自动改用同域副本。", "The official v3 URL is mutable. After CDN updates and cache expiry, browsers may load a newer SDK without redeployment. Enable and probe still require it to match the package. If the CDN updates first, upgrade Rumable or use a matching pinned URL. Rollback restores the URL, not the remote file. Loading failures do not fall back to a same-origin copy."],
  "app.endpointHelp": ["对应 {field}，仅填写 origin", "Maps to {field}; enter the origin only"],
  "output.emptyConfig": ["填写必填项后生成配置。", "Complete the required fields to generate the configuration."],
  "output.emptyCommand": ["读取有效发布清单并完成配置后生成命令。", "Load a valid release manifest and complete the configuration to generate a command."],
  "release.changed": ["发布地址已更改，请重新读取版本。", "The manifest URL changed. Load the release again."],
  "release.loading": ["正在读取发布版本…", "Loading release…"],
  "release.loaded": ["Rumable {version} · Web SDK {sdk}", "Rumable {version} · Web SDK {sdk}"],
  "release.failed": ["无法读取发布版本：{reason}。请检查地址和 OSS 跨域配置。", "Unable to load the release: {reason}. Check the URL and OSS CORS settings."],
  "action.copied": ["命令已复制。请在目标服务器终端执行。", "Command copied. Run it in a terminal on the target server."],
  "action.copyFailed": ["无法访问剪贴板，请手动选择并复制命令。", "Clipboard access failed. Select and copy the command manually."],
  "error.manifestURL": ["请使用不含凭据和查询参数的 HTTPS 发布清单地址", "Use an HTTPS manifest URL without credentials, query parameters or fragments"],
  "error.http": ["下载清单失败（HTTP {status}）", "Manifest download failed (HTTP {status})"],
  "error.manifestSize": ["发布清单超过大小限制", "The release manifest exceeds the size limit"],
  "error.manifestJSON": ["发布清单不是有效的 JSON", "The release manifest is not valid JSON"],
  "error.network": ["网络请求失败或被浏览器拦截", "The network request failed or was blocked by the browser"],
  "error.timeout": ["读取发布清单超时", "The release manifest request timed out"],
  "error.config": ["配置无效，请检查填写的参数", "Invalid configuration. Check the entered values"],
  "error.urlRequired": ["{label}需要完整的 HTTP(S) 地址", "{label} requires a full HTTP(S) URL"],
  "error.urlInvalid": ["{label}格式不正确", "Invalid {label}"],
  "error.urlHTTPS": ["{label}必须使用 HTTPS", "{label} must use HTTPS"],
  "error.urlOrigin": ["{label}只填写域名和端口，不带路径或查询参数", "{label} must contain only the origin, without a path or query parameters"],
  "error.sdkSource": ["请选择网站同域或 CDN 加载 SDK", "Choose same origin or CDN as the SDK source"],
  "error.sdkLoading": ["请选择异步或同步加载 SDK", "Choose asynchronous or synchronous SDK loading"],
  "error.cdnPath": ["CDN 地址需包含脚本路径，不能带查询参数", "The CDN URL must include a script path without query parameters"],
  "error.siteId": ["接入标识需以小写字母开头，使用小写字母、数字或短横线，最多 48 位", "The site ID must start with a lowercase letter and contain up to 48 lowercase letters, digits or hyphens"],
  "error.siteURL": ["网站地址必须以 / 结尾，不带查询参数或编码路径", "The website URL must end with /, without query parameters or encoded paths"],
  "error.server": ["请选择 Nginx 或 IIS", "Choose Nginx or IIS"],
  "error.token": ["{field}必填，且不能包含空白或 & # + % ; 字符", "{field} is required and cannot contain whitespace or & # + % ; characters"],
  "error.reporting": ["请选择上报方式", "Choose a reporting method"],
  "error.endpointHTTPS": ["HTTPS 网站需要 HTTPS 上报地址", "HTTPS websites require an HTTPS reporting endpoint"],
  "error.rate": ["{field}必须在 0–100 之间", "{field} must be between 0 and 100"],
  "error.privacy": ["请选择隐私等级", "Choose a privacy level"],
  "error.persistence": ["请选择会话存储方式", "Choose a session storage method"],
  "error.fallback": ["存储回退需要将会话存储设为自动选择", "Storage fallback requires automatic session storage"],
  "error.secureCookie": ["安全 Cookie 需要 HTTPS 网站", "Secure cookies require an HTTPS website"],
  "error.workerURL": ["Worker 地址必须是同源的绝对地址或 / 开头的路径", "The Worker URL must be a same-origin absolute URL or a path starting with /"],
  "error.traceType": ["请选择链路协议", "Choose a tracing protocol"],
  "error.path": ["{field}需要 / 开头的路径，不带查询、空白或编码字符", "{field} requires paths starting with /, without queries, whitespace or encoded characters"],
  "error.pathsRequired": ["至少填写一个注入路径和一个验收路径", "Enter at least one included path and one verification path"],
  "error.probePath": ["验收路径必须位于注入范围内且不能被排除", "Verification paths must be included and must not be excluded"],
  "error.nginxBinary": ["Nginx 程序需要绝对路径", "The Nginx binary requires an absolute path"],
  "error.nginxConfig": ["Nginx 配置需要绝对路径", "The Nginx configuration requires an absolute path"],
  "error.iisTarget": ["请填写 IIS 站点名及 / 开头的应用路径", "Enter an IIS site name and an application path starting with /"],
  "error.iisPath": ["网站地址的路径需与 IIS 应用路径一致", "The website URL path must match the IIS application path"],
  "error.manifestQuery": ["发布清单使用固定的版本地址，不带查询参数", "Use a manifest URL without query parameters"],
  "error.manifestUnsupported": ["清单不包含支持一键接入的版本", "The manifest does not describe a release that supports one-click setup"],
  "error.manifestBase": ["latest 清单必须指向对应的固定版本目录", "The latest manifest must point to its matching immutable release directory"],
  "error.installers": ["清单缺少有效的一键安装脚本，请选择 CI 发布的新版本", "The manifest lacks valid installers. Choose a release published by CI"],
  "error.platforms": ["发布清单存在重复或无效平台", "The release manifest contains duplicate or invalid platforms"],
  "error.package": ["发布包名称或 SHA-256 无效", "Invalid package name or SHA-256 checksum"],
  "error.discoveryUnsupported": ["此版本不支持自动发现站点，请选择支持该能力的安装包，或启用手动指定站点", "This release does not support site discovery. Choose a compatible package or specify the site manually"],
  "error.syncUnsupported": ["此安装包不支持同步加载，请读取支持该功能的新发布清单，或选择异步加载", "This package does not support synchronous loading. Load a compatible release or choose asynchronous loading"],
  "error.discoverySDKUnsupported": ["此安装包不支持自动发现时配置 CDN 或加载方式，请读取支持该功能的新发布清单，或手动指定站点生成完整配置", "This package cannot configure CDN or loading mode during discovery. Load a compatible release or specify the site manually"],
};

export const languageStorageKey = 'rumable.setup.language';

export function resolveLocale(preference = 'auto', languages = []) {
  if (preference === 'zh-CN' || preference === 'en') return preference;
  for (const language of languages) {
    if (/^zh(?:-|$)/i.test(language)) return 'zh-CN';
    if (/^en(?:-|$)/i.test(language)) return 'en';
  }
  return 'en';
}

export function readLanguagePreference(storage) {
  try {
    const saved = storage?.getItem(languageStorageKey);
    if (saved === 'zh-CN' || saved === 'en') return saved;
  } catch { /* Private browsing or site policies may block storage. */ }
  return 'auto';
}

export function saveLanguagePreference(storage, preference) {
  try {
    if (preference === 'zh-CN' || preference === 'en') storage?.setItem(languageStorageKey, preference);
    else storage?.removeItem(languageStorageKey);
  } catch { /* Switching languages still works for this page. */ }
}

export function createTranslator(locale) {
  const index = locale === 'zh-CN' ? 0 : 1;
  return function t(key, values = {}) {
    return (messages[key]?.[index] ?? key).replace(/\{(\w+)\}/g, (match, name) => {
      const value = values[name];
      return value?.key ? t(value.key, value.values) : value ?? match;
    });
  };
}

export class SetupError extends Error {
  constructor(key, values = {}) {
    super(createTranslator('zh-CN')(key, values));
    this.name = 'SetupError';
    this.key = key;
    this.values = values;
  }
}

export function errorMessage(error, t, fallback = 'error.config') {
  return error instanceof SetupError ? t(error.key, error.values) : t(fallback);
}

export function translatePage(root, t) {
  for (const element of root.querySelectorAll('[data-i18n]')) {
    element.textContent = t(element.dataset.i18n);
  }
  for (const attribute of ['placeholder', 'aria-label']) {
    for (const element of root.querySelectorAll(`[data-i18n-${attribute}]`)) {
      element.setAttribute(attribute, t(element.getAttribute(`data-i18n-${attribute}`)));
    }
  }
}
