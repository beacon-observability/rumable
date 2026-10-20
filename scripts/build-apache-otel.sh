#!/usr/bin/env bash
# Rebuild the small Apache shim against the target Apache headers and official SDK.
set -euo pipefail
cd "$(dirname "$0")/.."
if [ "$#" != 3 ];then echo 'Usage: build-apache-otel.sh SOURCE_MODULE_DIR SDK_DIR NEW_OUTPUT_DIR' >&2;exit 1;fi
source_dir=$1; sdk_dir=$2; output=$3
license="$source_dir/../../LICENSE"
test -f "$license" || { echo 'Source checkout LICENSE is required (use the full official source archive)' >&2;exit 1; }
test ! -e "$output" || { echo 'Output already exists; refusing overwrite' >&2;exit 1; }
work=$(mktemp -d "${TMPDIR:-/tmp}/rumable-apache-build.XXXXXX")
trap 'rm -rf "$work"' EXIT
mkdir "$work/source"
cp -R "$source_dir/src" "$source_dir/include" "$work/source/"
cp -R "$sdk_dir" "$work/sdk"
cp "$license" "$work/sdk/UPSTREAM-LICENSE"
go run -buildvcs=false scripts/patch-apache-otel.go "$work/source/src/apache/ApacheHooks.cpp"
cat > "$work/Dockerfile" <<'DOCKER'
ARG APACHE_IMAGE=httpd:2.4
FROM ${APACHE_IMAGE} AS build
USER root
RUN apt-get update && apt-get install -y --no-install-recommends g++ libapr1-dev libaprutil1-dev && rm -rf /var/lib/apt/lists/*
COPY source /source
COPY sdk /opt/otel
RUN g++ -D_GLIBCXX_USE_CXX11_ABI=0 -std=c++11 -O2 -shared -fPIC -fvisibility=hidden -fvisibility-inlines-hidden \
    -I/usr/local/apache2/include -I/usr/include/apr-1.0 \
    -I/source/include/core -I/source/include/util -I/source/include/apache \
    /source/src/apache/*.cpp -L/opt/otel/sdk_lib/lib -lopentelemetry_webserver_sdk \
    -o /opt/otel/WebServerModule/Apache/libmod_apache_otel.so \
 && printf 'rumable-apache-isolation-v1; upstream webserver/v1.0.4; target Apache headers from %s\n' "$(httpd -v | head -1)" > /opt/otel/RUMABLE-ISOLATION.txt \
 && cd /opt/otel && find . -type f ! -name SHA256SUMS -exec sha256sum {} + > SHA256SUMS
FROM scratch
COPY --from=build /opt/otel /otel
DOCKER
docker build --platform linux/amd64 --build-arg "APACHE_IMAGE=${APACHE_IMAGE:-httpd:2.4}" --output "type=local,dest=$output" "$work"
echo "Patched x64 SDK: $output/otel. Verify SHA256SUMS, install with its SDK libraries on the loader path, then run the Apache OTel smoke test before production."
