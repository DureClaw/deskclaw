#!/usr/bin/env sh
# deskclaw installer — detects your OS/CPU and fetches the matching prebuilt binary.
#   curl -fsSL https://github.com/DureClaw/deskclaw/releases/latest/download/install.sh | sh
# deskclaw is the DureClaw desktop GUI node (Windows/macOS; Linux = join-only stub).
set -e
REPO="DureClaw/deskclaw"
os=$(uname -s); arch=$(uname -m)
case "$os" in
  Darwin) goos=darwin ;;
  Linux)  goos=linux ;;
  MINGW*|MSYS*|CYGWIN*) goos=windows ;;
  *) echo "unsupported OS: $os"; exit 1 ;;
esac
case "$arch" in
  x86_64|amd64) goarch=amd64 ;;
  arm64|aarch64) goarch=arm64 ;;
  *) echo "unsupported arch: $arch"; exit 1 ;;
esac
# Linux ships amd64-only (join-only stub; full GUI is Win/macOS).
if [ "$goos" = linux ] && [ "$goarch" != amd64 ]; then
  echo "deskclaw on Linux is a join-only stub (amd64 asset only)"; goarch=amd64
fi
ext=""; [ "$goos" = windows ] && ext=".exe"
asset="deskclaw-${goos}-${goarch}${ext}"
url="https://github.com/${REPO}/releases/latest/download/${asset}"
echo "deskclaw: downloading ${asset} ..."
curl -fsSL "$url" -o deskclaw${ext}
# verify against the release's SHA256SUMS (from `make all`) when it's published
if sums=$(curl -fsSL "https://github.com/${REPO}/releases/latest/download/SHA256SUMS" 2>/dev/null); then
  want=$(printf '%s\n' "$sums" | awk -v a="$asset" '$2 == a || $2 == "*"a {print $1}')
  if command -v sha256sum >/dev/null 2>&1; then got=$(sha256sum deskclaw${ext} | awk '{print $1}')
  else got=$(shasum -a 256 deskclaw${ext} | awk '{print $1}'); fi
  if [ -z "$want" ] || [ "$want" != "$got" ]; then
    rm -f deskclaw${ext}; echo "✗ checksum mismatch for ${asset} (got $got, want ${want:-none}) — aborted"; exit 1
  fi
  echo "✓ checksum ok"
else
  echo "! no SHA256SUMS in the release — checksum not verified"
fi
chmod +x deskclaw${ext} 2>/dev/null || true
echo "✓ saved ./deskclaw${ext}"
echo "  run: STATE_SERVER=<bus:4000> OAH_SECRET=<token> WORK_KEY=<WK> AGENT_NAME=deskclaw@\$(hostname) ./deskclaw${ext}"
if [ "$goos" = darwin ]; then
  echo "  macOS: grant Screen Recording (screenshots) + Accessibility (click/type) on first run."
fi
