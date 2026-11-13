#!/usr/bin/env bash
#MISE description="Verify all GitHub Actions are pinned"
set -euo pipefail

go tool github.com/sethvargo/ratchet lint .github/workflows/*.yaml
