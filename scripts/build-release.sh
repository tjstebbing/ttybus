#!/bin/sh
# Cross-compile ttybus for the release matrix. Used by CI and `make dist`.
set -eu

cd "$(dirname "$0")/.."

VERSION=${VERSION:-dev}
OUT=${OUT:-dist}
BIN=ttybus

rm -rf "$OUT"
mkdir -p "$OUT"

checksum() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$@"
	else
		shasum -a 256 "$@"
	fi
}

for spec in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64; do
	goos=${spec%/*}
	goarch=${spec#*/}
	name="${BIN}_${VERSION}_${goos}_${goarch}"
	tmp=$(mktemp -d)
	trap 'rm -rf "$tmp"' EXIT
	echo "building ${name}" >&2
	CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" go build -trimpath \
		-ldflags "-s -w -X main.version=${VERSION}" \
		-o "$tmp/$BIN" ./cmd/ttybus
	tar -C "$tmp" -czf "$OUT/${name}.tar.gz" "$BIN"
	rm -rf "$tmp"
	trap - EXIT
done

(
	cd "$OUT"
	checksum *.tar.gz >checksums.txt
)

echo "wrote $OUT" >&2
ls -l "$OUT" >&2
