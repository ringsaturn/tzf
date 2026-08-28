#!/usr/bin/env bash
# Builds the tzf-dist artifact set from the upstream raw GeoJSON and installs
# it into the sibling tzf-dist checkout (go.mod replaces
# github.com/ringsaturn/tzf-dist with ../tzf-dist during development, W7).
# Gob intermediates stay in tmp/tzf-dist-dev — build-internal pipeline cache
# and parity fixtures (TZF_PARITY_TOPO/PREINDEX); they are never distributed.
#
# Run once after cloning (needs ../tzf-dist checked out; CI checks out
# ringsaturn/tzf-dist next to this repo). Drop the go.mod replace and this
# script once tzf-dist publishes the artifact release.
set -euo pipefail
cd "$(dirname "$0")/.."

TBB_VERSION="${TBB_VERSION:-2026c}"
WORK_DIR=tmp/tzf-dist-dev
DIST_DIR=../tzf-dist

if [ ! -d "$DIST_DIR" ]; then
  echo "error: $DIST_DIR not found — clone https://github.com/ringsaturn/tzf-dist next to this repo" >&2
  exit 1
fi

mkdir -p "$WORK_DIR"

# The artifact embeds ship on tzf-dist's v2-artifacts branch; when building
# against a checkout that predates them (e.g. main in CI), add the embed file
# so the replace target compiles.
if [ ! -f "$DIST_DIR"/embed_v2.go ]; then
  cat > "$DIST_DIR"/embed_v2.go <<'EOF'
package tzfdist

import _ "embed"

// The TZF embedded-binary artifact set backing the tzf/v2 finders.

//go:embed lite.tzb
var LiteTZB []byte

//go:embed lite.tzm
var LiteTZM []byte

//go:embed full.tzb
var FullTZB []byte
EOF
fi
touch "$DIST_DIR"/lite.tzb "$DIST_DIR"/lite.tzm "$DIST_DIR"/full.tzb

# Fetch the upstream boundary release (skipped when already present).
GEOJSON="$WORK_DIR"/combined-with-oceans.json
if [ ! -s "$GEOJSON" ]; then
  ZIP="$WORK_DIR"/timezones-with-oceans.geojson.zip
  curl -fL -o "$ZIP" \
    "https://github.com/evansiroky/timezone-boundary-builder/releases/download/$TBB_VERSION/timezones-with-oceans.geojson.zip"
  unzip -o -d "$WORK_DIR" "$ZIP"
  rm -f "$ZIP"
fi

# Run the pipeline (same stage order as the v1 data build).
export TIMEZONE_BOUNDARY_VERSION="$TBB_VERSION"
D="$WORK_DIR"/combined-with-oceans

go run ./cmd/geojson2tzpb "$D".json

# full line: dedup + compress on full precision data
go run ./cmd/deduplicatetzpb -o "$D".topo.gob "$D".gob
go run ./cmd/compresstopotzpb -o "$D".compress.topo.gob "$D".topo.gob

# lite line: topology-aware simplify + dedup + compress; preindex
go run ./cmd/reducetzpb -o "$D".topology.gob "$D".gob
go run ./cmd/deduplicatetzpb -o "$D".topology.topo.gob "$D".topology.gob
go run ./cmd/compresstopotzpb -o "$D".topology.compress.topo.gob "$D".topology.topo.gob
go run ./cmd/preindextzpb "$D".topology.gob

# artifacts, installed over the placeholders in the tzf-dist checkout
go run ./cmd/topo2embed -profile e -preindex "$D".topology.preindex.gob \
  -o "$DIST_DIR"/lite.tzb "$D".topology.compress.topo.gob
go run ./cmd/topo2embed -profile e -preindex "$D".topology.preindex.gob \
  -o "$DIST_DIR"/full.tzb "$D".compress.topo.gob
go run ./cmd/tzb2tzm -o "$DIST_DIR"/lite.tzm "$DIST_DIR"/lite.tzb

echo "tzf-dist artifacts installed:"
ls -l "$DIST_DIR"/lite.tzb "$DIST_DIR"/lite.tzm "$DIST_DIR"/full.tzb
