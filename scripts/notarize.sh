#!/usr/bin/env bash
# Notarizes and staples the redistributable .pkg.
#
# Before first use, store your App Store Connect API key once:
#   xcrun notarytool store-credentials isphub-notary \
#     --key /path/to/AuthKey_XXXXXXXXXX.p8 \
#     --key-id KEYID --issuer ISSUER_ID
#
# Usage:
#   scripts/notarize.sh [VERSION]
set -euo pipefail

VERSION="${1:-${VERSION:-0.2.0}}"
PROFILE="${NOTARY_PROFILE:-isphub-notary}"

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

PKG="dist/ISPHub-Print-Agent-${VERSION}.pkg"

echo "==> Enviando a notarización (puede tardar varios minutos)"
xcrun notarytool submit "$PKG" \
  --keychain-profile "$PROFILE" \
  --wait

echo "==> Grapeando (stapler)"
xcrun stapler staple "$PKG"

echo "==> Verificando"
xcrun stapler validate "$PKG"
spctl --assess --type install --verbose=4 "$PKG"

echo ""
echo "OK: $PKG firmado, notarizado y grapado"
