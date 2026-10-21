#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
mkdir -p build/bin
case "$(uname -s)" in
 Darwin)
  export CGO_LDFLAGS="${CGO_LDFLAGS:-} -framework UniformTypeIdentifiers"
  go build -tags desktop,production,sqlite_fts5 -ldflags '-s -w' -o build/bin/Shiwa
  mkdir -p build/bin/Shiwa.app/Contents/MacOS build/bin/Shiwa.app/Contents/Resources
  cp build/bin/Shiwa build/bin/Shiwa.app/Contents/MacOS/Shiwa.new
  mv -f build/bin/Shiwa.app/Contents/MacOS/Shiwa.new build/bin/Shiwa.app/Contents/MacOS/Shiwa
  cp build/darwin/Info.plist build/bin/Shiwa.app/Contents/Info.plist
  cp build/darwin/AppIcon.icns build/bin/Shiwa.app/Contents/Resources/AppIcon.icns
  # Sign outside iCloud/File Provider directories, which can add FinderInfo during signing.
  localens_stage=$(mktemp -d /private/tmp/localens-sign.XXXXXX)
  trap 'rm -rf "$localens_stage"' EXIT HUP INT TERM
  cp -R -X build/bin/Shiwa.app "$localens_stage/Shiwa.app"
  xattr -cr "$localens_stage/Shiwa.app"
  codesign --force --deep --sign - "$localens_stage/Shiwa.app"
  codesign --verify --deep --strict "$localens_stage/Shiwa.app"
  cp -R -X "$localens_stage/Shiwa.app/." build/bin/Shiwa.app/
  printf '已构建：%s/build/bin/Shiwa.app\n' "$PWD"
  ;;
 Linux)
  linux_tags=desktop,production,sqlite_fts5
  if pkg-config --exists webkit2gtk-4.1; then linux_tags="$linux_tags,webkit2_41"; fi
  go build -tags "$linux_tags" -ldflags '-s -w' -o build/bin/Shiwa
  printf '已构建：%s/build/bin/Shiwa\n' "$PWD"
  ;;
 *)
  wails build -s -skipbindings
  ;;
esac
