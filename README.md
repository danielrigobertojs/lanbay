# Lanbay

Lanbay is a local HTTPS web panel for Windows 10/11 x64 that manages local
storage and software installed by the panel itself. It is designed to run as a
single Go binary; its Svelte interface is compiled and embedded, with no CDN
or external runtime service.

This repository is in its foundation stage. Windows-only behavior requires
verification on the target device and is never considered verified from macOS.

## Build

Prerequisites: current stable Go, Node.js/npm, and `staticcheck`.

```text
npm --prefix web ci
make windows
dist/lanbay.exe version
```

`make windows` cross-compiles with `CGO_ENABLED=0` for `windows/amd64`.

## Dependencies

- `modernc.org/sqlite`: pure-Go SQLite driver.
- `golang.org/x/sys/windows`: Windows API bindings.
- Svelte: compiled local web UI.

Complete generated attribution is in `THIRD_PARTY_LICENSES.md` and is named by
`lanbay version`.

## Security and status

Lanbay is LAN-only by design. Do not expose it to the internet. It is not yet
released or signed; Windows SmartScreen or Defender can warn about an unsigned
binary. See `SECURITY.md` for private vulnerability reporting.

## License

Copyright 2026 Daniel Rigoberto Jacobo Sandoval. Licensed under Apache-2.0;
see `LICENSE`, `NOTICE`, and `TRADEMARK.md`.
