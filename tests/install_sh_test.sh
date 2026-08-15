#!/bin/sh

set -eu

ROOT=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
TEST_ROOT=$(mktemp -d "${TMPDIR:-/tmp}/luban-install-test.XXXXXX")
trap 'rm -rf -- "$TEST_ROOT"' EXIT HUP INT TERM

RELEASES="$TEST_ROOT/releases"
TAG=v0.1.0
ASSET=calculated-later
mkdir -p "$RELEASES/download/$TAG" "$TEST_ROOT/payload" "$TEST_ROOT/home"

cat > "$TEST_ROOT/payload/luban-code" <<'EOF'
#!/bin/sh
printf 'test-release-v1\n'
EOF
chmod +x "$TEST_ROOT/payload/luban-code"

ASSET=luban-code_Linux_x86_64.tar.gz
tar -czf "$RELEASES/download/$TAG/$ASSET" -C "$TEST_ROOT/payload" luban-code
if command -v sha256sum >/dev/null 2>&1; then
  HASH=$(sha256sum "$RELEASES/download/$TAG/$ASSET" | awk '{print $1}')
else
  HASH=$(shasum -a 256 "$RELEASES/download/$TAG/$ASSET" | awk '{print $1}')
fi
printf '%s  %s\n' "$HASH" "$ASSET" > "$RELEASES/download/$TAG/checksums.txt"

run_installer() {
  HOME="$TEST_ROOT/home" \
    LUBAN_INSTALL_OS=Linux \
    LUBAN_INSTALL_ARCH=x86_64 \
    LUBAN_INSTALL_RELEASES_URL="file://$RELEASES" \
    LUBAN_INSTALL_LATEST_VERSION="$TAG" \
    sh "$ROOT/install.sh" "$@"
}

assert_contains() {
  case "$1" in
    *"$2"*) ;;
    *) printf 'expected output to contain: %s\nactual: %s\n' "$2" "$1" >&2; exit 1 ;;
  esac
}

output=$(run_installer)
assert_contains "$output" "Installed luban-code v0.1.0"
[ "$("$TEST_ROOT/home/.local/bin/luban-code")" = test-release-v1 ]

# Reinstalling the same release exercises the atomic, idempotent upgrade path.
run_installer --version "$TAG" >/dev/null
[ "$("$TEST_ROOT/home/.local/bin/luban-code")" = test-release-v1 ]

output=$(run_installer --uninstall)
assert_contains "$output" "Removed"
[ ! -e "$TEST_ROOT/home/.local/bin/luban-code" ]
output=$(run_installer --uninstall)
assert_contains "$output" "is not installed"

CUSTOM="$TEST_ROOT/custom bin"
run_installer --install-dir "$CUSTOM" >/dev/null
[ -x "$CUSTOM/luban-code" ]

cp "$RELEASES/download/$TAG/checksums.txt" "$TEST_ROOT/good-checksums"
printf '%064d  %s\n' 0 "$ASSET" > "$RELEASES/download/$TAG/checksums.txt"
if run_installer --install-dir "$TEST_ROOT/bad-checksum" >"$TEST_ROOT/out" 2>"$TEST_ROOT/err"; then
  printf 'installer unexpectedly accepted a bad checksum\n' >&2
  exit 1
fi
assert_contains "$(cat "$TEST_ROOT/err")" "checksum verification failed"
mv "$TEST_ROOT/good-checksums" "$RELEASES/download/$TAG/checksums.txt"

if LUBAN_INSTALL_OS=Plan9 LUBAN_INSTALL_ARCH=x86_64 HOME="$TEST_ROOT/home" sh "$ROOT/install.sh" >"$TEST_ROOT/out" 2>"$TEST_ROOT/err"; then
  printf 'installer unexpectedly accepted an unsupported OS\n' >&2
  exit 1
fi
assert_contains "$(cat "$TEST_ROOT/err")" "unsupported operating system"

if run_installer --version nope >"$TEST_ROOT/out" 2>"$TEST_ROOT/err"; then
  printf 'installer unexpectedly accepted an invalid version\n' >&2
  exit 1
fi
assert_contains "$(cat "$TEST_ROOT/err")" "invalid release version"

if run_installer --version 'v1/../../bad' >"$TEST_ROOT/out" 2>"$TEST_ROOT/err"; then
  printf 'installer unexpectedly accepted an unsafe version\n' >&2
  exit 1
fi
assert_contains "$(cat "$TEST_ROOT/err")" "invalid release version"

assert_contains "$(sh "$ROOT/install.sh" --help)" "--install-dir"
printf 'install.sh tests passed\n'
