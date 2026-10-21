#!/usr/bin/env bash
set -euo pipefail

# Like browser-sdk, cloudcare-tools is a Shell executor: use Docker explicitly.
if [[ "${GITLAB_CI:-}" != true || "${CI_COMMIT_REF_PROTECTED:-}" != true ]]; then
  echo 'Publishing requires a protected GitLab tag pipeline' >&2
  exit 1
fi
if [[ "${CI_DEBUG_TRACE:-}" == true ]]; then
  echo 'Disable CI_DEBUG_TRACE before using release credentials' >&2
  exit 1
fi
command -v docker >/dev/null || { echo 'The release Runner requires Docker' >&2; exit 1; }
: "${CI_PROJECT_DIR:?Missing CI_PROJECT_DIR}"
: "${CI_JOB_ID:?Missing CI_JOB_ID}"
release_container="rumable-release-${CI_JOB_ID}"
release_image="rumable-ci:${CI_JOB_ID}"
trap 'docker rm -f "$release_container" "${release_container}-powershell" >/dev/null 2>&1 || true; docker image rm "$release_image" >/dev/null 2>&1 || true' EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

# No credentials enter the image or Docker build arguments.
# Run Windows installer contracts in the vendor's complete PowerShell runtime.
# This read-only test container has no network or release credentials and needs
# no apt downloads or cross-distribution runtime/library copies.
docker run --rm --init --name "${release_container}-powershell" \
  --network none --user "$(id -u):$(id -g)" \
  --env HOME=/tmp/rumable-powershell \
  --mount "type=bind,source=${CI_PROJECT_DIR},target=${CI_PROJECT_DIR},readonly" \
  --workdir "$CI_PROJECT_DIR" \
  "${RELEASE_POWERSHELL_IMAGE:-mcr.microsoft.com/powershell:7.4-ubuntu-22.04}" \
  pwsh -NoLogo -NoProfile -File scripts/test-windows-installer.ps1
docker build -t "$release_image" \
  --build-arg "MODERN_GO_IMAGE=${MODERN_GO_IMAGE:-golang:1.26.1-bookworm}" \
  --build-arg "LEGACY_GO_IMAGE=${LEGACY_GO_IMAGE:-golang:1.20.14-bookworm}" \
  --build-arg "RELEASE_NODE_IMAGE=${RELEASE_NODE_IMAGE:-node:22-bookworm}" \
  scripts/ci
docker run --rm --init --name "$release_container" \
  --user "$(id -u):$(id -g)" \
  --mount "type=bind,source=${CI_PROJECT_DIR},target=${CI_PROJECT_DIR}" \
  --workdir "$CI_PROJECT_DIR" \
  --env HOME=/tmp/rumable-ci --env GOPATH=/tmp/rumable-go \
  --env GOCACHE=/tmp/rumable-go-cache --env NPM_CONFIG_CACHE=/tmp/rumable-npm \
  --env "GOPROXY=${GOPROXY:-https://goproxy.cn,direct}" --env "GOSUMDB=${GOSUMDB:-sum.golang.org}" \
  --env GITLAB_CI --env CI_COMMIT_REF_PROTECTED --env CI_COMMIT_TAG --env CI_COMMIT_SHA \
  --env OSS_REGION --env OSS_BUCKET --env OSS_ACCESS_KEY_ID --env OSS_ACCESS_KEY_SECRET --env OSS_STS_TOKEN \
  "$release_image" node scripts/ci/ci.mjs
