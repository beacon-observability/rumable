# Nginx plain head injection

New or updated Nginx RUM deployments match plain `<head>` case-insensitively, including `<HEAD>` and `<Head>`. Attributes, whitespace before the closing bracket, newlines, tabs, self-closing tags, and missing head tags are unsupported. Identical tag-shaped text before the real head is ambiguous and rejected by preflight. A fixed-string filter is not an HTML parser: passing verification does not validate untested pages or future template changes.

`includePaths` and `excludePaths` control injection scope. `probePaths` selects HTTP verification pages only; it does not learn anchors or generate per-path rules. Plain-head pages outside the verification list can still receive scripts. IIS retains its existing plain-head boundary.

No new module, configuration switch, or installation option is required. The Range and CDN fixes from the GitLab 0.1.9 line are retained.

Replacing the CLI does not change deployed configuration. Before updating a legacy 0.1.9 RUM deployment, Rumable checks configured verification paths and saved attribute-head paths still in scope. Incompatibility stops the operation before configuration changes, preserving the deployment. Change affected templates to plain head before updating. Disable, OTel-only operations, and rollback continue reading legacy snapshots and retain the information needed by old rules. Successful updates generate plain-head rules without per-path anchors.

## Upstream verification record

The GitLab source records three consecutive targeted integration passes with Go 1.20.3 and native Nginx 1.24.0 with sub_filter/PCRE: plain and mixed-case head, multiple pages and queries, exclusions, rejected attributes/whitespace, verification-path independence, legacy migration/failure protection/disable/rollback, Range/cache behavior, and CDN lifecycle. It also records passing full Go tests, vet, and CLI build.

Its Chrome acceptance used generated Nginx configuration, the embedded SDK, synthetic local pages, and a local receiver. Asynchronous home and unlisted pages initialized; synchronous mixed-case pages initialized; attribute-head, spaced-head, and excluded pages received no scripts. The local receiver returned 202; shared-workspace ingestion was not verified. The source also records six-language desktop and 390px layout checks and client/configuration/component/locale tests. These are upstream historical results, separate from validation of this merge.
