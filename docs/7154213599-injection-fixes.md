# 7154213599: Nginx injection and CDN validation

## Range requests

Requests carrying `Range` bypass Rumable injection before the Nginx body filter, preserving origin range handling. Ordinary HTML requests still receive scripts; application-owned filters retain their behavior.

The `etag off` and `if_modified_since off` safeguards remain to prevent static HTML from returning 304 with stale validators. Static strong-ETag `If-Range` requests may therefore fall back to a complete 200 response without injection. Date-based `If-Range` and origin 206 responses remain supported.

## CDN SDK

Check and enable fetch the configured HTTPS SDK and validate a stable v3 version, SDK markers, JavaScript MIME, consistent identity/gzip bytes, and optional CORS. Its version need not match the bundled SDK.

Enable generates deployment assets from that same resolved file and saves its actual version, URL, and SHA-256. Deployed probes and rollback validate the saved digest and reject remote changes. Disable does not require CDN availability. Bundled mode continues using the embedded manifest.

The operator chooses and trusts the source. Version and marker checks identify candidate SDK format; a digest establishes byte consistency, not official provenance, completeness, or collection compatibility. Integrity defaults to false. When enabled, SRI uses the digest saved at activation. Mutable CDN changes cause old deployed probes to fail; re-enable to validate and record the new version and digest. Existing SRI-protected pages reject changed files.

## Nginx head tags

New deployments match plain `<head>` case-insensitively. Verification paths do not generate per-path rules. Legacy 0.1.9 attribute anchors remain for historical compatibility; updates validate templates first, while disable and rollback can restore old configuration. No new modules or installation options are needed. See [plain head injection](nginx-plain-head.md).

## Validation entry point

Native integration requires `RUMABLE_TEST_NGINX` and covers plain head, multiple pages, legacy migration/failure protection/disable/rollback, Range, and CDN lifecycle:

~~~sh
RUMABLE_TEST_NGINX=/absolute/path/to/nginx go test ./internal/rumable \
  -run 'TestNginxPlainHead|TestNginxLegacyHeadMigration|TestNginxRangeResponses|TestCDNEnableUpgradeRollback' -count=3 -v
~~~

Native cases explicitly skip without this variable. Browser acceptance and HTTP checks do not establish stored data in a shared workspace.
