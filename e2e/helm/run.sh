#!/bin/sh
# test the Helm chart on a throwaway kind cluster: ct install, a smoke test
# through the Ingress and the HTTPRoute, and an upgrade from the release
# before IMAGE_TAG. IMAGE_TAG defaults to the latest release. The cluster is
# deleted on exit, also after a failure.
#
# usage: e2e/helm/run.sh [IMAGE_TAG]
set -eu

cd "$(dirname "$0")/../.."

tags=$(gh release list --exclude-drafts --limit 20 --json tagName -q '.[].tagName' | sed 's/^v//')
image_tag=${1:-$(echo "$tags" | head -1)}
# a release being published has no GitHub release yet, so the newest one
# is the one before it.
previous_tag=$(echo "$tags" | awk -v tag="$image_tag" 'found { print; exit } $0 == tag { found = 1 }')
previous_tag=${previous_tag:-$(echo "$tags" | head -1)}
cluster=oxynote-helm
namespace=oxynote
chart=helm
# the newest Gateway API release the pinned Traefik supports.
gateway_api=v1.6.3
traefik_chart=42.4.1
tmp=$(mktemp -d)
export KUBECONFIG="$tmp/kubeconfig"

finish() {
	status=$?
	if [ "$status" -ne 0 ]; then
		kubectl get pods --all-namespaces >&2 || true

		for pod in $(kubectl --namespace "$namespace" get pods -o name 2>/dev/null); do
			echo "--- $pod" >&2
			kubectl --namespace "$namespace" logs "$pod" --tail 50 >&2 || true
		done
	fi

	kind delete cluster --name "$cluster" >/dev/null 2>&1 || true
	rm -rf "$tmp"
	exit "$status"
}
trap finish EXIT
# dash runs the EXIT trap on an interrupt only when the shell exits itself.
trap 'exit 130' INT TERM

# test_release FILE HOST [playwright flags]
test_release() {
	file=$1
	host=$2
	shift 2
	(cd e2e && E2E_BASE_URL="http://$host:19280" \
		pnpm exec playwright test -c playwright.helm.config.ts "tests/$file" "$@")
}

echo "==> creating the cluster"
kind create cluster --name "$cluster" --config e2e/helm/cluster.yaml --wait 120s

echo "==> installing the Gateway API and Traefik"
kubectl apply --server-side -f \
	"https://github.com/kubernetes-sigs/gateway-api/releases/download/$gateway_api/standard-install.yaml" >/dev/null
helm install traefik oci://ghcr.io/traefik/helm/traefik --version "$traefik_chart" \
	--namespace traefik --create-namespace --values e2e/helm/traefik-values.yaml \
	--wait --timeout 5m >/dev/null

echo "==> starting the external Postgres"
kubectl apply -f e2e/helm/postgres.yaml >/dev/null
kubectl --namespace ci-deps rollout status deployment/postgres --timeout 5m

echo "==> preparing the restricted namespace"
kubectl create namespace "$namespace" >/dev/null
kubectl label namespace "$namespace" \
	pod-security.kubernetes.io/enforce=restricted \
	pod-security.kubernetes.io/warn=restricted >/dev/null
kubectl --namespace "$namespace" create secret generic oxynote-ci-db \
	--from-literal=OXYNOTE_DB_DSN='postgresql://oxynote:oxynote@postgres.ci-deps.svc:5432/oxynote?sslmode=disable' >/dev/null

echo "==> ct install, one release per ci values file"
ct install --charts "$chart" --namespace "$namespace" \
	--helm-extra-set-args "--set=image.tag=$image_tag" \
	--helm-extra-args "--timeout 15m"

for route in ingress httproute; do
	echo "==> smoke test through the $route"
	helm install "smoke-$route" "$chart" --namespace "$namespace" \
		--values "$chart/ci/$route-values.yaml" --set "image.tag=$image_tag" \
		--wait --timeout 15m >/dev/null
	test_release helm-smoke.test.ts "$route.oxynote.localhost"
	helm uninstall "smoke-$route" --namespace "$namespace" --wait >/dev/null
done

echo "==> upgrade from $previous_tag to $image_tag"
export E2E_HELM_STATE="$tmp/upgrade.json"
helm install upgrade "$chart" --namespace "$namespace" \
	--values "$chart/ci/ingress-values.yaml" --set "image.tag=$previous_tag" \
	--wait --timeout 15m >/dev/null
test_release helm-upgrade.test.ts ingress.oxynote.localhost -g "before the upgrade"

helm upgrade upgrade "$chart" --namespace "$namespace" \
	--values "$chart/ci/ingress-values.yaml" --set "image.tag=$image_tag" \
	--wait --timeout 15m >/dev/null

running=$(kubectl --namespace "$namespace" get pod upgrade-oxynote-0 \
	-o jsonpath='{.spec.containers[0].image}')
if [ "$running" != "ghcr.io/oxynote/oxynote:$image_tag" ]; then
	echo "the upgraded pod runs $running" >&2
	exit 1
fi

test_release helm-upgrade.test.ts ingress.oxynote.localhost -g "after the upgrade"

echo "==> all helm e2e checks passed"
