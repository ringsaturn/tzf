#!/usr/bin/env bash
# Builds tmp/tzf-dist-dev, the development stand-in for the tzf-dist release
# that will carry the .tzb/.tzm artifact set (v2 plan W7). The committed
# go.work replaces github.com/ringsaturn/tzf-dist with this directory, so run
# this once after cloning (or when bumping TBB_VERSION) before building.
# Drop the replace and this script once tzf-dist publishes the artifacts.
#
# The whole chain runs from the upstream raw GeoJSON — no protobuf, no
# translation step. The gob intermediates it leaves behind are build-internal
# products (pipeline cache + parity fixtures for TZF_PARITY_TOPO/PREINDEX);
# they are never distributed.
set -euo pipefail
cd "$(dirname "$0")/.."

TBB_VERSION="${TBB_VERSION:-2026c}"
DEV_DIR=tmp/tzf-dist-dev

mkdir -p "$DEV_DIR"

# Step 1 (workspace-independent): the dev module needs to exist with
# placeholder artifacts before any workspace go command can run.
cat > "$DEV_DIR"/go.mod <<'EOF'
module github.com/ringsaturn/tzf-dist

go 1.25.0
EOF
touch "$DEV_DIR"/lite.tzb "$DEV_DIR"/lite.tzm "$DEV_DIR"/full.tzb
cat > "$DEV_DIR"/embed.go <<'EOF'
package tzfdist

import _ "embed"

// The TZF embedded-binary artifact set backing the tzf/v2 finders.
// All three files carry the same data_version.

//go:embed lite.tzb
var LiteTZB []byte

//go:embed lite.tzm
var LiteTZM []byte

//go:embed full.tzb
var FullTZB []byte
EOF

# Step 2: fetch the upstream boundary release (skipped when already present).
GEOJSON="$DEV_DIR"/combined-with-oceans.json
if [ ! -s "$GEOJSON" ]; then
  ZIP="$DEV_DIR"/timezones-with-oceans.geojson.zip
  curl -fL -o "$ZIP" \
    "https://github.com/evansiroky/timezone-boundary-builder/releases/download/$TBB_VERSION/timezones-with-oceans.geojson.zip"
  unzip -o -d "$DEV_DIR" "$ZIP"
  rm -f "$ZIP"
fi

# Step 3: run the pipeline (same stage order as the v1 data build).
export TIMEZONE_BOUNDARY_VERSION="$TBB_VERSION"
D="$DEV_DIR"/combined-with-oceans

go run ./cmd/geojson2tzpb "$D".json

# full line: dedup + compress on full precision data
go run ./cmd/deduplicatetzpb -o "$D".topo.gob "$D".gob
go run ./cmd/compresstopotzpb -o "$D".compress.topo.gob "$D".topo.gob

# lite line: topology-aware simplify + dedup + compress; preindex
go run ./cmd/reducetzpb -o "$D".topology.gob "$D".gob
go run ./cmd/deduplicatetzpb -o "$D".topology.topo.gob "$D".topology.gob
go run ./cmd/compresstopotzpb -o "$D".topology.compress.topo.gob "$D".topology.topo.gob
go run ./cmd/preindextzpb "$D".topology.gob

# artifacts
go run ./cmd/topo2embed -profile e -preindex "$D".topology.preindex.gob \
  -o "$DEV_DIR"/lite.tzb "$D".topology.compress.topo.gob
go run ./cmd/topo2embed -profile e -preindex "$D".topology.preindex.gob \
  -o "$DEV_DIR"/full.tzb "$D".compress.topo.gob
go run ./cmd/tzb2tzm -o "$DEV_DIR"/lite.tzm "$DEV_DIR"/lite.tzb

echo "tzf-dist-dev ready:"
ls -l "$DEV_DIR"/lite.tzb "$DEV_DIR"/lite.tzm "$DEV_DIR"/full.tzb
