#!/bin/sh
set -eu

# `make setup` copies each *.example.env to its *.local.env only when the
# local file is missing, so a variable that is renamed, added or removed in
# a template never reaches an env file that already exists. Core then boots
# against a name nobody reads: at best a dependency silently disables
# itself, at worst it refuses to start. This script compares the variable
# names in every template against its local file — reporting the difference
# by default, rewriting the local file to match the template with --fix.
#
# --fix keeps the local value of every variable the template still lists,
# so secrets and local tweaks survive; variables the template has dropped
# are removed, which is the point. Run from the repository root.

fix=false

if [ "${1:-}" = "--fix" ]; then
	fix=true
elif [ $# -gt 0 ]; then
	echo "usage: sync-env.sh [--fix]" >&2
	exit 2
fi

keys() {
	grep -oE '^[A-Za-z_][A-Za-z0-9_]*=' "$1" | tr -d '='
}

example_keys=$(mktemp)
local_keys=$(mktemp)

trap 'rm -f "$example_keys" "$local_keys"' EXIT

drifted=false

for name in core auth-realtime web; do
	example="docker/env/$name.example.env"
	local_file="docker/env/$name.local.env"

	if [ ! -f "$local_file" ]; then
		echo "sync-env: $local_file not found — run 'make setup' first" >&2
		exit 1
	fi

	keys "$example" | sort > "$example_keys"
	keys "$local_file" | sort > "$local_keys"

	missing=$(comm -23 "$example_keys" "$local_keys")
	unknown=$(comm -13 "$example_keys" "$local_keys")

	if [ -z "$missing" ] && [ -z "$unknown" ]; then
		continue
	fi

	drifted=true

	if [ "$fix" = false ]; then
		echo "$local_file has drifted from $example:"
		echo "$missing" | sed '/^$/d; s/^/  missing: /'
		echo "$unknown" | sed '/^$/d; s/^/  unknown: /'

		continue
	fi

	# rebuild the local file from the template so it picks up new
	# variables, renames and comments, substituting the local value
	# wherever the template still lists the variable.
	awk '
		NR == FNR {
			if ($0 ~ /^[A-Za-z_][A-Za-z0-9_]*=/) {
				eq = index($0, "=")
				value[substr($0, 1, eq - 1)] = substr($0, eq + 1)
			}

			next
		}
		{
			if ($0 ~ /^[A-Za-z_][A-Za-z0-9_]*=/) {
				eq = index($0, "=")
				key = substr($0, 1, eq - 1)

				if (key in value) {
					print key "=" value[key]

					next
				}
			}

			print
		}
	' "$local_file" "$example" > "$local_file.new"

	mv "$local_file.new" "$local_file"

	echo "sync-env: rewrote $local_file from $example"
	echo "$missing" | sed '/^$/d; s/^/  added:   /'
	echo "$unknown" | sed '/^$/d; s/^/  dropped: /'
done

if [ "$drifted" = false ]; then
	echo "sync-env: env files are in sync"
elif [ "$fix" = false ]; then
	echo "run 'make sync-env' to bring them in sync"
	exit 1
fi
