#!/bin/sh
# Run a command without its output: tick a line while it works, and
# replay the whole log only if it fails. goreleaser and docker are both
# very chatty, and none of what they say matters unless the step breaks.
#
# usage: run-quietly.sh <label> <command> [args...]

set -u

label=$1
shift

log=$(mktemp) || exit 1

"$@" >"$log" 2>&1 &
pid=$!

# an interrupt has to take the child with it and stop here — falling
# through would report a failure and replay a log that is already gone.
trap 'kill "$pid" 2>/dev/null; rm -f "$log"; exit 130' INT TERM
trap 'rm -f "$log"' EXIT

printf '  %s' "$label"

# kill -0 sends no signal, it only asks whether the process is still
# there; wait then reports the exit status the shell held on to.
while kill -0 "$pid" 2>/dev/null; do
	printf '.'
	sleep 1
done

if wait "$pid"; then
	printf ' ok\n'
else
	printf ' failed\n\n'
	cat "$log"
	exit 1
fi
