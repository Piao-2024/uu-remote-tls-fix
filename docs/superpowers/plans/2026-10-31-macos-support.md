# macOS Support Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [x]`) syntax for tracking.

**Goal:** Add installable macOS support without modifying the Windows implementation.

**Architecture:** A Go SOCKS5 service transports UU HTTPS and fragments its TLS
ClientHello. Bash scripts manage a per-user LaunchAgent; UU proxy setup is manual.

**Tech Stack:** Go 1.25 standard library, macOS Bash 3.2, launchd, GitHub Actions.

**Spec:** `docs/superpowers/specs/2026-10-31-macos-support-design.md`

## Global Constraints

- Preserve existing Windows files and pre-existing BAT changes.
- Loopback only, UU hostnames / TCP 443 only, TLS certificate validation preserved.
- No system proxy, hosts, DNS, UU configuration or certificate-store edits.
- macOS 12+, Apple Silicon and Intel distribution; no third-party Go dependencies.

## Review Focus

- Fragmented TCP reads and malformed TLS lengths must not corrupt the handshake.
- SOCKS method / command / address errors must return a protocol error promptly.
- Failed DNS and bad nodes must fall back without accepting private/fake addresses.
- Installation must not replace an unrelated process using the selected port.
- Uninstallation must preserve unrelated files and never rewrite UU preferences.

### Task 1: Service and protocol tests

**Files:** `macos/{go.mod,main.go,proxy.go,tls.go,resolver.go,interface_darwin.go,main_test.go}`.
**Interfaces:** `fragmentClientHello([]byte) []byte`, `readTLSRecord(io.Reader) ([]byte,error)`,
`readSOCKSRequest(io.ReadWriter) (string,error)`, `newResolver(*http.Client,string)` and
`(*resolver).resolve(context.Context,string) (string,error)`.

- [x] Write tests using real generated ClientHello bytes, chunked readers and a local HTTP DNS server.
- [x] Run `go test ./...`; confirm missing-feature failures before implementation.
- [x] Implement framing, SNI parsing, SOCKS validation, resolver and service lifecycle.
- [x] Run `go test -race ./...` and `go vet ./...`; build darwin/arm64 and darwin/amd64.

### Task 2: Install, uninstall and diagnose

**Files:** `install-macos.sh`, `uninstall-macos.sh`, `test-macos.sh`, `tests/macos-scripts.sh`.
**Interfaces:** Installed binary `$HOME/Library/Application Support/UURemoteTlsFix/uu-tls-fragmenter`,
LaunchAgent `com.uu-remote-tls-fix.fragmenter`, default SOCKS port `17899`.

- [x] Write tests with an isolated `UU_TLS_FIX_HOME` root with a fake launchctl and supplied test binary.
- [x] Run the shell tests before the scripts exist to establish failure.
- [x] Implement source/prebuilt installation, rendered plist, owned-file removal and endpoint diagnostics.
- [x] Run `bash -n` and shell tests; validate generated plist with `plutil`.

### Task 3: Docs, CI and delivery

**Files:** `README.md`, `docs/macos.md`, `.gitignore`, `.gitattributes`, `.github/workflows/macos.yml`.
**Interfaces:** Source build / explicit binary install; tag `macos-v*` produces archives and SHA256SUMS.

- [x] Document setup and recovery accurately; separate Windows/macOS entry points.
- [x] Add CI for Go race tests, shell lifecycle tests and both architecture builds.
- [x] Run final unit, lifecycle, cross-build and live six-endpoint verification.
- [x] Review the complete change. Deliver only macOS-related files on the feature branch; commit/push status is recorded in Git and the PR.

## Verification record

Core Go race tests and vet passed. Installer lifecycle and plist validation passed.
Both Darwin architectures built. The formal arm64 binary reached all six official
HTTPS endpoints with certificate verification enabled. A fresh static reviewer found
no blocking issue. The user requested lean verification; additional tunnel integration
tests were omitted. No live LaunchAgent was installed on the user’s Mac.
