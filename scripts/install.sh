#!/bin/sh
# Release packaging replaces the version and archive hashes below.
# Keep execution at the end so a truncated function body cannot start setup.
main() {
  set -eu
  fail() { printf '%s\n' "Rumable: $*" >&2; exit 1; }
  version='@@VERSION@@'
  base_url= config=
  while [ "$#" -gt 0 ]; do
    case "$1" in
      --base-url|--config)
        [ "$#" -ge 2 ] || fail "Missing value for $1"
        case "$1" in --base-url) base_url=$2 ;; --config) config=$2 ;; esac
        shift 2 ;;
      *) fail "Unknown argument: $1" ;;
    esac
  done
  [ -n "$config" ] || fail 'Missing --config (Base64 TOML)'
  case "$base_url" in https://?*) ;; *) fail '--base-url must use HTTPS' ;; esac
  case "$base_url" in *[[:space:]]*|*'?'*|*'#'*|*'@'*|*'\'*) fail 'Invalid --base-url' ;; esac
  [ "$(id -u)" = 0 ] || fail 'Run with root or sudo'
  [ "$(uname -s)" = Linux ] || fail 'This installer requires Linux'
  case "$(uname -m)" in
    i386|i486|i586|i686) arch=386; digest='@@LINUX_386_SHA256@@' ;;
    x86_64) arch=amd64; digest='@@LINUX_AMD64_SHA256@@' ;;
    aarch64|arm64) arch=arm64; digest='@@LINUX_ARM64_SHA256@@' ;;
    *) fail 'Supported Linux architectures: x86, x64 and ARM64' ;;
  esac
  case "$version$digest" in *'@@'*) fail 'Use the CI-published installer, not the source template' ;; esac
  # A cached latest installer must still fetch the archive pinned in this script.
  base_url=${base_url%/}
  case "$base_url" in */latest) base_url="${base_url%/latest}/$version" ;; esac
  for tool in curl sha256sum tar base64 cmp install mktemp; do
    command -v "$tool" >/dev/null 2>&1 || fail "Missing required tool: $tool"
  done
  umask 077
  work=$(mktemp -d)
  trap 'rm -rf "$work"' EXIT
  trap 'exit 130' INT
  trap 'exit 143' TERM
  printf '%s' "$config" | base64 -d > "$work/rumable.toml" || fail 'Invalid Base64 configuration'
  [ -s "$work/rumable.toml" ] || fail 'Empty configuration'
  curl --proto '=https' --proto-redir '=https' --tlsv1.2 -fsSL \
    "${base_url%/}/rumable_${version}_linux_${arch}.tar.gz" -o "$work/package.tar.gz"
  printf '%s  %s\n' "$digest" "$work/package.tar.gz" | sha256sum -c -
  tar -xzf "$work/package.tar.gz" -C "$work" rumable
  bin_dir="/opt/rumable/$version"
  bin="$bin_dir/rumable"
  mkdir -p "$bin_dir"
  if [ -e "$bin" ]; then
    cmp "$work/rumable" "$bin" || fail 'Installed version has different bytes'
  else
    install -m 755 "$work/rumable" "$bin"
  fi
  # Public SDK directories need to be traversable by the web server. The CLI
  # itself creates private state/config files with restrictive permissions.
  umask 022
  "$bin" setup --config "$work/rumable.toml" --state-dir /var/lib/rumable --yes
}

main "$@"
