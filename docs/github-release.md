# GitHub releases

Rumable is part of Beacon Observability and keeps the `rumable` executable and existing configuration format.

- [Latest release and downloads](https://github.com/beacon-observability/rumable/releases/latest)
- [English configuration page](https://beacon-observability.github.io/rumable/latest/setup/index.html)
- [Latest configuration manifest](https://beacon-observability.github.io/rumable/latest/manifest.json)
- [v0.1.9 release notes](releases/0.1.9.md)

## Download and verification

All executable packages, installation scripts, `manifest.json`, `sdk.json`, and `SHA256SUMS` are GitHub Release assets. Configuration pages are hosted on GitHub Pages; their manifests pin downloads to an explicit GitHub Release tag. The page runs locally in the browser and sends configuration only when the generated command is executed on the target server.

For example, download and verify Linux AMD64 0.1.9:

```sh
base=https://github.com/beacon-observability/rumable/releases/download/v0.1.9
curl --proto '=https' --proto-redir '=https' -fsSL "$base/rumable_0.1.9_linux_amd64.tar.gz" -o rumable_0.1.9_linux_amd64.tar.gz
curl --proto '=https' --proto-redir '=https' -fsSL "$base/SHA256SUMS" -o SHA256SUMS
sha256sum -c SHA256SUMS --ignore-missing
mkdir rumable-0.1.9
tar -xzf rumable_0.1.9_linux_amd64.tar.gz -C rumable-0.1.9
./rumable-0.1.9/rumable version
```

Use the package for your OS and architecture from the release assets. Modern builds use Go 1.26.4; legacy builds use Go 1.20.14. Review [platform compatibility](compatibility.md) before selecting legacy packages. GitHub redirects asset downloads; command-line clients must follow HTTPS redirects. Browser configuration pages load their same-origin Pages manifest rather than fetching a GitHub Release asset through browser CORS.

`SHA256SUMS` covers archives, installers, metadata, and configuration-page files. Page files retain their `setup/` paths on Pages and in the Actions artifact; they are not flattened into release assets. `build.json` inside each archive records the source revision, compiler, platform, flavor, and executable digest. Package documentation and licenses remain inside archives.

## Prepare a version

Release titles and the first heading in release notes use `v<version>` (for example, `v0.1.9`).

1. Set `Version` in `internal/rumable/config.go`, update the release builder default, add `docs/releases/<version>.md`, and update `CHANGELOG.md` and current download examples. Include new release documents in the builder's explicit support-file list.
2. If changing the SDK, run `go run scripts/sync-sdk.go`, review SDK bytes and metadata, and commit them. Tagged builds use the committed SDK with `--skip-sdk-sync`.
3. Run the checks in [development](development.md), commit, and push `main`.
4. Run the Release workflow manually from `main` for a complete candidate build. Manual runs build packages and run Linux/Windows validation without publishing.
5. After it passes, create the matching tag:

```sh
bash scripts/release-github.sh --dry-run
bash scripts/release-github.sh
```

The helper checks the clean committed version, intended GitHub remote, and existing local/remote tags. It pushes only `v<version>` and never replaces an existing tag. Source branches must already be pushed. Run `gh run list --workflow release.yml` to follow progress.

## Publication and retry

The workflow builds ten Linux/Windows archives using isolated Go 1.26.4 and Go 1.20.14 compilers, tests both compilers, checks source revision and artifact hashes, exercises the packaged Linux executable with Nginx, and checks Windows AMD64 modern/legacy executables on Windows Server 2022. PowerShell regression tests cover installation and IIS contracts; they do not replace native IIS acceptance or prove all older Windows versions.

Publication starts after build and Windows checks pass. It creates a draft, uploads the fifteen root assets, downloads them again, verifies every digest, publishes the release, and verifies anonymous HTTPS downloads. Setup pages deploy separately with the official Pages Actions. Older published stable pages are rebuilt from their tagged sources and checked against their published checksums before redeployment.

Latest aliases select the highest published stable version, so retrying an older release cannot move them backward. Identical retry uploads are accepted. Different bytes, unexpected assets, incomplete already-published releases, or another source revision fail without overwriting assets. If a draft upload or Pages deployment fails, fix credentials or infrastructure and rerun the failed jobs. When the publishing script itself needs a fix, use the **Resume release** workflow from current main with the existing tag and original Release run ID. It verifies the tag revision and successful build/Windows jobs, reuses the checked artifact without rebuilding, and applies the current publication code. For example: `gh workflow run resume-release.yml -f tag=v0.1.9 -f run_id=REPLACE_RUN_ID`. If source code needs correction, publish a new patch version rather than replacing an already-published release or moving its tag. A Pages deployment failure leaves the GitHub Release downloads available.

Repository administrators must enable GitHub Actions and set Pages to **GitHub Actions**. Allow `v*` tags in the `github-pages` environment deployment rules so tagged releases can deploy. Build jobs use read permissions; only the publication job can write releases, and only the Pages job receives Pages/OIDC deployment permissions. No OSS secrets are needed.

## Existing GitLab publication

The existing [GitLab/OSS workflow](gitlab-ci-release.md) remains available for existing deployments. Its separate trigger is `release-oss-v<version>`. Select the GitLab remote explicitly with `bash scripts/release-oss.sh --remote gitlab`; the GitHub remote uses `scripts/release-github.sh`.
