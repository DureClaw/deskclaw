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
chmod +x deskclaw${ext} 2>/dev/null || true
echo "✓ saved ./deskclaw${ext}"
echo "  run: STATE_SERVER=<bus:4000> OAH_SECRET=<token> WORK_KEY=<WK> AGENT_NAME=deskclaw@\$(hostname) ./deskclaw${ext}"
if [ "$goos" = darwin ]; then
  echo "  macOS: grant Screen Recording (screenshots) + Accessibility (click/type) on first run."
fi
