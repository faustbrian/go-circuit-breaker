#!/bin/sh
set -eu

module=$(go list -m -f '{{.Path}}')
baseline=$(git tag --list 'v[0-9]*' --sort=-v:refname | head -n 1)

if [ -z "$baseline" ]; then
	echo "API compatibility: no released baseline; current API establishes v1"
	exit 0
fi

if ! command -v apidiff >/dev/null 2>&1; then
	echo "apidiff is required when a release baseline exists" >&2
	echo "install: go install golang.org/x/exp/cmd/apidiff@latest" >&2
	exit 1
fi

apidiff "$module@$baseline" "$module"
