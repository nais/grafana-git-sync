#!/usr/bin/env bash
#MISE description="Ensure Go files are formatted"
set -euo pipefail

unformatted="$(go tool mvdan.cc/gofumpt -l ./)"
if [[ -n "$unformatted" ]]; then
	echo "Not formatted (run \`mise run fmt\`):"
	echo "$unformatted"
	exit 1
fi
