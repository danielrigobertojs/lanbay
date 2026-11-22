#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
set -euo pipefail

# Mutating/opening paths is security-sensitive. These are the reviewed
# implementation boundaries; tests are intentionally excluded.
pattern='os\.(Remove(All)?|Rename|Create|Open(File)?|Mkdir(All)?)|windows\.CreateFile'

# Use grep (available on both macOS and the ubuntu CI runner) instead of
# ripgrep, which is not guaranteed to be installed.
while IFS= read -r file; do
	[ -z "$file" ] && continue
	file="${file#./}"
	case "$file" in
		internal/pathguard/*|internal/files/*)
			;;
		# internal/uploads: destination validated with guard.CheckNew in Create;
		# the temporary name (.lanbay-upload-<id>.part) is server-generated,
		# never client-controlled. Residual TOCTOU between CheckNew and the
		# os.OpenFile/os.Rename is documented in JCB-474 and tracked for
		# handle-based hardening (see internal/files for the reference pattern).
		internal/uploads/*)
			;;
		# internal/software/backend_windows.go: os.MkdirAll/Temp/RemoveAll y windows.CreateFile
		# solo en resolveWingetPath/readAppExecLink.
		# internal/software/staging.go: todas las operaciones de disco sobre staging
		# (MkdirAll, Stat, Open, Remove, EvalSymlinks) a través de ResolveStagingPath.
		# internal/software/finalpath_windows.go: windows.CreateFile con FILE_FLAG_OPEN_REPARSE_POINT
		# para canonicalizar la ruta (GetFinalPathNameByHandle) sin seguir reparse points.
		internal/software/backend_windows.go|internal/software/staging.go|internal/software/finalpath_windows.go|internal/software/finalpath_unix.go)
			;;
		internal/config/datadir_unix.go|internal/config/datadir_windows.go|internal/server/server.go|internal/server/tls.go|spikes/*|tools/kitpack/main.go|tools/kitserver/main.go)
			;;
		*)
			echo "unreviewed filesystem entry point: $file" >&2
			exit 1
			;;
	esac
done < <({ grep -rlE --include='*.go' --exclude='*_test.go' --exclude-dir=.git --exclude-dir=node_modules --exclude-dir=vendor "$pattern" . 2>/dev/null || true; } | sort)
