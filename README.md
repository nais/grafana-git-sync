# grafana-git-sync

CronJob that sets up read-only [Grafana Git Sync](https://grafana.com/docs/grafana/latest/as-code/git-sync/) for every repository a GitHub App installation can access. It runs in Grafana's cluster, so it reaches Grafana directly instead of through IAP.

Each run creates a GitHub App connection and one repository per GitHub repository, syncing `grafana/` from the default branch. Repositories are read-only: Grafana never writes back. Removed or archived repositories are pruned. Only objects labelled `app.kubernetes.io/managed-by: grafana-git-sync` are touched, and manual changes to them are overwritten.

The Grafana UI can't do this: it requires the GitHub App to have write permissions ([grafana/grafana#137435](https://github.com/grafana/grafana/issues/137435)). Grafana will show the connection as unhealthy, but syncing still works.

## Install

Requires:
- A GitHub App with Contents and Metadata read access.
- A Grafana service account token with the Admin role.
- A Secret with both:

```sh
kubectl create secret generic grafana-git-sync \
  --from-literal=grafana-token="$GRAFANA_TOKEN" \
  --from-file=github-app-private-key=app.private-key.pem

helm upgrade --install grafana-git-sync \
  oci://europe-north1-docker.pkg.dev/nais-io/nais/grafana-git-sync --version <chart-version> \
  --set image.tag=<image-version> \
  --set grafana.url=http://<grafana-service>.<namespace>.svc.cluster.local \
  --set github.appID=<app-id> --set github.installationID=<installation-id> \
  --set existingSecret.name=grafana-git-sync
```

## Development

`mise run test`, `mise run check`, `mise run fmt`.

Pushes to `main` publish the image. Changes under `charts/` publish the chart; bump its version manually.
