#!/usr/bin/env bash
set -euo pipefail

required=(
  README.md CHANGELOG.md COMPATIBILITY.md SECURITY.md SUPPORT.md
  CONTRIBUTING.md CODE_OF_CONDUCT.md
  docs/README.md docs/api.md docs/assurance.md docs/composition.md
  docs/design.md docs/operations.md docs/policies.md docs/verification.md
  example_test.go breakertest/example_test.go window/example_test.go
)

for path in "${required[@]}"; do
  test -s "$path"
done

if grep -En '(^|[[:space:]])(_ =|[^[:space:]]+, _ :=|_, _ =)' \
  example_test.go breakertest/example_test.go window/example_test.go; then
  echo "executable examples must handle public errors" >&2
  exit 1
fi

while IFS=: read -r source match; do
  link="$(sed -E 's/.*\(([^)]+)\)/\1/' <<<"$match")"
  link="${link%%#*}"
  [[ -z "$link" || "$link" == http://* || "$link" == https://* ]] && continue
  target="$(dirname "$source")/$link"
  test -e "$target" || {
    echo "broken local documentation link: $source -> $link" >&2
    exit 1
  }
done < <(grep -REo '\[[^]]+\]\([^)]+\)' README.md SUPPORT.md SECURITY.md docs)

go test -run='^Example' ./...
go list -f '{{if .GoFiles}}{{.ImportPath}}{{end}}' ./... |
  xargs -n 1 go doc >/dev/null
