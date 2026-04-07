#!/usr/bin/env bash
set -euo pipefail

REPO="thevibeworks/ccx"
BINARY="ccx"
SKILL_NAME="ccx"

BIN_DIR="${HOME}/.local/bin"
SKILL_DIR="${HOME}/.claude/skills/${SKILL_NAME}"
INSTALL_MODE="all"
USE_SUDO=false

usage() {
  cat <<EOF
Install ccx - session viewer for Claude Code & Codex

Usage: install.sh [OPTIONS]

Options:
  --system        Install binary to /usr/local/bin (requires sudo)
  --bin-only      Install only the binary
  --skill-only    Install only the Claude Code skill
  --bin-dir DIR   Custom binary directory (default: ~/.local/bin)
  --skill-dir DIR Custom skill directory (default: ~/.claude/skills/ccx)
  -h, --help      Show this help

Examples:
  curl -fsSL https://raw.githubusercontent.com/${REPO}/main/install.sh | bash
  curl -fsSL https://raw.githubusercontent.com/${REPO}/main/install.sh | bash -s -- --system
EOF
  exit 0
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --system) USE_SUDO=true; BIN_DIR="/usr/local/bin"; shift ;;
    --bin-only) INSTALL_MODE="bin"; shift ;;
    --skill-only) INSTALL_MODE="skill"; shift ;;
    --bin-dir) BIN_DIR="$2"; shift 2 ;;
    --skill-dir) SKILL_DIR="$2"; shift 2 ;;
    -h|--help) usage ;;
    *) echo "Unknown option: $1"; usage ;;
  esac
done

detect_platform() {
  local os arch
  os=$(uname -s | tr '[:upper:]' '[:lower:]')
  arch=$(uname -m)

  case "$os" in
    darwin) os="macOS" ;;
    linux) os="linux" ;;
    *) echo "Unsupported OS: $os"; exit 1 ;;
  esac

  case "$arch" in
    x86_64|amd64) arch="x86_64" ;;
    arm64|aarch64) arch="arm64" ;;
    *) echo "Unsupported architecture: $arch"; exit 1 ;;
  esac

  echo "${os}_${arch}"
}

get_latest_version() {
  curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
    | grep '"tag_name"' \
    | head -1 \
    | sed 's/.*"tag_name": *"v\([^"]*\)".*/\1/'
}

install_binary() {
  local platform version archive_name url tmp_dir

  platform=$(detect_platform)
  echo "Detected platform: ${platform}"

  echo "Fetching latest release..."
  version=$(get_latest_version)
  if [[ -z "$version" ]]; then
    echo "Failed to determine latest version. Falling back to go install..."
    install_via_go
    return
  fi
  echo "Latest version: v${version}"

  archive_name="${BINARY}_${version}_${platform}.tar.gz"
  url="https://github.com/${REPO}/releases/download/v${version}/${archive_name}"

  tmp_dir=$(mktemp -d)
  trap "rm -rf ${tmp_dir}" EXIT

  echo "Downloading ${url}..."
  if ! curl -fsSL -o "${tmp_dir}/${archive_name}" "$url"; then
    echo "Download failed. Falling back to go install..."
    install_via_go
    return
  fi

  echo "Extracting..."
  tar -xzf "${tmp_dir}/${archive_name}" -C "${tmp_dir}"

  local binary_path
  binary_path=$(find "${tmp_dir}" -name "${BINARY}" -type f | head -1)
  if [[ -z "$binary_path" ]]; then
    echo "Binary not found in archive"
    exit 1
  fi

  mkdir -p "${BIN_DIR}"
  if $USE_SUDO; then
    sudo install -m 755 "$binary_path" "${BIN_DIR}/${BINARY}"
  else
    install -m 755 "$binary_path" "${BIN_DIR}/${BINARY}"
  fi

  echo "Installed ${BINARY} to ${BIN_DIR}/${BINARY}"
}

install_via_go() {
  if ! command -v go &>/dev/null; then
    echo "Go is not installed. Install Go from https://go.dev/dl/ or download a prebuilt binary."
    exit 1
  fi
  echo "Installing via go install..."
  go install "github.com/${REPO}/cmd/${BINARY}@latest"
  echo "Installed via go install"
}

install_skill() {
  local base_url
  base_url="https://raw.githubusercontent.com/${REPO}/main/skills/${SKILL_NAME}"

  mkdir -p "${SKILL_DIR}"

  echo "Installing skill to ${SKILL_DIR}..."

  # If running from repo checkout, copy locally
  local script_dir
  script_dir=$(cd "$(dirname "$0")" && pwd)
  if [[ -f "${script_dir}/skills/${SKILL_NAME}/SKILL.md" ]]; then
    cp -r "${script_dir}/skills/${SKILL_NAME}/"* "${SKILL_DIR}/"
    echo "Installed skill from local repo"
    return
  fi

  # Download from GitHub
  curl -fsSL -o "${SKILL_DIR}/SKILL.md" "${base_url}/SKILL.md"
  echo "Installed skill from GitHub"
}

main() {
  echo ""
  echo "  ccx installer"
  echo "  ─────────────"
  echo ""

  if [[ "$INSTALL_MODE" != "skill" ]]; then
    install_binary
  fi

  if [[ "$INSTALL_MODE" != "bin" ]]; then
    install_skill
  fi

  echo ""
  echo "  Done! Run 'ccx --help' to get started."

  # Check PATH
  if [[ "$INSTALL_MODE" != "skill" ]] && ! echo "$PATH" | tr ':' '\n' | grep -q "^${BIN_DIR}$"; then
    echo ""
    echo "  Note: ${BIN_DIR} is not in your PATH."
    echo "  Add this to your shell profile:"
    echo "    export PATH=\"${BIN_DIR}:\$PATH\""
  fi

  echo ""
}

main
