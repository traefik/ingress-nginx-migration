#!/usr/bin/env bash

set -e

APP_NAME="ingress-nginx-migration"
REPO_OWNER="traefik"
REPO_NAME="ingress-nginx-migration"
GITHUB_RELEASES_URL="https://github.com/${REPO_OWNER}/${REPO_NAME}/releases"

: "${USE_SUDO:=true}"
: "${INSTALL_DIR:=/usr/local/bin}"

HAS_CURL="$(type "curl" &> /dev/null && echo true || echo false)"
HAS_WGET="$(type "wget" &> /dev/null && echo true || echo false)"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

info() {
  echo -e "${GREEN}[INFO]${NC} $1"
}

warn() {
  echo -e "${YELLOW}[WARN]${NC} $1"
}

error() {
  echo -e "${RED}[ERROR]${NC} $1"
}

showHelp() {
  echo "Usage: $0 [options]"
  echo ""
  echo "Options:"
  echo "  --no-sudo    Install without sudo (installs to ~/bin)"
  echo "  --help       Show this help message"
  echo ""
  echo "Environment variables:"
  echo "  TAG          Version to install (e.g., v0.0.1). Default: latest"
  echo "  INSTALL_DIR  Installation directory. Default: /usr/local/bin"
  echo "  USE_SUDO     Use sudo for installation. Default: true"
  echo ""
  echo "Examples:"
  echo "  # Install latest version"
  echo "  curl -sSL https://raw.githubusercontent.com/${REPO_OWNER}/${REPO_NAME}/main/scripts/install.sh | bash"
  echo ""
  echo "  # Install specific version"
  echo "  curl -sSL https://raw.githubusercontent.com/${REPO_OWNER}/${REPO_NAME}/main/scripts/install.sh | TAG=v0.0.1 bash"
  echo ""
  echo "  # Install without sudo"
  echo "  curl -sSL https://raw.githubusercontent.com/${REPO_OWNER}/${REPO_NAME}/main/scripts/install.sh | bash -s -- --no-sudo"
}

initArch() {
  ARCH=$(uname -m)
  case $ARCH in
    armv5*) ARCH="armv5" ;;
    armv6*) ARCH="armv6" ;;
    armv7*) ARCH="arm" ;;
    aarch64) ARCH="arm64" ;;
    x86) ARCH="386" ;;
    x86_64) ARCH="amd64" ;;
    i686) ARCH="386" ;;
    i386) ARCH="386" ;;
  esac
}

initOS() {
  OS=$(uname | tr '[:upper:]' '[:lower:]')
  case "$OS" in
    darwin) OS='darwin' ;;
    linux) OS='linux' ;;
    msys*|mingw*|cygwin*) OS='windows' ;;
    *)
      error "Unsupported OS: $OS"
      exit 1
      ;;
  esac

  # Windows specific adjustments
  if [ "$OS" = "windows" ]; then
    USE_SUDO="false"
    INSTALL_DIR="${INSTALL_DIR:-$HOME/bin}"
    BINARY_EXT=".exe"
    ARCHIVE_EXT="zip"
  else
    BINARY_EXT=""
    ARCHIVE_EXT="tar.gz"
  fi
}

verifySupported() {
  local supported="darwin-amd64\ndarwin-arm64\nlinux-amd64\nlinux-arm64\nwindows-amd64\nwindows-arm64"
  if ! echo -e "$supported" | grep -q "${OS}-${ARCH}"; then
    error "Unsupported platform: ${OS}-${ARCH}"
    error "Supported platforms:"
    echo -e "$supported" | sed 's/^/  - /'
    exit 1
  fi

  if [ "$HAS_CURL" != "true" ] && [ "$HAS_WGET" != "true" ]; then
    error "Either curl or wget is required to download files"
    exit 1
  fi
}

getLatestRelease() {
  if [ -n "$TAG" ]; then
    info "Using specified version: $TAG"
    return
  fi

  info "Fetching latest release..."
  local latest_release_url="${GITHUB_RELEASES_URL}/latest"

  if [ "$HAS_CURL" = "true" ]; then
    TAG=$(curl -sI "$latest_release_url" | grep -i "^location:" | sed 's/.*tag\///' | tr -d '\r\n')
  elif [ "$HAS_WGET" = "true" ]; then
    TAG=$(wget -qO- --server-response "$latest_release_url" 2>&1 | grep -i "Location:" | tail -1 | sed 's/.*tag\///' | tr -d '\r\n')
  fi

  if [ -z "$TAG" ]; then
    error "Failed to fetch latest release tag"
    exit 1
  fi

  info "Latest version: $TAG"
}

downloadFile() {
  ARCHIVE_NAME="${APP_NAME}-${TAG}-${OS}-${ARCH}.${ARCHIVE_EXT}"
  DOWNLOAD_URL="${GITHUB_RELEASES_URL}/download/${TAG}/${ARCHIVE_NAME}"
  CHECKSUM_URL="${GITHUB_RELEASES_URL}/download/${TAG}/${APP_NAME}_checksums.txt"

  TMP_DIR=$(mktemp -d)
  trap cleanup EXIT

  info "Downloading ${ARCHIVE_NAME}..."

  if [ "$HAS_CURL" = "true" ]; then
    curl -fsSL "$DOWNLOAD_URL" -o "${TMP_DIR}/${ARCHIVE_NAME}"
    curl -fsSL "$CHECKSUM_URL" -o "${TMP_DIR}/checksums.txt"
  elif [ "$HAS_WGET" = "true" ]; then
    wget -q "$DOWNLOAD_URL" -O "${TMP_DIR}/${ARCHIVE_NAME}"
    wget -q "$CHECKSUM_URL" -O "${TMP_DIR}/checksums.txt"
  fi
}

verifyChecksum() {
  info "Verifying checksum..."

  local expected_checksum
  expected_checksum=$(grep "${ARCHIVE_NAME}" "${TMP_DIR}/checksums.txt" | awk '{print $1}')

  if [ -z "$expected_checksum" ]; then
    error "Could not find checksum for ${ARCHIVE_NAME}"
    exit 1
  fi

  local actual_checksum
  if command -v sha256sum &> /dev/null; then
    actual_checksum=$(sha256sum "${TMP_DIR}/${ARCHIVE_NAME}" | awk '{print $1}')
  elif command -v shasum &> /dev/null; then
    actual_checksum=$(shasum -a 256 "${TMP_DIR}/${ARCHIVE_NAME}" | awk '{print $1}')
  else
    warn "sha256sum/shasum not found, skipping checksum verification"
    return
  fi

  if [ "$expected_checksum" != "$actual_checksum" ]; then
    error "Checksum verification failed!"
    error "Expected: $expected_checksum"
    error "Actual:   $actual_checksum"
    exit 1
  fi

  info "Checksum verified successfully"
}

installFile() {
  info "Extracting archive..."

  if [ "$ARCHIVE_EXT" = "zip" ]; then
    unzip -q "${TMP_DIR}/${ARCHIVE_NAME}" -d "${TMP_DIR}"
  else
    tar -xzf "${TMP_DIR}/${ARCHIVE_NAME}" -C "${TMP_DIR}"
  fi

  # Find the binary (it's in a subdirectory after extraction)
  local binary_path
  binary_path=$(find "${TMP_DIR}" -name "${APP_NAME}${BINARY_EXT}" -type f | head -1)

  if [ -z "$binary_path" ]; then
    error "Could not find ${APP_NAME}${BINARY_EXT} in extracted archive"
    exit 1
  fi

  chmod +x "$binary_path"

  info "Installing to ${INSTALL_DIR}/${APP_NAME}${BINARY_EXT}..."

  # Create install directory if it doesn't exist
  if [ "$USE_SUDO" = "true" ]; then
    sudo mkdir -p "$INSTALL_DIR"
    sudo mv "$binary_path" "${INSTALL_DIR}/${APP_NAME}${BINARY_EXT}"
  else
    mkdir -p "$INSTALL_DIR"
    mv "$binary_path" "${INSTALL_DIR}/${APP_NAME}${BINARY_EXT}"
  fi
}

testVersion() {
  info "Verifying installation..."

  if ! command -v "${APP_NAME}" &> /dev/null; then
    # Check if it's in the install dir but not in PATH
    if [ -x "${INSTALL_DIR}/${APP_NAME}${BINARY_EXT}" ]; then
      warn "${APP_NAME} installed to ${INSTALL_DIR} but not in PATH"
      warn "Add the following to your shell profile:"
      warn "  export PATH=\"\$PATH:${INSTALL_DIR}\""
      return
    fi
    error "Installation failed: ${APP_NAME} not found"
    exit 1
  fi

  local installed_version
  installed_version=$("${APP_NAME}" version 2>/dev/null || echo "unknown")
  info "Successfully installed ${APP_NAME} ${installed_version}"
}

cleanup() {
  if [ -n "$TMP_DIR" ] && [ -d "$TMP_DIR" ]; then
    rm -rf "$TMP_DIR"
  fi
}

main() {
  # Parse arguments
  while [ $# -gt 0 ]; do
    case "$1" in
      --no-sudo)
        USE_SUDO="false"
        INSTALL_DIR="${HOME}/bin"
        shift
        ;;
      --help|-h)
        showHelp
        exit 0
        ;;
      *)
        error "Unknown option: $1"
        showHelp
        exit 1
        ;;
    esac
  done

  echo ""
  echo "  ${APP_NAME} installer"
  echo "  ========================"
  echo ""

  initArch
  initOS
  verifySupported
  getLatestRelease
  downloadFile
  verifyChecksum
  installFile
  testVersion

  echo ""
  info "Installation complete!"
  echo ""
}

main "$@"
