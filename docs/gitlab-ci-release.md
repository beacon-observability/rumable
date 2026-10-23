# GitLab CI and OSS releases

This is the retained GitLab/OSS publication path. New Beacon distribution uses [GitHub releases](github-release.md). Select the GitLab remote explicitly with `--remote gitlab`.

The existing deployment channel uses protected tags, a Shell Runner with Docker, ali-oss, OSS_* credentials, serial publication, and a credential-free report. Public documentation calls the product Rumable. Existing release tags, binary names, object paths, and the configured Runner label remain compatible.

This workflow publishes platform archives, installers, and static setup files, not npm packages. It does not establish a new public download domain.

## Initial configuration

Configure GitLab CI/CD variables with environment scope *, variable expansion disabled, and secrets Masked/Protected:

| Variable | Purpose |
| --- | --- |
| OSS_REGION | Storage region, such as oss-cn-hangzhou |
| OSS_BUCKET | Artifact/page bucket |
| OSS_ACCESS_KEY_ID / OSS_ACCESS_KEY_SECRET | Publishing identity with GetObject/PutObject on the intended rumable/* prefix |
| OSS_STS_TOKEN | Optional matching temporary credential |

Public download domains are not CI variables. The page resolves files against the chosen HTTPS manifest and passes the resulting base URL to installers. Identical artifacts can use different hosting domains.

Use an actual static-site/custom domain that serves HTML inline. Some raw storage domains force HTML download. The publisher sets text/html; charset=utf-8 and Content-Disposition:inline for versioned and latest pages, but HTML tags cannot override a forced-download host.

Configure anonymous artifact reads, MIME, and page-origin GET CORS externally. The publisher does not change bucket ACL, CORS, domain, or CDN settings. OSS API readback is separate from public CDN/download acceptance.

Protect release-* tags and limit creation to release maintainers. Use the Runner selected in .gitlab-ci.yml with Docker and access to compiler/container registries, Go modules, npm, and OSS. A Shell executor uses explicit Docker commands rather than GitLab image settings.

Build images contain Go 1.26.x, Go 1.20.14, and Node. MODERN_GO_IMAGE, LEGACY_GO_IMAGE, RELEASE_NODE_IMAGE, and RELEASE_POWERSHELL_IMAGE can select reviewed images/digests. The builder validates compiler versions independently.

Portable PowerShell contracts run first in a separate official PowerShell 7.4 container with read-only source, no network, and no publishing secrets. These mocks are not native Windows acceptance. Node and Go are development/release tools, not user prerequisites.

GOPROXY defaults to a reachable mirror plus direct fallback; GOSUMDB remains sum.golang.org. Override by CI configuration when necessary; do not disable checksum verification to bypass network errors.

## Prepare and trigger a version

Review source changes, the Version in internal/rumable/config.go, and release notes. For SDK updates, run scripts/sync-sdk.go during preparation, review the bytes/metadata, and commit. Tag CI verifies that committed digest and uses --skip-sdk-sync; retries do not follow mutable latest.

CLI, page, and capability changes must be released together. The manifest advertises rum-sdk-loading-v1 and rum-discovery-sdk-v1 where implemented. Replacing a page in an old immutable directory cannot add CLI capabilities.

Merge and push reviewed source to main, verify a clean checkout aligned with origin/main, then use the release script. The helper reads the current committed source version:

~~~sh
git switch main
git pull --ff-only origin main
bash scripts/release-oss.sh --remote gitlab --dry-run
# Run only when publication of the reviewed version is authorized:
bash scripts/release-oss.sh --remote gitlab
~~~

Omitting the version reads source Version. The default remote is origin; --remote selects an existing remote. The script validates the actual push destination and creates/pushes only one annotated matching release tag. It neither commits nor pushes branches and refuses conflicting tags.

A failed push retains the local tag for retry. An identical existing remote tag is reported without retriggering a pipeline. Successful tag readback does not prove storage publication; inspect the pipeline/report.

Branches, ordinary v... tags, and unprotected tags do not publish. No GitLab API token is required. The script does not configure tag protection. Stable versions promote latest only after verification; prereleases only publish their own version directory.

## Execution order

1. Verify protected tag, source version, commit, and required storage variables.
2. Check English documentation, wording, and local links; verify pinned SDK hash.
3. Install publishing dependencies from their lockfile and run page/installer/release contracts.
4. Run tests/vet and packaging contracts with both real Go compilers.
5. Build ten platform archives, two installers, five setup files, sdk.json, manifest.json, and SHA256SUMS.
6. Validate the public allowlist: 20 objects. Documents, examples, and license notices stay inside platform archives; unexpected standalone files are rejected even with valid hashes.
7. Pre-read existing immutable objects. Equal bytes can be reused; any different bytes stop publication.
8. Upload each object and GET-read its SHA-256. Upload/verify manifest only after all preceding objects pass.
9. For a stable version, promote latest page and installers, then latest manifest, verifying each readback.

Versioned paths remain rumable/<version>/...; the page is setup/index.html and installers are install.sh/install.ps1 in that same version directory. Public URLs are formed from the operator's actual HTTPS base, not guessed product-hosting addresses.

| Stable relative entry | Purpose |
| --- | --- |
| latest/setup/index.html | Redirect to a complete immutable version page |
| latest/manifest.json | Version/digests/artifactBase |
| latest/install.sh | Linux installation entry |
| latest/install.ps1 | Windows installation entry |

Resolve artifactBase=../<version>/ against the manifest URL before resolving installer or package files. Generated commands pin a version. A direct latest installer uses its compiled-in version to resolve the corresponding immutable directory, avoiding mixed old/new files.

Latest only advances numerically. Older retries skip promotion when a newer version exists. Equal-version different metadata is refused. Multiple latest writes are not atomic, but each alias references an already complete immutable release; retry can finish partial promotion. Latest uses no-store, max-age=0; immutable versions retain long-lived caching.

x-oss-forbid-overwrite and a serial resource group supplement byte checks. This header does not enforce immutability in a versioned/suspended bucket; use one writer for the prefix. See [OSS PutObject](https://www.alibabacloud.com/help/en/oss/developer-reference/putobject). The pipeline does not change bucket versioning.

## Distribution scope and retry

Platform archives contain the executable, build.json, SDK metadata, English documents, TOML examples, and required third-party licenses. They exclude Go source, fixtures, acceptance/CI scripts, and Git metadata. Obtain automated acceptance scripts from the matching release-oss-v<version> source tag.

Publication is not a multi-file transaction. A failed run can leave verified partial objects; manifest is withheld until the complete set passes. Retry the same failed job after fixing network/environment issues without changing source. Source/compiler changes need a new version, not a forced tag or overwritten release.

release-report.json and dist/ are retained as CI artifacts for 30 days even on failure. status=verified proves OSS API readback. Also open the public page, fetch manifest and archives through actual hosting, verify digests/revision, install on target servers, and validate disable/rollback and real ingestion.

Pushing main does not publish. Native Windows/IIS and actual telemetry ingestion are separate acceptance requirements.

Local release-logic tests require no live credentials:

~~~sh
node scripts/check-docs.mjs
node --test scripts/ci/ci_test.mjs web/generator_test.mjs scripts/installer_test.mjs scripts/release-oss_test.mjs
go test scripts/release.go scripts/release_test.go
~~~
