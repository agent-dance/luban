#!/bin/sh

set -eu

REPOSITORY="agent-dance/luban"
PROGRAM="luban"
ASSET_PREFIX="luban-code"
VERSION="latest"
INSTALL_DIR="${XDG_BIN_HOME:-${HOME}/.local/bin}"
UNINSTALL=false

usage() {
  cat <<'EOF'
Install LUBAN Code from an official GitHub release.

Usage:
  install.sh [--version <version>] [--install-dir <directory>]
  install.sh --uninstall [--install-dir <directory>]
  install.sh --help

Options:
  --version       Release tag to install (for example, v0.1.0). Default: latest.
  --install-dir   Destination directory. Default: $XDG_BIN_HOME or ~/.local/bin.
  --uninstall     Remove luban from the destination directory.
  -h, --help      Show this help.

The installer never invokes sudo. It verifies the release SHA-256 checksum
before replacing an existing installation.
EOF
}

die() {
  printf 'Error: %s\n' "$*" >&2
  exit 1
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --version)
      [ "$#" -ge 2 ] || die "--version requires a value"
      VERSION=$2
      shift 2
      ;;
    --install-dir)
      [ "$#" -ge 2 ] || die "--install-dir requires a value"
      INSTALL_DIR=$2
      shift 2
      ;;
    --uninstall)
      UNINSTALL=true
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      die "unknown option: $1 (run with --help for usage)"
      ;;
  esac
done

case "$INSTALL_DIR" in
  /*) ;;
  *) die "--install-dir must be an absolute path: $INSTALL_DIR" ;;
esac

TARGET="$INSTALL_DIR/$PROGRAM"
LEGACY_TARGET="$INSTALL_DIR/luban-code"

if [ "$UNINSTALL" = true ]; then
  if [ ! -e "$TARGET" ] && [ ! -e "$LEGACY_TARGET" ]; then
    printf '%s is not installed at %s\n' "$PROGRAM" "$TARGET"
    exit 0
  fi
  if [ -e "$TARGET" ]; then
    [ -f "$TARGET" ] || die "refusing to remove non-file path: $TARGET"
  fi
  if [ -e "$LEGACY_TARGET" ]; then
    [ -f "$LEGACY_TARGET" ] || die "refusing to remove non-file path: $LEGACY_TARGET"
  fi
  [ -w "$INSTALL_DIR" ] || die "installation directory is not writable: $INSTALL_DIR"
  rm -f -- "$TARGET"
  if [ -f "$LEGACY_TARGET" ]; then
    rm -f -- "$LEGACY_TARGET"
  fi
  printf 'Removed %s\n' "$TARGET"
  exit 0
fi

command -v curl >/dev/null 2>&1 || die "curl is required to download releases"
command -v tar >/dev/null 2>&1 || die "tar is required to unpack the release"

OS=${LUBAN_INSTALL_OS:-$(uname -s)}
ARCH=${LUBAN_INSTALL_ARCH:-$(uname -m)}

case "$OS" in
  Darwin|Linux) ;;
  *) die "unsupported operating system: $OS (supported: macOS and Linux)" ;;
esac

case "$ARCH" in
  x86_64|amd64) ARCH=x86_64 ;;
  arm64|aarch64) ARCH=arm64 ;;
  *) die "unsupported architecture: $ARCH (supported: x86_64 and arm64)" ;;
esac

RELEASES_URL=${LUBAN_INSTALL_RELEASES_URL:-"https://github.com/$REPOSITORY/releases"}
if [ "$VERSION" = latest ]; then
  if [ -n "${LUBAN_INSTALL_LATEST_VERSION:-}" ]; then
    VERSION=$LUBAN_INSTALL_LATEST_VERSION
  else
    latest_url=$(curl -fsSL -o /dev/null -w '%{url_effective}' "$RELEASES_URL/latest") || \
      die "could not resolve the latest release from $RELEASES_URL/latest"
    VERSION=${latest_url##*/}
    [ -n "$VERSION" ] && [ "$VERSION" != latest ] || \
      die "the latest release did not resolve to a version tag"
  fi
fi

if ! printf '%s\n' "$VERSION" | awk '
  /^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z.-]+)?$/ { valid = 1 }
  END { exit !valid }
'; then
  die "invalid release version '$VERSION'; expected a tag such as v0.1.0"
fi

ARCHIVE="${ASSET_PREFIX}_${OS}_${ARCH}.tar.gz"
DOWNLOAD_ROOT="$RELEASES_URL/download/$VERSION"
TMP_DIR=$(mktemp -d "${TMPDIR:-/tmp}/luban-install.XXXXXX") || die "could not create a temporary directory"
STAGED=
cleanup() {
  if [ -n "$STAGED" ]; then
    rm -f -- "$STAGED"
  fi
  rm -rf -- "$TMP_DIR"
}
trap cleanup EXIT
trap 'exit 1' HUP INT TERM

printf 'Downloading %s %s for %s/%s...\n' "$PROGRAM" "$VERSION" "$OS" "$ARCH"
curl -fsSL --retry 3 --retry-delay 1 -o "$TMP_DIR/$ARCHIVE" "$DOWNLOAD_ROOT/$ARCHIVE" || \
  die "download failed; verify that $VERSION supports $OS/$ARCH"
curl -fsSL --retry 3 --retry-delay 1 -o "$TMP_DIR/checksums.txt" "$DOWNLOAD_ROOT/checksums.txt" || \
  die "could not download checksums.txt for $VERSION"

expected=$(awk -v file="$ARCHIVE" '$2 == file || $2 == "*" file { print $1; exit }' "$TMP_DIR/checksums.txt")
[ -n "$expected" ] || die "checksums.txt has no entry for $ARCHIVE"

if command -v sha256sum >/dev/null 2>&1; then
  actual=$(sha256sum "$TMP_DIR/$ARCHIVE" | awk '{print $1}')
elif command -v shasum >/dev/null 2>&1; then
  actual=$(shasum -a 256 "$TMP_DIR/$ARCHIVE" | awk '{print $1}')
else
  die "sha256sum or shasum is required to verify the download"
fi
[ "$actual" = "$expected" ] || die "checksum verification failed for $ARCHIVE"

tar -xzf "$TMP_DIR/$ARCHIVE" -C "$TMP_DIR"
[ -f "$TMP_DIR/$PROGRAM" ] || die "release archive does not contain $PROGRAM"
[ -x "$TMP_DIR/$PROGRAM" ] || chmod +x "$TMP_DIR/$PROGRAM"

if [ ! -d "$INSTALL_DIR" ]; then
  mkdir -p "$INSTALL_DIR" || die "cannot create $INSTALL_DIR; choose a user-writable directory with --install-dir"
fi
[ -w "$INSTALL_DIR" ] || die "$INSTALL_DIR is not writable; choose another directory with --install-dir"

# Rename within the destination filesystem so an interrupted upgrade cannot
# leave a partially copied executable behind.
STAGED="$INSTALL_DIR/.${PROGRAM}.install.$$"
cp "$TMP_DIR/$PROGRAM" "$STAGED" || die "could not stage the executable in $INSTALL_DIR"
chmod +x "$STAGED"
mv -f "$STAGED" "$TARGET"
if [ -f "$LEGACY_TARGET" ]; then
  rm -f -- "$LEGACY_TARGET"
fi

printf 'Installed %s %s to %s\n' "$PROGRAM" "$VERSION" "$TARGET"
case ":${PATH}:" in
  *:"$INSTALL_DIR":*) ;;
  *) printf 'Add %s to PATH to run %s from any directory.\n' "$INSTALL_DIR" "$PROGRAM" ;;
esac
printf 'Upgrade by running this installer again. Uninstall with:\n  sh install.sh --uninstall --install-dir %s\n' "$INSTALL_DIR"
