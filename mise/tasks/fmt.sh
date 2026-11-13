#!/usr/bin/env bash
#MISE description="Format Go files using gofumpt"
set -euo pipefail

go tool mvdan.cc/gofumpt -w ./
