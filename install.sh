#!/bin/sh
# ttybus installer.
#
#   curl -fsSL https://raw.githubusercontent.com/tjstebbing/ttybus/main/install.sh | sh
#
# Optional env:
#   VERSION   release tag (default: latest GitHub release)
#   PREFIX    install prefix (default: $HOME/.local) — binary goes in $PREFIX/bin
#   BINDIR    override install directory
#   REPO      GitHub owner/name (default: tjstebbing/ttybus)
set -eu

REPO="${REPO:-tjstebbing/ttybus}"
PREFIX="${PREFIX:-${HOME}/.local}"
BINDIR="${BINDIR:-${PREFIX}/bin}"
BIN=ttybus

need() {
	if ! command -v "$1" >/dev/null 2>&1; then
		printf 'ttybus-install: need %s\n' "$1" >&2
		exit 1
	fi
}

need curl
need tar
need uname
need mktemp

os=$(uname -s | tr '[:upper:]' '[:lower:]')
arch=$(uname -m)
case "$arch" in
x86_64 | amd64) arch=amd64 ;;
aarch64 | arm64) arch=arm64 ;;
*)
	printf 'ttybus-install: unsupported arch %s\n' "$arch" >&2
	exit 1
	;;
esac
case "$os" in
linux | darwin) ;;
*)
	printf 'ttybus-install: unsupported os %s\n' "$os" >&2
	exit 1
	;;
esac

latest_tag() {
	tag=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" |
		grep -m1 '"tag_name"' |
		sed 's/.*"tag_name": *"\([^"]*\)".*/\1/') || true
	if [ -n "$tag" ] && [ "$tag" != "null" ]; then
		printf '%s\n' "$tag"
		return
	fi
	url=$(curl -fsSLI -o /dev/null -w '%{url_effective}' \
		"https://github.com/${REPO}/releases/latest") || true
	printf '%s\n' "${url##*/}"
}

VERSION="${VERSION:-$(latest_tag)}"
case "$VERSION" in
'' | latest)
	printf 'ttybus-install: no GitHub release found.\n' >&2
	printf '  try: go install github.com/%s/cmd/ttybus@latest\n' "$REPO" >&2
	exit 1
	;;
esac

asset="${BIN}_${VERSION}_${os}_${arch}.tar.gz"
base="https://github.com/${REPO}/releases/download/${VERSION}"
workdir=$(mktemp -d)
trap 'rm -rf "$workdir"' EXIT

printf 'ttybus-install: %s %s/%s -> %s\n' "$VERSION" "$os" "$arch" "$BINDIR" >&2

curl -fsSL -o "$workdir/$asset" "${base}/${asset}"
curl -fsSL -o "$workdir/checksums.txt" "${base}/checksums.txt"

want=$(awk -v f="$asset" '$2 == f { print $1 }' "$workdir/checksums.txt")
if [ -z "$want" ]; then
	printf 'ttybus-install: %s not listed in checksums.txt\n' "$asset" >&2
	exit 1
fi
if command -v sha256sum >/dev/null 2>&1; then
	got=$(sha256sum "$workdir/$asset" | awk '{ print $1 }')
else
	need shasum
	got=$(shasum -a 256 "$workdir/$asset" | awk '{ print $1 }')
fi
if [ "$want" != "$got" ]; then
	printf 'ttybus-install: checksum mismatch for %s\n' "$asset" >&2
	exit 1
fi

tar -C "$workdir" -xzf "$workdir/$asset"
if [ ! -f "$workdir/$BIN" ]; then
	printf 'ttybus-install: archive missing %s\n' "$BIN" >&2
	exit 1
fi
chmod 755 "$workdir/$BIN"

if ! mkdir -p "$BINDIR" 2>/dev/null || [ ! -w "$BINDIR" ]; then
	printf 'ttybus-install: cannot write %s (set PREFIX or BINDIR)\n' "$BINDIR" >&2
	exit 1
fi
mv "$workdir/$BIN" "$BINDIR/$BIN"

printf 'installed %s to %s\n' "$("$BINDIR/$BIN" version)" "$BINDIR/$BIN" >&2
case ":$PATH:" in
*":$BINDIR:"*) ;;
*)
	printf 'ttybus-install: add %s to PATH\n' "$BINDIR" >&2
	;;
esac
