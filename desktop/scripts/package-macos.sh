#!/usr/bin/env bash

set -euo pipefail

script_dir="$(cd "$(dirname "$0")" && pwd)"
desktop_dir="$(cd "$script_dir/.." && pwd)"
dist_dir="$desktop_dir/dist"
app_dir="$dist_dir/Status Deck.app"
contents_dir="$app_dir/Contents"
macos_dir="$contents_dir/MacOS"
resources_dir="$contents_dir/Resources"
iconset_dir="$dist_dir/StatusDeck.iconset"

rm -rf "$app_dir"
rm -rf "$iconset_dir"
mkdir -p "$macos_dir" "$resources_dir"

cd "$desktop_dir"
go run ./cmd/icon-gen --iconset "$iconset_dir"
iconutil -c icns "$iconset_dir" -o "$resources_dir/StatusDeck.icns"
go build -trimpath -ldflags="-s -w" -o "$macos_dir/status-deck" ./cmd/status-deck
cp "$desktop_dir/packaging/macos/Info.plist" "$contents_dir/Info.plist"
plutil -lint "$contents_dir/Info.plist"
codesign --force --deep --sign - "$app_dir"

mkdir -p "$dist_dir"
rm -f "$dist_dir/Status-Deck-macos.zip"
ditto -c -k --sequesterRsrc --keepParent "$app_dir" "$dist_dir/Status-Deck-macos.zip"

printf 'Built %s\n' "$app_dir"
printf 'Built %s\n' "$dist_dir/Status-Deck-macos.zip"
