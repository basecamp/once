#!/bin/sh
set -eu

REPO='basecamp/once'
INSTALL_DIR='/usr/local/bin'
IMAGE_REF='{{ .ImageRef }}'
RELEASE_JSON=''
ONCE_BIN=''

# The public keys that sign each release's release.txt. A release installs
# only when one of them verifies it. This must match release-key.pub at the
# root of the once repository byte for byte; installer/main_test.go checks.
RELEASE_KEYS='PLACEHOLDER - this is not a key, and nothing verifies against it.

Replace this whole file with the PEM public key (or keys) that sign
release.txt, and paste the same text into internal/version/release_keys.go
and installer/templates/install.sh. Until then every self-update and every
install from get.once.com refuses to proceed.
'

main() {
  os=$(detect_os)
  arch=$(detect_arch)
  ensure_docker "$os"
  fetch_latest_release
  install_once "$arch"

  if [ "${ONCE_INTERACTIVE:-}" = "false" ]; then
    echo "once is installed. Run 'once' to get started."
    return
  fi

  run_once
}

detect_os() {
  case "$(uname -s)" in
  Linux*) echo "linux" ;;
  Darwin*) echo "darwin" ;;
  *)
    echo "Unsupported OS: $(uname -s)" >&2
    exit 1
    ;;
  esac
}

detect_arch() {
  case "$(uname -m)" in
  x86_64) echo "amd64" ;;
  aarch64) echo "arm64" ;;
  arm64) echo "arm64" ;;
  *)
    echo "Unsupported architecture: $(uname -m)" >&2
    exit 1
    ;;
  esac
}

ensure_docker() {
  os="$1"

  if docker info >/dev/null 2>&1; then
    DOCKER_MODE=normal
    return
  fi

  if command -v docker >/dev/null 2>&1; then
    if [ "$os" = "darwin" ]; then
      echo "Docker Desktop is installed but not running."
      echo "Please start Docker Desktop and try again."
      exit 1
    fi
    DOCKER_MODE=sudo
    return
  fi

  if [ "$os" = "darwin" ]; then
    echo "Docker is required. Install Docker Desktop from"
    echo "https://www.docker.com/products/docker-desktop"
    exit 1
  fi

  echo "Installing Docker..."
  if is_root; then
    sh -c "$(curl -fsSL https://get.docker.com)" >/dev/null 2>&1
  else
    curl -fsSL https://get.docker.com | sudo sh >/dev/null 2>&1
  fi

  if ! is_root; then
    sudo usermod -aG docker "$USER"
  fi

  DOCKER_MODE=sg
}

install_once() {
  arch="$1"

  if command -v once >/dev/null 2>&1; then
    ONCE_BIN=$(command -v once)
    echo "once is already installed at ${ONCE_BIN}"
    return
  fi

  version=$(latest_version)
  echo "Installing once ${version}..."

  binary="once-${os}-${arch}"
  asset_url=$(get_asset_url "$binary")

  if [ -z "$asset_url" ]; then
    echo "Could not find release asset: ${binary}" >&2
    exit 1
  fi

  require_openssl

  manifest_url=$(get_asset_url "release.txt")
  signature_url=$(get_asset_url "release.txt.sig")
  if [ -z "$manifest_url" ] || [ -z "$signature_url" ]; then
    echo "Release ${version} is not signed (it has no release.txt and release.txt.sig), so it was not installed." >&2
    exit 1
  fi

  workdir=$(mktemp -d)
  trap 'rm -rf "$workdir"' EXIT
  download "$manifest_url" "${workdir}/release.txt" "application/octet-stream"
  download "$signature_url" "${workdir}/release.txt.sig" "application/octet-stream"
  download "$asset_url" "${workdir}/${binary}" "application/octet-stream"

  verify_release "$workdir" "$version" "$binary"

  if is_root; then
    install -m 755 "${workdir}/${binary}" "${INSTALL_DIR}/once"
  else
    sudo install -m 755 "${workdir}/${binary}" "${INSTALL_DIR}/once"
  fi
  rm -rf "$workdir"
  trap - EXIT

  ONCE_BIN="${INSTALL_DIR}/once"
  echo "Installed once to ${ONCE_BIN}"

  echo "Installing background service..."
  if is_root; then
    "${ONCE_BIN}" background install
  else
    sudo "${ONCE_BIN}" background install
  fi
}

run_once() {
  install_flag=""
  if [ -n "$IMAGE_REF" ]; then
    install_flag="--install=${IMAGE_REF}"
  fi

  case "$DOCKER_MODE" in
  normal)
    exec "${ONCE_BIN}" ${install_flag} </dev/tty
    ;;
  sudo)
    exec sudo "${ONCE_BIN}" ${install_flag} </dev/tty
    ;;
  sg)
    sg docker -c "${ONCE_BIN} ${install_flag}" </dev/tty
    ;;
  esac
}

# Private

fetch_latest_release() {
  RELEASE_JSON=$(download "https://api.github.com/repos/${REPO}/releases/latest" -)
}

latest_version() {
  echo "$RELEASE_JSON" | awk '/"tag_name"/ { gsub(/.*"tag_name": *"/, ""); gsub(/".*/, ""); print; exit }'
}

get_asset_url() {
  binary="$1"
  echo "$RELEASE_JSON" | awk -v binary="$binary" '
        /"url":.*api\.github\.com.*assets/ {
            u = $0; gsub(/.*"url": *"/, "", u); gsub(/".*/, "", u)
        }
        /"name":/ && index($0, "\"" binary "\"") { print u; exit }
    '
}

require_openssl() {
  if ! command -v openssl >/dev/null 2>&1; then
    echo "once needs openssl to verify the release before installing it, and openssl was not found." >&2
    echo "Install it (for example: sudo apt-get install openssl, or sudo dnf install openssl) and run this again." >&2
    exit 1
  fi
}

# verify_release DIR VERSION BINARY checks that DIR/release.txt is signed by
# one of RELEASE_KEYS, names VERSION, and lists the SHA-256 of DIR/BINARY.
# It exits on any failure, so nothing unverified is installed.
verify_release() {
  dir="$1"
  version="$2"
  binary="$3"

  printf '%s\n' "$RELEASE_KEYS" | awk -v dir="$dir" '
    /^-----BEGIN PUBLIC KEY-----$/ { n++; file = dir "/release-key-" n ".pem" }
    file { print > file }
    /^-----END PUBLIC KEY-----$/ { close(file); file = "" }
  '

  verified=''
  for key in "$dir"/release-key-*.pem; do
    [ -f "$key" ] || continue
    if openssl dgst -sha256 -verify "$key" -signature "${dir}/release.txt.sig" "${dir}/release.txt" >/dev/null 2>&1; then
      verified=1
      break
    fi
  done
  if [ -z "$verified" ]; then
    echo "The signature on release ${version} did not verify against the once release key, so it was not installed." >&2
    exit 1
  fi

  if [ "$(sed -n 1p "${dir}/release.txt")" != "once ${version}" ]; then
    echo "The signed release.txt is not for ${version}, so it was not installed." >&2
    exit 1
  fi

  expected=$(awk -v binary="$binary" 'NR > 2 && NF == 2 && $2 == binary { print $1; exit }' "${dir}/release.txt")
  actual=$(sha256 "${dir}/${binary}")
  if [ -z "$expected" ] || [ "$expected" != "$actual" ]; then
    echo "The downloaded ${binary} does not match the signed checksum, so it was not installed." >&2
    exit 1
  fi
}

sha256() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{ print $1 }'
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$1" | awk '{ print $1 }'
  else
    openssl dgst -sha256 "$1" | awk '{ print $NF }'
  fi
}

download() {
  url="$1"
  output="$2"
  accept="${3:-}"

  if command -v curl >/dev/null 2>&1; then
    set -- -fsSL -o "$output"
    [ -n "${GITHUB_TOKEN:-}" ] && set -- "$@" -H "Authorization: token ${GITHUB_TOKEN}"
    [ -n "$accept" ] && set -- "$@" -H "Accept: $accept"
    curl "$@" "$url"
  elif command -v wget >/dev/null 2>&1; then
    set -- -q -O "$output"
    [ -n "${GITHUB_TOKEN:-}" ] && set -- "$@" --header="Authorization: token ${GITHUB_TOKEN}"
    [ -n "$accept" ] && set -- "$@" --header="Accept: $accept"
    wget "$@" "$url"
  else
    echo "curl or wget is required" >&2
    exit 1
  fi
}

is_root() {
  [ "$(id -u)" -eq 0 ]
}

main
