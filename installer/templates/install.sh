#!/bin/sh
set -eu

REPO='basecamp/once'
INSTALL_DIR='/usr/local/bin'
IMAGE_REF='{{ .ImageRef }}'
RELEASE_JSON=''
ONCE_BIN=''

main() {
  os=$(detect_os)
  arch=$(detect_arch)
  ensure_docker "$os"
  fetch_latest_release
  install_once "$arch"
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

  tmpfile=$(mktemp)
  download "$asset_url" "$tmpfile" "application/octet-stream"

  if is_root; then
    install -m 755 "$tmpfile" "${INSTALL_DIR}/once"
  else
    sudo install -m 755 "$tmpfile" "${INSTALL_DIR}/once"
  fi
  rm -f "$tmpfile"

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
