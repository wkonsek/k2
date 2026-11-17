#!/usr/bin/env bash
set -euo pipefail

repo="wkonsek/k2"
api="https://api.github.com/repos/${repo}/releases"

die() {
  echo "install.sh: $*" >&2
  exit 1
}

have() {
  command -v "$1" >/dev/null 2>&1
}

fetch() {
  local out="${2:--}"
  if have curl; then
    curl -fsSL -o "$out" "$1"
  elif have wget; then
    wget -qO "$out" "$1"
  else
    die "curl or wget is required"
  fi
}

[ "$(uname -s)" = "Linux" ] || die "unsupported OS: $(uname -s)"

case "$(uname -m)" in
  x86_64 | amd64) deb_arch="amd64"; rpm_arch="x86_64" ;;
  *) die "unsupported architecture: $(uname -m) (packages exist for x86_64 only)" ;;
esac

if have apt-get || { have dpkg && ! have rpm; }; then
  kind="deb"
elif have dnf || have yum || have zypper || have rpm; then
  kind="rpm"
else
  die "unsupported system: need apt/dpkg (.deb) or dnf/yum/zypper/rpm (.rpm)"
fi

if [ -n "${K2_VERSION:-}" ]; then
  url="${api}/tags/${K2_VERSION}"
else
  url="${api}/latest"
fi

json="$(fetch "$url")" || die "cannot read release info from $url"
tag="$(printf '%s' "$json" | grep -o '"tag_name": *"[^"]*"' | head -n1 | sed 's/.*: *"\(.*\)"/\1/')"
[ -n "$tag" ] || die "no release found"

if [ "$kind" = "deb" ]; then
  pattern="_${deb_arch}\.deb"
else
  pattern="\.${rpm_arch}\.rpm"
fi
asset="$(printf '%s' "$json" | grep -o '"browser_download_url": *"[^"]*"' | sed 's/.*: *"\(.*\)"/\1/' | grep -E "${pattern}\$" | head -n1 || true)"
[ -n "$asset" ] || die "release $tag has no .${kind} package for $(uname -m)"

if have k2; then
  current="$(k2 --version 2>/dev/null | awk '{print $2}' || true)"
  if [ "$current" = "$tag" ]; then
    echo "k2 $tag is already installed"
    exit 0
  fi
fi

sudo=""
if [ "$(id -u)" -ne 0 ]; then
  have sudo || die "run as root or install sudo"
  sudo="sudo"
fi

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
chmod 755 "$tmp"
file="$tmp/$(basename "$asset")"

echo "downloading $asset"
fetch "$asset" "$file" || die "download failed"

echo "installing $(basename "$file")"
if [ "$kind" = "deb" ]; then
  if have apt-get; then
    $sudo apt-get install -y --allow-downgrades "$file"
  else
    $sudo dpkg -i "$file"
  fi
elif have dnf; then
  $sudo dnf install -y "$file"
elif have yum; then
  $sudo yum install -y "$file"
elif have zypper; then
  $sudo zypper --non-interactive install -y --allow-unsigned-rpm "$file"
else
  $sudo rpm -Uvh "$file"
fi

hash -r
if have k2; then
  k2 --version
fi
