#!/bin/sh
set -eu
manifest=${1:?usage: compare-all.sh MANIFEST}
root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
while IFS= read -r candidate; do [ -z "$candidate" ] || "$root/tools/candidates/compare.sh" "$candidate"; done < "$manifest"
