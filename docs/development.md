# Development, build, and release validation

Rumable public documentation and setup-page copy are English. Product prose and documentation links use neutral receiver terminology. Runtime SDK bytes, machine-readable source metadata, configuration field names, and third-party license notices retain their upstream identities.

The compatible binary, paths, and markers remain rumable. Documentation branding does not change data transport, component behavior, or the source version.

## Local checks

~~~sh
node scripts/check-docs.mjs
go test ./...
go vet ./...
go build -o rumable ./cmd/rumable
node --test web/*_test.mjs scripts/*_test.mjs scripts/ci/*_test.mjs
go test scripts/sync-sdk.go scripts/sync-sdk_test.go
go test scripts/release.go scripts/release_test.go
~~~

The documentation check covers public Markdown, example comments, setup HTML/catalog strings, relative links and anchors, and generated release-copy templates. It excludes vendored SDK bytes, required license files, machine metadata, and internal authoring skills. The [GitHub release guide](github-release.md) documents download and configuration-page URLs.

## Linux integration

Use an isolated Docker environment with Go and curl available:

~~~sh
bash scripts/smoke-nginx.sh
NGINX_IMAGE=nginx:1.14-alpine bash scripts/smoke-nginx.sh
docker build -f scripts/nginx-1.9.4.Dockerfile -t rumable-nginx:1.9.4 .
NGINX_IMAGE=rumable-nginx:1.9.4 bash scripts/smoke-nginx.sh
bash scripts/smoke-onboarding.sh
bash scripts/smoke-discovery.sh
~~~

The 1.9.4 image is built from pinned official source because old image formats are incompatible with current Docker. `KEEP=1` preserves temporary test resources and prints their cleanup identifiers. Mock intake verifies requests, not real stored events.

Set `RUMABLE_BINARY` to a matching Linux release executable to test that artifact; Go still builds source-only fixtures. Historical runs used Linux ARM64 under OrbStack; do not infer architecture or kernel coverage from a different host.

## Windows validation

Use the archive CLI and the matching source tag's scripts in an isolated administrator Windows VM:

~~~powershell
.\scripts\windows-smoke.ps1 -Binary C:\Rumable\rumable.exe -ExpectedIISVersion 10.0 -ResultDir C:\RumableResults\iis10-01
~~~

The suite covers compression baselines, inheritance, conflicts, lifecycle idempotency, update/rollback, and post-commit failure recovery. Exit codes 0/1/2 mean passed/failed/partial. StaticOnly is never a complete pass. Native IIS, NTFS, PowerShell 5.1, and Agent behavior require Windows; portable PowerShell mocks do not establish those results.

See [Windows acceptance](windows-acceptance.md). Acceptance scripts and ASP.NET fixtures are source-only, not platform-archive contents.

## Release builds

Modern and legacy packages use two actual compilers:

~~~sh
go run scripts/release.go --go /path/to/go1.26.x/bin/go \
  --legacy-go /path/to/go1.20.14/bin/go --version 0.1.9 --skip-sdk-sync \
  --github-repository beacon-observability/rumable
~~~

Prepare SDK updates explicitly with `go run scripts/sync-sdk.go`, review bytes/metadata, and commit before building a release. The standalone builder syncs by default and rejects a dirty revision. Tagged CI uses `--skip-sdk-sync` to rebuild the reviewed bytes without following mutable sources.

An offline candidate can explicitly use `--skip-sdk-sync`. A local uncommitted candidate can use `--revision uncommitted`, which must not be presented as a pinned official release.

The output directory must not already exist. Builds inspect metadata, run tests/vet with both compilers, reject source drift, and produce ten Windows/Linux archives plus installers, setup files, manifest, sdk.json, and SHA256SUMS. build.json records revision, compiler, architecture, and binary digest. Documents, examples, and required third-party notices enter platform archives; source and test fixtures do not.

The [GitHub release workflow](github-release.md) documents the actual upload allowlist and immutable/latest promotion. Compiler examples and historical version records are not promises about untested operating systems.

## Loading-mode regression

~~~sh
node --test web/generator_test.mjs
CDN=1 bash scripts/smoke-attach.sh
KEEP=1 CDN=1 SYNC=1 bash scripts/smoke-attach.sh
~~~

The /loading fixture records SDK state at the first business script; synchronous mode expects initialization beforehand. Inspect browser execution separately, then clean up the printed test resources. IIS script contracts can be exercised with scripts/test-powershell.ps1; final native acceptance remains independent.
