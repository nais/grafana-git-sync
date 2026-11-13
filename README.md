# grafana-git-sync

Sets up read-only [Grafana Git Sync](https://grafana.com/docs/grafana/latest/as-code/git-sync/) for every repository a GitHub App installation can access.

Runs as a Kubernetes CronJob in the same cluster as Grafana, so it reaches Grafana's API directly instead of going through IAP.

## What it does

Each run:

1. Lists all non-archived repositories the GitHub App installation can access.
2. Creates or updates the Grafana connection `github-<installationID>`, with webhooks disabled because GitHub can't reach Grafana through IAP. The private key is only sent on create, or when it changes (tracked by a SHA-256 annotation).
3. Creates or updates one Grafana repository per GitHub repository:
   - Name: `<owner>-<repo>`, shortened with a hash suffix if needed.
   - Title: `owner/repo`.
   - Syncs `grafana/` from the default branch every 60 seconds, into a folder named after the repository.
   - `workflows: []`, which makes it **read-only**: Grafana never commits or opens pull requests.
   - The description notes that this job manages it.
4. Deletes managed repositories whose GitHub repository is gone, archived, or removed from the installation. Grafana then removes the dashboards they provisioned.

Only objects labelled `app.kubernetes.io/managed-by: grafana-git-sync` with this job's installation ID are changed or deleted. Objects that already exist with the same name but without those labels are left alone and reported as errors. Manual changes to managed objects are overwritten on the next run.

The job exits non-zero if any repository fails. If listing repositories fails, it stops before touching Grafana, so a partial list never triggers pruning.

### Why not the Grafana UI?

Grafana requires the GitHub App to have Contents **write** and Pull requests **write**, even for read-only repositories. It only enforces this on dry-run requests, which the UI always sends first. This job creates objects directly, so the app can stay read-only. Expect the connection to be shown as unhealthy in Grafana ("lacks required 'contents' permission"); sync still works. See [grafana/grafana#137435](https://github.com/grafana/grafana/issues/137435).

The setup wizard also can't create repositories for connections with webhooks disabled ([grafana/grafana#136077](https://github.com/grafana/grafana/issues/136077)).

## Prerequisites

1. **GitHub App** with repository permissions **Contents: read** and **Metadata: read**, webhooks inactive, installed on the repositories to sync. Note its app ID, installation ID (from `https://github.com/settings/installations/<id>`), and a generated private key.
2. **Grafana service account** with the Admin role, and a token for it.
3. **Kubernetes Secret** in the release namespace:

   ```sh
   kubectl create secret generic grafana-git-sync \
     --from-literal=grafana-token="$GRAFANA_TOKEN" \
     --from-file=github-app-private-key=app.private-key.pem
   ```

4. Delete any hand-made Git Sync repository or connection with the same name. The job refuses to take them over.

## Install

The chart is installed manually by a platform engineer:

```sh
helm upgrade --install grafana-git-sync \
  oci://europe-north1-docker.pkg.dev/nais-io/nais/grafana-git-sync \
  --version <chart-version> \
  --namespace <grafana-namespace> \
  --set image.tag=<image-version> \
  --set grafana.url=http://<grafana-service>.<namespace>.svc.cluster.local \
  --set github.appID=<app-id> \
  --set github.installationID=<installation-id> \
  --set existingSecret.name=grafana-git-sync
```

Image versions are listed in each "Build and push image" workflow run summary. See [`values.yaml`](charts/grafana-git-sync/values.yaml) for all options.

Trigger a run immediately instead of waiting for the schedule:

```sh
kubectl create job --from=cronjob/grafana-git-sync grafana-git-sync-manual
```

## Development

Tooling and tasks are managed with [mise](https://mise.jdx.dev):

| Task | Command |
|---|---|
| Build | `mise run build` |
| Test | `mise run test` |
| Format | `mise run fmt` |
| All checks | `mise run check` |

### Release

- **Image:** every push to `main` builds, signs, and pushes `europe-north1-docker.pkg.dev/nais-io/nais/images/grafana-git-sync`.
- **Chart:** changes under `charts/` push the chart to `oci://europe-north1-docker.pkg.dev/nais-io/nais`. Bump `version` in `Chart.yaml` manually; pushing an existing version fails.

CI pushes as `gh-grafana-git-sync@nais-io.iam.gserviceaccount.com` through the `NAIS_IO_WORKLOAD_IDENTITY_PROVIDER` secret. Provision that service account and its Artifact Registry write access in the nais-io Terraform.
