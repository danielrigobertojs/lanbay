#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
set -euo pipefail

status=0
while IFS= read -r -d '' file; do
  case "$file" in
    *.svelte) marker='<!-- SPDX-License-Identifier: Apache-2.0 -->' ;;
    *) marker='// SPDX-License-Identifier: Apache-2.0' ;;
  esac
  if ! head -n 1 "$file" | grep -Fqx "$marker"; then
    printf 'missing SPDX header: %s\n' "$file" >&2
    status=1
  fi
done < <(find . -type f \( -name '*.go' -o -name '*.svelte' -o -name '*.ts' -o -name '*.js' \) \
  -not -path './web/node_modules/*' -not -path './internal/web/dist/*' -print0)

exit "$status"
