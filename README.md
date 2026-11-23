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

## Installation (Windows)

Run from an elevated PowerShell or Command Prompt:

```text
lanbay.exe install
```

`install` creates `%ProgramData%\Lanbay` with an Administrators + SYSTEM-only ACL,
registers the `Lanbay` scheduled task (at logon, interactive, highest privileges,
hidden) and adds an inbound firewall rule for the configured HTTPS port **only on
the Private profile**. It also generates the initial administrator password and
writes it to `%ProgramData%\Lanbay\initial-password.txt` (protected). Change it
after the first login.

Lanbay starts only after you sign in; there is no panel before logon. Drive
letters created with `subst` are not shown (see ADR-006).

To remove it:

```text
lanbay.exe uninstall
```

`uninstall` removes the scheduled task and the firewall rule and asks whether to
keep the data directory. Managed applications installed by the panel are **not**
uninstalled.

## Trusting the certificate

Lanbay uses a self-signed TLS certificate. Its SHA-256 fingerprint is printed on
startup and stored in the panel. To avoid browser warnings:

- **macOS:** open Keychain Access, import `tls-cert.pem` into the System
  keychain, then set it to "Always Trust" for SSL.
- **iOS/iPadOS:** install the profile, then enable full trust under
  Settings > General > About > Certificate Trust Settings.
- **Windows (client):** import into "Trusted Root Certification Authorities".

## Windows SmartScreen / Defender

The binary is not code-signed. Windows SmartScreen and Defender may warn about
an unknown publisher; choose "More info" > "Run anyway" only after verifying the
SHA-256 checksum from the release. See `SECURITY.md`.

## Windows M0 bootstrap kit

Before Lanbay is installed, its M0 spikes must be run on the target Windows
device. From a Mac on the same private LAN, build and serve a precompiled kit:

```sh
make ally-kit
go run ./tools/kitserver
```

The server prints one command. Open **PowerShell as Administrator** on the
Windows device and paste it. The short-lived server uses a random token, serves
only private or link-local LAN clients, and stops after receiving the results
or after 30 minutes of inactivity. The bootstrap downloads the kit, verifies
its SHA-256 before extracting or running anything, then guides the required
microSD, test MSI, and destructive-action confirmations. It uploads an
unchanged result archive to `m0-results/<timestamp>/` on the Mac.

This is an M0-only convenience flow; it does not install Lanbay. It deliberately
uses HTTP because the bootstrapping command itself is served by the local Mac.
The token and pinned kit hash reduce exposure, but do not provide TLS-grade
protection against an attacker controlling the LAN. Treat that as an accepted,
time-limited M0 risk; do not use this server to distribute Lanbay releases.

See [the M0 spike guide](spikes/README.md) for prerequisites, the manual
alternative, cleanup, and the Windows-device evidence still required.

## Dependencies

- `modernc.org/sqlite`: pure-Go SQLite driver.
- `golang.org/x/sys/windows`: Windows API bindings.
- `golang.org/x/crypto/argon2`: memory-hard administrator password hashing.
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
