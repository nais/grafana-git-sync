# AGENTS.md - grafana-git-sync

Use mise for tools and tasks (`mise run test`, `mise run check`, `mise run fmt`). Commits follow Conventional Commits; the allowed types are feat, fix, ci, docs, refactor, perf, test, build and style.

## Grafana provisioning gotchas

These were found in Grafana v13.2.2's source (`apps/provisioning/pkg/connection/validator.go` and `.../connection/github/connection.go`). Check them again when Grafana is upgraded.

- **Read-only GitHub App:** Grafana requires the app to have Contents write and Pull requests write, but only checks this on dry-run requests. Create and update connections and repositories with normal requests, never `dryRun`. The connection is therefore always reported as unhealthy ("lacks required 'contents' permission"); syncing still works.
- **Webhooks:** `spec.webhook.disabled: true` must be set on both the connection and every repository that uses it. Grafana rejects the repository otherwise. Webhooks are disabled because IAP stops GitHub from reaching Grafana.
- **Private key:** Grafana expects a base64-encoded PEM. Send it only on create or when its hash annotation changes; resending it makes Grafana regenerate tokens and rerun health checks.
- **Idempotency:** an unchanged run must make no writes. The desired spec must therefore match what Grafana stores after its own rewrites: the connection `spec.url` becomes `https://github.com/settings/installations/<id>`, and repository URLs lose any trailing `/` and `.git`. `TestRunIsIdempotentAndDoesNotResendKey` guards this.
- **Names:** repository names are capped at 40 characters, because each synced folder is keyed by the repository name and Grafana folder UIDs max out at 40.

## Safety

- Deleting a Grafana repository deletes the dashboards it provisioned, through Grafana's finalizers. Only prune objects with both the managed-by label and this installation's ID label, and never prune after a failed or partial GitHub listing.
- Never adopt unlabelled objects: hand-made Git Sync setups must stay untouched.

## Deployment

A platform engineer installs it with Helm in Grafana's cluster; it is not deployed through Fasit. Bump the chart `version` manually, because CI fails when pushing an existing chart version.
