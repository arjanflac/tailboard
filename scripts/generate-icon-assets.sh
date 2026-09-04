#!/bin/sh
set -eu

SCRIPT_DIR=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
PROJECT_DIR=$(dirname "$SCRIPT_DIR")
BRAND_DIR="$PROJECT_DIR/assets/branding"
SOURCE="$BRAND_DIR/tailboard-mark-source.png"

command -v magick >/dev/null 2>&1 || {
  echo "ImageMagick is required (missing: magick)." >&2
  exit 1
}

# The approved transparent source includes detached low-alpha remnants from
# web background removal. Crop only; do not redraw or restyle the approved mark.
magick "$SOURCE" -crop 1020x1246+51+72 +repage "$BRAND_DIR/tailboard-mark.png"

magick -size 1024x1024 'xc:#202124' \
  \( "$BRAND_DIR/tailboard-mark.png" -resize 680x760 \) \
  -gravity center -composite -depth 8 \
  "$BRAND_DIR/tailboard-app-icon-macos-1024.png"

magick -size 512x512 'xc:#202124' \
  \( "$BRAND_DIR/tailboard-mark.png" -resize 250x305 \) \
  -gravity center -composite -depth 8 \
  "$BRAND_DIR/tailboard-app-icon-android-512.png"

# Android launchers apply their own mask and visual zoom to adaptive
# foregrounds. Keep the mark well inside the safe zone so round Pixel masks
# show the full phone silhouette with comfortable breathing room.
magick -size 432x432 xc:none \
  \( "$BRAND_DIR/tailboard-mark.png" -resize 120x146 \) \
  -gravity center -composite -depth 8 \
  "$BRAND_DIR/tailboard-adaptive-foreground-432.png"

for density_spec in mdpi:48 hdpi:72 xhdpi:96 xxhdpi:144 xxxhdpi:192; do
  density=${density_spec%%:*}
  size=${density_spec##*:}
  mkdir -p "$PROJECT_DIR/android/app/src/main/res/mipmap-$density"
  magick "$BRAND_DIR/tailboard-app-icon-android-512.png" -resize "${size}x${size}" \
    "$PROJECT_DIR/android/app/src/main/res/mipmap-$density/ic_launcher.png"
done

for density_spec in mdpi:108 hdpi:162 xhdpi:216 xxhdpi:324 xxxhdpi:432; do
  density=${density_spec%%:*}
  size=${density_spec##*:}
  mkdir -p "$PROJECT_DIR/android/app/src/main/res/mipmap-$density"
  magick "$BRAND_DIR/tailboard-adaptive-foreground-432.png" -resize "${size}x${size}" \
    "$PROJECT_DIR/android/app/src/main/res/mipmap-$density/ic_launcher_foreground.png"
done

MAC_ICON_DIR="$PROJECT_DIR/macos/TailboardMacHost/Assets.xcassets/AppIcon.appiconset"
for icon_size in 16 32 64 128 256 512 1024; do
  magick "$BRAND_DIR/tailboard-app-icon-macos-1024.png" \
    -resize "${icon_size}x${icon_size}" "$MAC_ICON_DIR/icon-${icon_size}.png"
done
