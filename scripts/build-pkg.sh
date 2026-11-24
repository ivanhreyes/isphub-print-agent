#!/usr/bin/env bash
# Builds and signs the redistributable .pkg for the print agent.
#
# Usage:
#   scripts/build-pkg.sh [VERSION] ["Developer ID Application: ..."] ["Developer ID Installer: ..."]
#
# The signing identities default to the ISPHub Developer ID certificates.
set -euo pipefail

VERSION="${1:-${VERSION:-0.2.0}}"
APP_ID="${2:-${APP_ID:-Developer ID Application: JORGE IVAN HERRERA REYES (X39UPFT8SK)}}"
INSTALLER_ID="${3:-${INSTALLER_ID:-Developer ID Installer: JORGE IVAN HERRERA REYES (X39UPFT8SK)}}"

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

BIN="dist/isphub-print-agent"
PAYLOAD="dist/pkg/root"
SCRIPTS="dist/pkg/scripts"
RESOURCES="dist/pkg/Resources"
COMPONENT="dist/pkg/isphub-print-agent-component.pkg"
DIST_XML="dist/pkg/Distribution.xml"
OUT="dist/ISPHub-Print-Agent-${VERSION}.pkg"

rm -rf dist/pkg
mkdir -p "$PAYLOAD/usr/local/bin" "$SCRIPTS" "$RESOURCES"

echo "==> Compilando binario arm64 (v${VERSION})"
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -trimpath \
  -ldflags "-s -w -X main.version=${VERSION}" \
  -o "$BIN" ./cmd/isphub-print-agent

echo "==> Firmando binario (Developer ID Application, hardened runtime)"
codesign --force --options runtime --timestamp --sign "$APP_ID" "$BIN"
codesign --verify --verbose=2 "$BIN"

cp "$BIN" "$PAYLOAD/usr/local/bin/isphub-print-agent"
cp installer/scripts/postinstall "$SCRIPTS/postinstall"
chmod +x "$SCRIPTS/postinstall"
cp installer/Resources/*.html "$RESOURCES/"
sed "s/__VERSION__/${VERSION}/" installer/Distribution.xml > "$DIST_XML"

echo "==> Empaquetando componente (pkgbuild)"
pkgbuild \
  --root "$PAYLOAD" \
  --identifier app.isphub.print-agent \
  --version "$VERSION" \
  --install-location / \
  --scripts "$SCRIPTS" \
  --sign "$INSTALLER_ID" \
  "$COMPONENT"

echo "==> Construyendo instalador (productbuild)"
productbuild \
  --distribution "$DIST_XML" \
  --package-path dist/pkg \
  --resources "$RESOURCES" \
  --sign "$INSTALLER_ID" \
  "$OUT"

echo "==> Verificando firma del instalador"
pkgutil --check-signature "$OUT"

echo ""
echo "OK: $OUT"
