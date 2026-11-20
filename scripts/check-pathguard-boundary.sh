#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
set -euo pipefail

# Mutating/opening paths is security-sensitive. These are the reviewed
# implementation boundaries; tests are intentionally excluded.
pattern='os\.(Remove(All)?|Rename|Create|Open(File)?|Mkdir(All)?)|windows\.CreateFile'
while IFS= read -r file; do
	case "$file" in
		internal/pathguard/*|internal/files/*|internal/config/datadir_unix.go|internal/config/datadir_windows.go|internal/server/server.go|internal/server/tls.go|spikes/*|tools/kitpack/main.go|tools/kitserver/main.go)
			;;
		*)
			echo "unreviewed filesystem entry point: $file" >&2
			exit 1
			;;
	esac
done < <(rg -l "$pattern" --glob '*.go' --glob '!*_test.go' . | sed 's|^./||' | sort)
