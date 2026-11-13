#!/usr/bin/env bash
#MISE description="Lint and render the Helm chart"
set -euo pipefail

chart=charts/grafana-git-sync
values=(--set grafana.url=http://grafana.example:3000 --set github.appID=1 --set github.installationID=2 --set existingSecret.name=example --set image.tag=test)
helm lint --strict "$chart" "${values[@]}"
helm template grafana-git-sync "$chart" "${values[@]}" >/dev/null
