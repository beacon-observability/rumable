# Platform and version compatibility

CLI, server, instrumentation, and browser requirements are separate. Optional server-side OTel needs stricter environments than RUM. See the [OTel support matrix](otel.md#support-scope).

## Compiler and operating system

Source declares Go 1.20 and uses compatible standard-library APIs. The pinned TOML dependency is v1.6.0. Modern release packages use Go 1.26.x; legacy packages must actually be built with Go 1.20.14. There are no unofficial runtime patches.

| Package | Compiler | Operating-system requirements |
| --- | --- | --- |
| Windows 386 / amd64 / arm64 modern | Go 1.26.x | Windows 10+ / Server 2016+, matching architecture |
| Windows 386 / amd64 legacy | Go 1.20.14 | Applicable old x86/x64 Windows, including Windows 7 SP1+ / Server 2008 R2 SP1 and Server 2012 / 2012 R2 |
| Linux 386 / amd64 / arm64 modern | Go 1.26.x | Kernel 3.2+ with architecture-specific requirements |
| Linux 386 / amd64 legacy | Go 1.20.14 | Kernel 2.6.32+; CentOS 5's 2.6.18 is unsupported |

Only Linux and Windows packages are distributed. CGO_ENABLED=0 removes a host glibc dependency; GOAMD64=v1, GO386=sse2, and GOARM64=v8.0 preserve baseline CPU targets. Kernel, Windows API, and architecture requirements still apply.

Go 1.20 is unmaintained; legacy artifacts lack later standard-library security fixes. Prefer modern packages. Verify old-system TLS and certificate chains without disabling certificate validation.

References: [Go minimum requirements](https://go.dev/wiki/MinimumRequirements), [Go 1.21 Windows changes](https://go.dev/doc/go1.21#windows), [Go 1.20 Windows support](https://go.dev/doc/go1.20#windows), and [maintenance policy](https://go.dev/doc/devel/release#policy).

## Nginx RUM

Nginx needs 1.9.4+ and `--with-http_sub_module`. Version strings alone are insufficient. Automatic installation, discovery, attachment, and attachment rollback run on Linux. Windows/macOS Nginx is outside that automatic scope.

No njs, Lua, OpenResty, add_header_inherit, or recent-version-only directive is required. A technical minimum does not constitute a production recommendation or remove old-server security issues. Existing compression, cache, filter, and header inheritance must be reviewed.

### Nginx 1.9.4 guard

Inherited http-level sub_filter can crash workers in 1.9.4. Before attachment, enable, rollback to enabled state, and candidate reload, the tool inspects the effective `nginx -T` include graph. Any http-level sub_filter is rejected, even when a lower context overrides it or attachmentReviewed is true. A patched package still reporting 1.9.4 is subject to the guard.

Literal and quoted paths, relative nested includes, simple `*` / ASCII `?` masks, and repeated references in different contexts are supported. Comments, quoted text, and map data are not mistaken for directives.

Missing/inconsistent dumps, unknown include contexts, cycles, excessive depth, and unsupported masks fail closed. Character-class masks, wildcard directories followed by `..`, and ambiguous non-ASCII `?` matching require explicit paths or simple masks. This version-specific restriction does not apply to 1.9.5+.

Disable remains available but still requires native configuration and HTTP verification. Recovery encountering an unsafe enabled candidate retains the journal for review. Move business filters to server/location or upgrade before retrying.

References: [sub_filter module](https://nginx.org/en/docs/http/ngx_http_sub_module.html) and [Nginx changes](https://nginx.org/en/CHANGES).

HTML HEAD responses receive the same filter-related header cleanup as eligible GETs without a body. Method maps remain compatible with old map bucket defaults. SDK and script probes verify identity/gzip bytes; a passing identity response does not validate stale compressed assets.

## IIS and PowerShell

- IIS 7.5+, URL Rewrite 2.x, Static Content, and anonymous static-file access are required for RUM.
- Each target application, including a child, requires its own started pool. Lifecycle changes recycle that pool and wait for old workers; in-process sessions may be lost. The tool does not allocate/start pools or recycle shared pools.
- RUM uses Microsoft.Web.Administration and Windows PowerShell. It does not require .NET 6/8, PowerShell 7, ASP.NET Core, or ARR.
- The manual bridge requires 64-bit Windows PowerShell 3.0+; 5.1 is recommended. One-command Windows installation requires 5.1+. Default PowerShell 2.0 on old Windows is insufficient; deploy appropriate WMF beforehand.
- TOML must be UTF-8. Older Out-File defaults can produce UTF-16; use init or explicitly choose UTF-8.
- HTML must reach URL Rewrite uncompressed. With permission, the tool disables both IIS compression attributes in the selected application and restores original values/inheritance on disable. It does not change backend middleware.
- Locked configuration, reserved mapping conflicts, rewriteBeforeCache=true, invalid ACLs, and MIME/handler issues require operator correction.
- x86 CLI on 64-bit Windows uses Sysnative for the 64-bit bridge. Native 32-bit Windows only supports limited generation/status/probe commands, not IIS management. Windows ARM64 management also requires native API/PowerShell support.
- Historical executable JavaScript MIME types such as application/x-javascript are accepted.

References: [URL Rewrite 2](https://learn.microsoft.com/en-us/iis/extensions/url-rewrite-module/using-url-rewrite-module-20) and [JavaScript MIME types](https://mimesniff.spec.whatwg.org/#javascript-mime-type).

IIS RUM clears old conditional headers for selected HTML navigation GETs and needs two application-level allowedServerVariables permissions. Review inheritance, recovery, and conditional-cache implications in [operations](operations.md#compression-and-caching).

## Optional OTel

Nginx requires Linux 1.25.3+ and a matching native module. Automatic dependency installation targets x64/ARM64 through exact-version APT packages or verified offline archives.

IIS OTel requires IIS 10, Windows 10/Server 2016+, administrator 64-bit Windows PowerShell 5.1, and a dedicated .NET pool. The pinned automatic Agent is v1.17.0 on x64 Windows with x64/x86 pools. Windows ARM64 is rejected, including from an emulated management host. Old Windows/IIS remains RUM-only.

ASP.NET Core requires No Managed Code. Conflicting machine, WAS/W3SVC, pool, and application-level instrumentation is rejected. See [installation](otel-installation.md) and the [OTel guide](otel.md).

## Browsers and CDN

Visitor browsers follow the bundled Web SDK's requirements independently of server OS compatibility. Passing a legacy Windows CLI test does not imply support for old browsers.

CDN loading does not change server minimums. Browsers must reach the HTTPS SDK URL, CSP must permit it, and SRI needs browser support plus CDN CORS. Validate replay, Workers, storage, and actual reporting in intended browsers. No broader browser support is inferred from server-side tests.
