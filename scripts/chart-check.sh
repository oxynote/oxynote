#!/bin/sh
# every render-only check of the Helm chart.
#
# usage: scripts/chart-check.sh
set -eu

cd "$(dirname "$0")/.."

chart=charts/oxynote
helm3_image=alpine/helm:3.22.1@sha256:8191701ad1f82ee3695c87211fcbfe6d7b1bb7df92ac1c8de9ccdb0995a6a2c1
kube_versions="1.34.0 1.35.0 1.36.0 1.37.0"
# CRDs-catalog has no releases, so the commit is the pin.
crds_catalog=https://raw.githubusercontent.com/datreeio/CRDs-catalog/784ec895547a36e548e6810b41f27baade3a0890
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

echo "==> chart metadata"
for line in 'apiVersion: v2' 'version: 0.0.0' 'appVersion: "0.0.0"' \
	'kubeVersion: ">=1.29.0-0"' '  - https://github.com/oxynote/oxynote'; do
	if ! grep -qxF -- "$line" "$chart/Chart.yaml"; then
		echo "Chart.yaml lacks: $line" >&2
		exit 1
	fi
done

echo "==> ct lint"
ct lint --charts "$chart" --validate-maintainers=false --check-version-increment=false \
	--helm-lint-extra-args=--strict

echo "==> unit tests on Helm 4"
helm unittest --strict "$chart"

echo "==> unit tests on Helm 3"
# helm plugin install clones the plugin with git, which the image lacks.
docker run --rm -v "$PWD/$chart:/chart:ro" -w /chart --entrypoint sh "$helm3_image" -c '
	set -e
	apk add --quiet git
	out=$(helm plugin install https://github.com/helm-unittest/helm-unittest --version v1.2.1 2>&1) || { echo "$out" >&2; exit 1; }
	helm lint --strict --quiet . --set publicUrl=https://docs.example.com
	helm unittest --strict .'

echo "==> kubeconform and reproducible renders"
for values in "$chart"/ci/*-values.yaml; do
	helm template oxynote "$chart" --values "$values" >"$tmp/a.yaml"
	helm template oxynote "$chart" --values "$values" >"$tmp/b.yaml"

	if ! cmp -s "$tmp/a.yaml" "$tmp/b.yaml"; then
		echo "two renders of $values differ" >&2
		exit 1
	fi

	for version in $kube_versions; do
		kubeconform -strict -kubernetes-version "$version" \
			-schema-location default \
			-schema-location "$crds_catalog/{{.Group}}/{{.ResourceKind}}_{{.ResourceAPIVersion}}.json" \
			"$tmp/a.yaml"
	done
done

echo "==> rendered kinds"
# a Deployment, a PodDisruptionBudget or an autoscaler would let two pods
# share the data volume or block every node drain.
kinds=$(helm template oxynote "$chart" --set publicUrl=https://docs.example.com \
	--set ingress.enabled=true --set 'ingress.hosts[0]=docs.example.com' \
	--set httpRoute.enabled=true --set 'httpRoute.parentRefs[0].name=public' \
	| sed -n 's/^kind: //p' | sort | tr '\n' ' ')
if [ "$kinds" != "HTTPRoute Ingress PersistentVolumeClaim Pod Service ServiceAccount StatefulSet " ]; then
	echo "unexpected kinds: $kinds" >&2
	exit 1
fi

echo "==> README"
cp "$chart/README.md" "$tmp/README.md"
helm-docs --chart-search-root charts --sort-values-order file --log-level warning
if ! cmp -s "$chart/README.md" "$tmp/README.md"; then
	echo "$chart/README.md was out of date. helm-docs has regenerated it." >&2
	exit 1
fi

echo "==> env names"
node scripts/check-chart-env.mjs

echo "==> reproducible package"
epoch=$(git log -1 --format=%ct)
for run in a b; do
	SOURCE_DATE_EPOCH=$epoch helm package "$chart" --version 1.2.3 \
		--app-version 1.2.3 --destination "$tmp/$run" >/dev/null
done

if ! cmp -s "$tmp/a/oxynote-1.2.3.tgz" "$tmp/b/oxynote-1.2.3.tgz"; then
	echo "two packages of the same commit differ" >&2
	exit 1
fi

echo "==> all chart checks passed"
