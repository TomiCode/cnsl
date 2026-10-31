#!/usr/bin/env bash
# Vendors a compiled trmnl-framework release into internal/assets/framework.
#
#   scripts/vendor-framework.sh <path-to-trmnl-framework-checkout> <version>
#
# Only the gzipped minified CSS/JS are copied (served with Content-Encoding: gzip) to keep the
# binary small, plus the fonts the CSS references (woff2 where available, else ttf).
set -euo pipefail

src=${1:?path to trmnl-framework checkout}
version=${2:?framework version, e.g. 3.4.4}
dest="$(dirname "$0")/../internal/assets/framework"

[ -f "$src/public/css/$version/plugins.min.css.gz" ] || { echo "no release $version in $src" >&2; exit 1; }

mkdir -p "$dest/$version" "$dest/fonts"
cp "$src/public/css/$version/plugins.min.css.gz" "$dest/$version/plugins.min.css.gz"
cp "$src/public/js/$version/plugins.min.js.gz" "$dest/$version/plugins.min.js.gz"

for f in "$src"/public/fonts/*.woff2 "$src"/public/fonts/*.ttf; do
  name=$(basename "$f")
  base=${name%.*}
  # Skip a ttf when a woff2 of the same font exists: the CSS lists woff2 first.
  if [[ $name == *.ttf && -f "$src/public/fonts/$base.woff2" ]]; then continue; fi
  cp "$f" "$dest/fonts/$name"
done

cp "$src/LICENSE" "$dest/LICENSE"
cp "$src/THIRD_PARTY_NOTICES.md" "$dest/THIRD_PARTY_NOTICES.md"
commit=$(git -C "$src" rev-parse --short HEAD 2>/dev/null || echo unknown)
printf 'trmnl-framework %s (source commit %s)\n' "$version" "$commit" > "$dest/$version/SOURCE"
echo "vendored trmnl-framework $version ($commit) into $dest"
