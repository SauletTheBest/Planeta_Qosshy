#!/usr/bin/env bash
# Builds everything the end-to-end scripts start, into /tmp/planeta-e2e (outside the repo).
# Run from Git Bash:   scripts/e2e/build.sh
set -euo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"
REPO="$(cd "$HERE/../.." && pwd)"
# NOT inside the repo: "Рабочий стол" has a space and Cyrillic letters
RUN="${RUN:-/tmp/planeta-e2e}"
BANK_REPO="${BANK_REPO:-$REPO/../planeta_qosshy_microservice}"
mkdir -p "$RUN"

echo "shop (current code)";  (cd "$REPO" && go build -o "$RUN/app.exe" .)
echo "test bank";            (cd "$BANK_REPO" && go build -o "$RUN/payments.exe" .)

# Old versions of the shop: the migrations are tested on data written by OLD code,
# the way it happens when a real server is updated.
build_old() {  # $1 = commit, $2 = source folder, $3 = binary
  rm -rf "$RUN/$2" && mkdir -p "$RUN/$2"
  git -C "$REPO" archive "$1" | tar -x -C "$RUN/$2"
  (cd "$RUN/$2" && go build -o "$RUN/$3" .)
}
echo "shop at schema v5"; build_old 6c24b6e old-src old.exe   # before block A (testA)
echo "shop at schema v7"; build_old dfcdf1e v7-src app7.exe   # before block C (testC)

python "$HERE/mkpng.py" "$RUN" >/dev/null   # test images for the photo upload checks
echo "done: $RUN"
