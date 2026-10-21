ARG MODERN_GO_IMAGE=golang:1.26.1-bookworm
ARG LEGACY_GO_IMAGE=golang:1.20.14-bookworm
ARG RELEASE_NODE_IMAGE=node:22-bookworm
ARG RELEASE_POWERSHELL_IMAGE=mcr.microsoft.com/powershell:7.4-ubuntu-22.04
FROM ${RELEASE_POWERSHELL_IMAGE} AS powershell
FROM ${LEGACY_GO_IMAGE} AS legacy
FROM ${RELEASE_NODE_IMAGE} AS node
FROM ${MODERN_GO_IMAGE}
RUN apt-get update && apt-get install -y --no-install-recommends libicu72 && rm -rf /var/lib/apt/lists/*
COPY --from=powershell /opt/microsoft/powershell/7 /opt/microsoft/powershell/7
RUN ln -s /opt/microsoft/powershell/7/pwsh /usr/local/bin/pwsh
COPY --from=legacy /usr/local/go /opt/go-legacy
COPY --from=node /usr/local/bin/node /usr/local/bin/node
COPY --from=node /usr/local/lib/node_modules/npm /usr/local/lib/node_modules/npm
RUN ln -s ../lib/node_modules/npm/bin/npm-cli.js /usr/local/bin/npm
