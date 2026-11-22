#!/bin/sh
# test the Helm chart on a throwaway kind cluster: ct install, a smoke test
# through the Ingress and the HTTPRoute, an upgrade from the release before
# IMAGE_TAG, and a backup and restore with the README's commands when
# IMAGE_TAG has them. IMAGE_TAG defaults to the latest release. A tag that is no release, such as
# e2e-prod, names a local image, which is loaded into the cluster. The
# cluster is deleted on exit, also after a failure.
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

# readme_block TEXT prints the sh block of the chart's README that holds
# TEXT, so the test runs the commands an operator copies.
readme_block() {
	awk -v text="$1" '
		/^```sh$/ { block = ""; inside = 1; next }
		/^```$/ { if (inside && index(block, text)) printf "%s", block; inside = 0; next }
		inside { block = block $0 "\n" }
	' "$chart/README.md"
}

# restarts POD prints how often the pod's container has restarted.
restarts() {
	kubectl --namespace "$namespace" get pod "$1" \
		-o jsonpath='{.status.containerStatuses[0].restartCount}'
}

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

if ! echo "$tags" | grep -qx "$image_tag"; then
	echo "==> loading the local image ghcr.io/oxynote/oxynote:$image_tag"
	kind load docker-image --name "$cluster" "ghcr.io/oxynote/oxynote:$image_tag"
fi

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

# a release from before the commands has no backup to test.
if ! kubectl --namespace "$namespace" exec upgrade-oxynote-0 -- sh -c "command -v backup" >/dev/null; then
	echo "==> $image_tag has no backup command, so the backup and restore are skipped"
	echo "==> all helm e2e checks passed"
	exit 0
fi

echo "==> backup and restore with the README's commands"
# the README's commands name the release oxynote in the namespace oxynote.
readme_block "-- backup >" > "$tmp/backup.sh"
readme_block "-- restore <" > "$tmp/restore.sh"
test -s "$tmp/backup.sh" && test -s "$tmp/restore.sh"
export E2E_HELM_STATE="$tmp/backup.json"
install_backup_release() {
	helm install oxynote "$chart" --namespace "$namespace" \
		--values "$chart/ci/httproute-values.yaml" --set "image.tag=$image_tag" \
		--wait --timeout 15m >/dev/null
}
install_backup_release
test_release helm-backup-restore.test.ts httproute.oxynote.localhost -g "before the backup"
(cd "$tmp" && sh -eu backup.sh)

# the restore goes into a new volume, as after a lost one.
helm uninstall oxynote --namespace "$namespace" --wait >/dev/null
kubectl --namespace "$namespace" delete pvc oxynote-data --wait >/dev/null
install_backup_release
before=$(restarts oxynote-0)
(cd "$tmp" && sh -eu restore.sh)

# the restore restarts the pod, and its boot swaps the instance in.
tries=0
until [ "$(restarts oxynote-0)" -gt "$before" ]; do
	tries=$((tries + 1))
	if [ "$tries" -gt 150 ]; then
		echo "the pod did not restart after the restore" >&2
		exit 1
	fi
	sleep 2
done
kubectl --namespace "$namespace" wait --for=condition=Ready pod/oxynote-0 --timeout 15m >/dev/null
test_release helm-backup-restore.test.ts httproute.oxynote.localhost -g "after the restore"

echo "==> all helm e2e checks passed"
