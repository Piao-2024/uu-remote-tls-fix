# macOS support

## Intent and scope

Support UU Remote on macOS as a controller, using the SOCKS5 → TLS record
fragmentation → official-server path already verified on the local Mac.
The existing Windows implementation and pre-existing BAT changes stay untouched.

## Approaches

The agreed direction is a standard-library Go executable distributed for darwin/arm64
and darwin/amd64. Keeping C# would require a separate .NET runtime; Python would
require users to install Python. Go needs no runtime on the target Mac.

## Behavior

- Listen only on IPv4 loopback, default `127.0.0.1:17899`.
- Implement anonymous SOCKS5 CONNECT to approved UU hostnames on port 443.
- Read complete TLS records even when TCP delivers them in small pieces. Parse the
  ClientHello SNI extension; split inside `nrd.nie.163.com` without changing the
  handshake bytes. Other approved UU HTTPS hosts pass through unchanged.
- Resolve IPv4 through AliDNS HTTPS with certificate verification, a bounded cache,
  the existing API alias / signal ECS policies, and known official fallback nodes.
- Do not terminate client TLS, install certificates, change hosts, DNS or system
  proxies, or edit UU's undocumented shared configuration.
- Support an optional physical interface override; default to the system route.
- Install a per-user LaunchAgent. UU's own proxy setting is changed manually.
  Uninstall removes only files owned by this tool and reminds users to disable
  the UU proxy before stopping the service.
- Document tested scope accurately: local Apple Silicon / UU 4.42.0 controller
  worked with the spike; unattended control, UDP proxying and arbitrary IP targets
  are not claimed. Go source builds require macOS 12+ and Go 1.25+.

## Distribution and verification

Source installation builds locally; prebuilt binaries may be supplied explicitly.
CI tests and builds both Mac architectures; version tags publish archives and
checksums. No release is published as part of this implementation.

Tests cover byte-preserving TLS fragmentation, malformed/truncated input, SOCKS5
errors, node selection, concurrency and installer/uninstaller ownership. Verify
the formal binary with the six public endpoints. The user's earlier successful
remote-control test is evidence for the spike, not a claim that every future build
has been manually tested.
