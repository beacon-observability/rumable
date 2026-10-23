#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
Usage: bash scripts/release-github.sh [VERSION] [--remote NAME] [--dry-run]

Create and push v<VERSION> for the current committed source.
VERSION defaults to internal/rumable/config.go. The GitHub Actions tag
workflow builds and verifies packages, publishes a GitHub Release, and deploys Pages.

  bash scripts/release-github.sh --dry-run
  bash scripts/release-github.sh

--dry-run checks local and remote state without creating or pushing a tag.
This script does not commit changes, push branches, or overwrite tags.
USAGE
}

fail() { printf 'Error: %s\n' "$*" >&2; exit 1; }
version=''
remote=origin
dry_run=false
while (($#)); do
  case "$1" in
    --dry-run) dry_run=true; shift ;;
    --remote)
      (($# >= 2)) || fail '--remote requires a name'
      remote=$2; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    -*) fail "Unknown option: $1" ;;
    *) [[ -z "$version" ]] || fail 'Only one version is allowed'; version=$1; shift ;;
  esac
done
[[ "$remote" =~ ^[A-Za-z0-9_][A-Za-z0-9_.-]*$ ]] || fail 'Invalid remote name'

root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd -P)
cd -- "$root"
[[ "$(git rev-parse --show-toplevel)" == "$root" ]] || fail 'Run the script from its Rumable repository'
head=$(git rev-parse --verify 'HEAD^{commit}')
require_source() {
  [[ "$(git rev-parse --verify 'HEAD^{commit}')" == "$head" ]] || fail 'HEAD changed; restart the release'
  [[ -z "$(git status --porcelain --untracked-files=normal)" ]] || fail 'Commit or move pending changes before releasing (including untracked files)'
}
require_source
source_version=$(git show HEAD:internal/rumable/config.go | sed -n 's/^var Version = "\([^"]*\)"$/\1/p')
[[ -n "$version" ]] || version=$source_version
[[ "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+(-[a-z0-9]+(\.[a-z0-9]+)*)?$ ]] || fail 'Invalid release version'
[[ "$version" == "$source_version" ]] || fail 'Version must match internal/rumable/config.go'
git cat-file -e HEAD:.github/workflows/release.yml || fail 'Commit .github/workflows/release.yml before releasing'
push_url=$(git remote get-url --push --all "$remote") || fail 'Remote is not configured'
[[ -n "$push_url" && "$push_url" != *$'\n'* ]] || fail 'Release requires exactly one push URL'

[[ "$push_url" == https://github.com/beacon-observability/rumable.git || "$push_url" == git@github.com:beacon-observability/rumable.git ]] || fail 'Release remote must be beacon-observability/rumable on GitHub'

remote_main=$(git ls-remote --heads -- "$push_url" refs/heads/main) || fail 'Cannot inspect remote main'
[[ "${remote_main%%$'\t'*}" == "$head" ]] || fail 'Push this source commit to main and validate the candidate workflow before tagging'

tag="v$version"
ref="refs/tags/$tag"
if git show-ref --verify --quiet "$ref"; then
  [[ "$(git rev-parse "$ref^{commit}")" == "$head" ]] || fail 'Local release tag points to another commit; use a new version'
fi

# Inspect the push destination, which can differ from the fetch URL.
remote_commit() {
  local refs direct peeled
  refs=$(git ls-remote --tags -- "$push_url" "$ref" "$ref^{}") || return 1
  direct=$(printf '%s\n' "$refs" | awk -v ref="$ref" '$2 == ref {print $1}')
  peeled=$(printf '%s\n' "$refs" | awk -v ref="$ref^{}" '$2 == ref {print $1}')
  printf '%s' "${peeled:-$direct}"
}
published=$(remote_commit) || fail 'Cannot inspect the remote release tag; nothing was pushed'
if [[ -n "$published" ]]; then
  [[ "$published" == "$head" ]] || fail 'Remote release tag points to another commit; use a new version'
  printf '%s already points to %s on %s. No tag was changed.\n' "$tag" "$head" "$remote"
  printf 'Check the existing GitHub pipeline; retry its failed job if needed. This does not confirm GitHub Release publication.\n'
  exit 0
fi
printf 'Version: %s\nCommit: %s\nTag: %s\nRemote: %s\n' "$version" "$head" "$tag" "$remote"
if "$dry_run"; then
  printf 'Dry run passed. No tag was created or pushed.\n'
  exit 0
fi

require_source
if ! git show-ref --verify --quiet "$ref"; then
  git tag -a -m "Rumable $version" "$tag" "$head"
fi
[[ "$(git rev-parse "$ref^{commit}")" == "$head" ]] || fail 'Local tag changed; nothing was pushed'
tag_object=$(git rev-parse "$ref")
require_source
# Pin the exact object and push only this tag, even with push.followTags enabled.
if ! git -c push.followTags=false push -- "$push_url" "$tag_object:$ref"; then
  fail 'Tag push failed. The local tag is retained; fix the reported issue and rerun the same command'
fi
published=$(remote_commit) || fail 'Push finished but remote verification failed; rerun to check the tag'
[[ "$published" == "$head" ]] || fail 'Remote release tag did not verify against the selected commit'
printf 'Verified remote tag %s -> %s.\n' "$tag" "$head"
printf 'Check the Release workflow in GitHub Actions for build and GitHub Release upload results. Tag creation alone is not publication success.\n'
