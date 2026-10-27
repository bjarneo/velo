#!/usr/bin/env sh
# Install the latest velo release on Linux.
#
#   curl -fsSL https://raw.githubusercontent.com/bjarneo/velo/main/install.sh | sh
#
# Set VELO_INSTALL_DIR to install somewhere other than ~/.local/bin.
# Set VELO_VERSION, for example v0.1.0, to install one release.
set -eu

repo="bjarneo/velo"
dir="${VELO_INSTALL_DIR:-$HOME/.local/bin}"

fail() {
	echo "velo: $1" >&2
	exit 1
}

[ "$(uname -s)" = Linux ] || fail "release builds are for Linux. On other systems, run: go install github.com/$repo@latest"

case "$(uname -m)" in
x86_64 | amd64) arch=amd64 ;;
aarch64 | arm64) arch=arm64 ;;
*) fail "no release build for $(uname -m). Run: go install github.com/$repo@latest" ;;
esac

for tool in curl tar sha256sum; do
	command -v "$tool" >/dev/null 2>&1 || fail "$tool is not installed"
done

tag="${VELO_VERSION:-}"
if [ -z "$tag" ]; then
	# The latest-release page redirects to .../releases/tag/<tag>.
	url=$(curl -fsSLI -o /dev/null -w '%{url_effective}' "https://github.com/$repo/releases/latest") || fail "cannot reach github.com"
	tag="${url##*/}"
fi
case "$tag" in v*) ;; *) fail "cannot find the latest release" ;; esac

version="${tag#v}"
file="velo_${version}_linux_${arch}.tar.gz"
base="https://github.com/$repo/releases/download/$tag"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

echo "velo: download $file"
curl -fsSL -o "$tmp/$file" "$base/$file" || fail "cannot download $base/$file"
curl -fsSL -o "$tmp/checksums.txt" "$base/checksums.txt" || fail "cannot download checksums.txt"
(cd "$tmp" && grep " $file\$" checksums.txt | sha256sum -c - >/dev/null) || fail "checksum of $file does not match"

tar -xzf "$tmp/$file" -C "$tmp" velo
mkdir -p "$dir"
install -m 755 "$tmp/velo" "$dir/velo"
echo "velo: installed $version to $dir/velo"

case ":$PATH:" in
*":$dir:"*) echo "velo: run velo to start a test" ;;
*) echo "velo: $dir is not in PATH. Add it, or run $dir/velo" ;;
esac
