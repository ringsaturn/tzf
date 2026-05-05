#!/bin/bash
set -euo pipefail

detect_license() {
    local file="$1"
    if grep -q "MIT License\|The MIT License" "$file" 2>/dev/null; then
        echo "MIT"
    elif grep -q "Apache License" "$file" 2>/dev/null; then
        echo "Apache-2.0"
    elif grep -q "Open Database License" "$file" 2>/dev/null; then
        echo "ODbL-1.0"
    elif grep -q "Neither the name" "$file" 2>/dev/null; then
        echo "BSD-3-Clause"
    elif grep -q "Redistribution" "$file" 2>/dev/null; then
        echo "BSD-2-Clause"
    elif grep -q "Permission is hereby granted" "$file" 2>/dev/null; then
        # MIT without explicit "MIT License" header line
        echo "MIT"
    else
        echo "Unknown"
    fi
}

{
echo "NOTICE"
echo ""
echo "This product includes software developed by [github.com/ringsaturn/tzf]."
echo "------------------------------------------------------------------------"

find THIRD_PARTY_LICENSES -name "LICENSE*" | sort | while read -r license_file; do
    dir=$(dirname "$license_file" | sed 's|^THIRD_PARTY_LICENSES/||')
    filename=$(basename "$license_file")

    # skip the project's own license
    [ "$dir" = "github.com/ringsaturn/tzf" ] && continue

    license_type=$(detect_license "$license_file")
    # match only lines that start with "Copyright (c)" or "Copyright 20xx" to avoid
    # false positives from license body text (e.g. Apache-2.0, ODbL preamble)
    copyright=$(grep -E "^Copyright \(c\)|^Copyright 20[0-9][0-9]" "$license_file" | head -1 || true)

    # label LICENSE_DATA entries with a "(data)" suffix to distinguish from code license
    if [ "$filename" = "LICENSE_DATA" ]; then
        label="$dir (data)"
    else
        label="$dir"
    fi

    echo ""
    echo "$label"
    echo "License: $license_type"
    [ -n "$copyright" ] && echo "$copyright"
done
} > NOTICE
