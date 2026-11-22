# oxynote

The official chart for [Oxynote](https://github.com/oxynote/oxynote). It
runs the all-in-one image `ghcr.io/oxynote/oxynote` as one pod with a
persistent volume. The image's
[README](https://github.com/oxynote/oxynote/blob/main/docker/prod/README.md)
documents every `OXYNOTE_*` variable.

## Prerequisites

- Kubernetes 1.29 or later, and Helm 3.22 or 4
- a default StorageClass, or an existing PVC
- an Ingress controller or a Gateway, to serve a real domain

## Install

```sh
helm install oxynote oci://ghcr.io/oxynote/helm/oxynote \
  --namespace oxynote --create-namespace \
  --set publicUrl=https://docs.example.com
```

The commands in this README assume the release `oxynote` in the namespace
`oxynote`. `publicUrl` is required. It is the address users open, and the
app builds its links and cookies from it. To try the chart without a
domain, set it to `http://localhost:8080` and run the `port-forward`
command the install prints.

## Verifying the chart

Each release is signed in its release workflow. Check it before you install:

```sh
cosign verify ghcr.io/oxynote/helm/oxynote:1.2.3 \
  --certificate-identity-regexp '^https://github.com/oxynote/oxynote/\.github/workflows/release\.yml@refs/tags/v' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

## Configuration

Non-secret settings go in `env`, and secret ones in a Secret named by
`envFromSecret`. Create the Secret before you install.

```yaml
publicUrl: https://docs.example.com
env:
  OXYNOTE_EMAIL_FROM_ADDRESS: docs@example.com
envFromSecret: oxynote-secrets
```

```sh
kubectl --namespace oxynote create secret generic oxynote-secrets \
  --from-literal=OXYNOTE_SMTP_DSN='smtp://user:password@mail.example.com:587'
```

When a Secret's key names are not yours to choose, such as a database
operator's, map one key with `extraEnv`:

```yaml
extraEnv:
  - name: OXYNOTE_DB_DSN
    valueFrom:
      secretKeyRef: {name: oxynote-db-app, key: uri}
```

A change to a Secret reaches the pod after
`kubectl --namespace oxynote rollout restart statefulset/oxynote`.

The GitHub App's private key is a file. Put it in a Secret under the key
`private-key.pem` and name that Secret in `githubAppSecret`. Mount any
other file, such as a private CA for `OXYNOTE_SOCIAL_LOGIN_OIDC_CA_CERTS`,
with `extraVolumes` and `extraVolumeMounts`.

While `OXYNOTE_MAX_ORGANIZATIONS` is `1`, the default, the first boot
creates the admin `admin@example.com` with the password
`oxynote-admin-1234`. Change it right away.

## Exposing it

Turn on `ingress` or `httpRoute`. Both send every path to the one Service.

```yaml
httpRoute:
  enabled: true
  parentRefs:
    - name: public
      namespace: gateways
  hostnames:
    - docs.example.com
```

Realtime editing uses a websocket on `/auth-realtime/hocuspocus`, so the
controller must pass websocket upgrades on. Files up to 25 MB can be
uploaded, so allow request bodies of at least 30 MB. Some controllers
default to 1 MB.

Oxynote does not rate-limit requests. Limit sign-in attempts at the
controller, on `/api/auth/sign-in/`, `/api/auth/sign-up/`,
`/auth-realtime/api/auth/sign-in/` and `/auth-realtime/api/auth/sign-up/`.

## Upgrading

The chart version is the image version, so chart 1.2.3 runs image 1.2.3.
The pod keeps its state on one volume, so the chart runs one pod and an
upgrade has a short outage. If an upgrade never becomes ready,
`helm rollback` alone does not replace the broken pod. Delete it with
`kubectl --namespace oxynote delete pod oxynote-0`.

## Backups

The volume holds the embedded database and the secrets the image
generates. Losing the secrets loses every stored data-source credential, so
back up both:

```sh
kubectl --namespace oxynote exec statefulset/oxynote -- sh -c \
  'PGPASSWORD="$(cat /oxynote/data/secrets/database-password)" \
  /usr/libexec/postgresql18/pg_dump -h /tmp/postgresql -U oxynote oxynote' > oxynote.sql
kubectl --namespace oxynote exec statefulset/oxynote -- \
  tar -C /oxynote/data -c secrets > oxynote-secrets.tar
```

## Uninstalling

`helm uninstall` keeps the PVC. Delete it by hand to remove the data.

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| image.repository | string | `"ghcr.io/oxynote/oxynote"` |  |
| image.tag | string | `""` | Empty means the chart's appVersion. |
| image.digest | string | `""` | Image digest (`sha256:...`). When set, it is used instead of the tag. |
| image.pullPolicy | string | `"IfNotPresent"` |  |
| publicUrl | string | required | The origin users open Oxynote at, with no path, such as `https://docs.example.com`. It becomes `OXYNOTE_PUBLIC_URL`. For a port-forward, use `http://localhost:8080`. |
| env | object | `{}` | Non-secret `OXYNOTE_*` variables, as the image's README lists them. A name the image does not know fails the install. Values are strings, so quote numbers and booleans. Secret ones go in `envFromSecret`. |
| envFromSecret | string | `""` | Name of an existing Secret with the secret `OXYNOTE_*` variables, such as `OXYNOTE_SMTP_DSN`. Each key becomes a variable. |
| extraEnv | list | `[]` | `OXYNOTE_*` variables read with `valueFrom`, for a Secret whose key names you do not choose, such as a database operator's `uri`. |
| githubAppSecret | string | `""` | Name of a Secret that holds the GitHub App's private key under the key `private-key.pem`. |
| persistence.existingClaim | string | `""` | Name of an existing PVC to use. The chart then creates none. |
| persistence.size | string | `"10Gi"` | Size of the PVC the chart creates. |
| persistence.storageClass | string | `""` | Storage class of the PVC. Empty means the cluster default. |
| persistence.annotations | object | `{}` | Extra annotations on the PVC the chart creates. |
| service.type | string | `"ClusterIP"` |  |
| service.port | int | `8080` |  |
| service.annotations | object | `{}` |  |
| ingress.enabled | bool | `false` |  |
| ingress.className | string | `""` |  |
| ingress.annotations | object | `{}` |  |
| ingress.hosts | list | `[]` | Hosts the Ingress serves. Each one routes every path to Oxynote. |
| ingress.tls | list | `[]` | TLS entries as in the Ingress spec: `secretName` and `hosts`. |
| httpRoute.enabled | bool | `false` |  |
| httpRoute.parentRefs | list | `[]` | Gateways to attach to. Required when enabled. |
| httpRoute.hostnames | list | `[]` |  |
| httpRoute.annotations | object | `{}` |  |
| resources | object | `{}` | Set at least `requests.memory`, since the pod runs Postgres. |
| serviceAccount.create | bool | `true` |  |
| serviceAccount.name | string | `""` | Empty means the release's full name when created, and `default` otherwise. |
| serviceAccount.annotations | object | `{}` |  |
| extraVolumes | list | `[]` | Extra volumes for the pod, such as a ConfigMap with a CA file. |
| extraVolumeMounts | list | `[]` |  |
| nodeSelector | object | `{}` |  |
| tolerations | list | `[]` |  |
| affinity | object | `{}` |  |
| podAnnotations | object | `{}` |  |
| podLabels | object | `{}` |  |
