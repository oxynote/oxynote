# AGENTS.md

Guidance for `charts/oxynote/`, the Helm chart for the all-in-one image
([docker/prod](../../docker/prod/AGENTS.md)). Shared principles live in the
root [AGENTS.md](../../AGENTS.md).

## Invariants

- **One pod, always.** The image runs Postgres on the data volume, and two
  pods would corrupt it. So the chart renders a StatefulSet with
  `replicas: 1` and no value to change it, and never a Deployment, a
  PodDisruptionBudget or an autoscaler. `scripts/chart-check.sh` fails on
  any other kind.
- **Persistence cannot be turned off.** The volume holds the database and
  the secrets the image generates.
- **The chart never generates or looks up a secret.** Two renders of the
  same values must be identical, or Argo CD and Flux see drift. Credentials
  reach the pod only through `envFromSecret`.
- **`env` takes only the launcher's declared, non-secret names.** The
  schema lists them under `env`, and `scripts/check-chart-env.mjs` keeps
  the secret-bearing ones. It compares both lists with
  `docker/prod/launcher/src/env.ts`.
- **`version: 0.0.0` stays in git.** The release sets the chart version and
  the app version to the tag (`scripts/chart-publish.sh`), so chart 1.2.3
  always runs image 1.2.3. A breaking chart change goes in the release
  notes.
- **The pod meets the restricted Pod Security Standard**, the helm test pod
  included. The kind run installs into a namespace that enforces it.

## Files

- `values.schema.json` is written by hand, draft-07 for Helm 3, with
  `additionalProperties: false` on every object the chart defines. The
  Kubernetes objects it passes through stay open.
- `README.md` is generated: edit `README.md.gotmpl` and the `# --`
  comments in `values.yaml`, then run
  `helm-docs --chart-search-root charts --sort-values-order file`.
- `tests/` holds the helm-unittest suites, and `ci/` one values file per
  `ct install` release.

## Commands

```sh
make chart-check   # render-only checks: lint, unittest on Helm 4 and 3, kubeconform, README, env names
make e2e-kind      # the chart on kind: ct install, smoke and upgrade tests
helm unittest --strict charts/oxynote
```

Tools: helm 4 with the helm-unittest plugin (`--verify=false`), ct,
kubeconform, helm-docs and docker for `chart-check`; kind, kubectl and gh
for `e2e-kind`.
