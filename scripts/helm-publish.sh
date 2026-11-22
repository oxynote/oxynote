#!/bin/sh
# package the Helm chart at VERSION and push it to ghcr. It refuses a
# version that is already there, because a second push would change the
# digest that the signature and every install point to.
#
# usage: scripts/helm-publish.sh VERSION
set -eu

cd "$(dirname "$0")/.."

version=${1:?usage: scripts/helm-publish.sh VERSION}
registry=oci://ghcr.io/oxynote/helm
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

# helm show succeeds only for a version that exists. A failure other than
# "not found", such as auth or network, stops the release.
if helm show chart "$registry/oxynote" --version "$version" >"$tmp/show" 2>&1; then
	echo "$registry/oxynote $version is already published" >&2
	exit 1
fi

if ! grep -qi "not found" "$tmp/show"; then
	cat "$tmp/show" >&2
	exit 1
fi

SOURCE_DATE_EPOCH=$(git log -1 --format=%ct) helm package helm \
	--version "$version" --app-version "$version" --destination "$tmp" >/dev/null

if ! helm push "$tmp/oxynote-$version.tgz" "$registry" >"$tmp/push" 2>&1; then
	cat "$tmp/push" >&2
	exit 1
fi

digest=$(sed -n 's/^Digest: //p' "$tmp/push")
echo "digest=$digest" >>"${GITHUB_OUTPUT:-/dev/stdout}"
