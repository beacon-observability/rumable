ARG MODERN_GO_IMAGE=golang:1.26.1-bookworm
ARG LEGACY_GO_IMAGE=golang:1.20.14-bookworm
ARG RELEASE_NODE_IMAGE=node:22-bookworm
FROM ${LEGACY_GO_IMAGE} AS legacy
FROM ${RELEASE_NODE_IMAGE} AS node
FROM ${MODERN_GO_IMAGE}
COPY --from=legacy /usr/local/go /opt/go-legacy
COPY --from=node /usr/local/bin/node /usr/local/bin/node
COPY --from=node /usr/local/lib/node_modules/npm /usr/local/lib/node_modules/npm
RUN ln -s ../lib/node_modules/npm/bin/npm-cli.js /usr/local/bin/npm
