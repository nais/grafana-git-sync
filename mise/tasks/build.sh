#!/usr/bin/env bash
#MISE description="Build the grafana-git-sync binary"
set -euo pipefail

CGO_ENABLED=0 go build -o bin/grafana-git-sync .
