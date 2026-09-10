#!/usr/bin/env bash
# Builds the tzf-dist artifact set from the upstream raw GeoJSON and installs
# it into the sibling tzf-dist checkout for tzf-dist's own embed tests (the
# tzf runtime itself embeds the published tzf-dist module).
# Gob intermediates stay in tmp/tzf-dist-dev — build-internal pipeline cache
# and parity fixtures (TZF_PARITY_TOPO/PREINDEX); they are never distributed.
#
# Run once after cloning (needs ../tzf-dist checked out; CI checks out
# ringsaturn/tzf-dist next to this repo). The installed artifacts are
# uncommitted worktree state over tzf-dist's committed placeholders — real
# data is never committed on main; restore with
#   git -C ../tzf-dist checkout -- lite.tzb lite.tzm full.tzb
set -euo pipefail
cd "$(dirname "$0")/.."

# --shim-only reconciles the tzf-dist embed shim and exits, skipping the
# minutes-long pipeline. CI calls it on every run (the shim is workspace
# state, never cached) before deciding whether the artifacts need rebuilding.
SHIM_ONLY=false
if [ "${1:-}" = "--shim-only" ]; then
  SHIM_ONLY=true
fi

TBB_VERSION="${TBB_VERSION:-2026c}"
WORK_DIR=tmp/tzf-dist-dev
DIST_DIR=../tzf-dist

if [ ! -d "$DIST_DIR" ]; then
  echo "error: $DIST_DIR not found — clone https://github.com/ringsaturn/tzf-dist next to this repo" >&2
  exit 1
fi

mkdir -p "$WORK_DIR"

# A previous run against a pre-embed checkout may have left the shim behind
# (a restored CI cache does the same). Once the checkout itself declares the
# embeds, the shim would be a duplicate declaration — drop it first.
if [ -f "$DIST_DIR"/embed_v2.go ]; then
  for f in "$DIST_DIR"/*.go; do
    case "$f" in */embed_v2.go) continue ;; esac
    if grep -qs "var LiteTZB" "$f"; then
      echo "removing stale $DIST_DIR/embed_v2.go shim ($f declares the embeds)" >&2
      rm -f "$DIST_DIR"/embed_v2.go
      break
    fi
  done
fi

# The artifact embeds ship on tzf-dist's v2-artifacts branch; when building
# against a checkout that predates them (e.g. main before the branch merges),
# add the embed file so the checkout compiles.
if ! grep -qs "var LiteTZB" "$DIST_DIR"/*.go; then
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
  touch "$DIST_DIR"/lite.tzb "$DIST_DIR"/lite.tzm "$DIST_DIR"/full.tzb
fi

if [ "$SHIM_ONLY" = true ]; then
  exit 0
fi

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
echo
echo "note: these overwrite the committed placeholders in $DIST_DIR as"
echo "uncommitted worktree state (real data is never committed on main —"
echo "it ships via tags on the data branch). Restore the placeholders with:"
echo "  git -C $DIST_DIR checkout -- lite.tzb lite.tzm full.tzb"
