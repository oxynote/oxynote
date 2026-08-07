#!/bin/sh
set -eu

# changedetection.io generates its API key on first boot and only stores it
# inside its datastore volume — there is no way to preset it via env or CLI.
# This script reads the key out of the running changedetection container and
# writes it into core.local.env so core can call the changedetection API
# without a manual copy-paste from the UI. Run from the repository root with
# the changedetection container already started (see the Makefile).

env_file="docker/env/core.local.env"

if [ ! -f "$env_file" ]; then
	echo "sync-changedetection-key: $env_file not found — run 'make setup' first" >&2
	exit 1
fi

# the datastore file is written by a background save thread shortly after
# first boot, so poll instead of reading once.
key=""

for _ in $(seq 1 30); do
	key=$(docker exec changedetection python3 -c \
		"import json; print(json.load(open('/datastore/changedetection.json'))['settings']['application']['api_access_token'])" \
		2>/dev/null) && [ -n "$key" ] && break
	key=""
	sleep 1
done

if [ -z "$key" ]; then
	echo "sync-changedetection-key: could not read the API key from the changedetection container" >&2
	exit 1
fi

current=$(grep '^OXYNOTE_CORE_CHANGEDETECTION_API_KEY=' "$env_file" | cut -d= -f2-)

if [ "$current" != "$key" ]; then
	sed -i.bak "s/^OXYNOTE_CORE_CHANGEDETECTION_API_KEY=.*/OXYNOTE_CORE_CHANGEDETECTION_API_KEY=$key/" "$env_file"
	rm -f "$env_file.bak"
	echo "sync-changedetection-key: updated OXYNOTE_CORE_CHANGEDETECTION_API_KEY in $env_file"
fi
