# transport

## Owns

Reaching daemons for panes and the shell: routing `crowbar://<host>/v0/...` requests, HTTP over the
trusted local socket or pinned TLS to paired remote hosts, their bearer tokens, pairing a new host,
and the token- and origin-checked loopback WebSocket relay that panes connect to.

## Public interface

- `Transport` (`handle`) and `ShellTransport` (the shell's `protocol::DaemonTransport`).
- `HostRegistry` and `Endpoint`: the paired hosts and how to reach them.
- `pair_remote_host`, `PairRequest`, `probe_fingerprint`, `fingerprint_of`: pairing a remote host.
- Ports: `TokenStore` (with `CachedTokenStore`, `MemoryTokenStore`), `Keychain` (and `OsKeychain`
  behind the `os-keychain` feature), `SocketLocator`, `StreamAccess`, `OriginPolicy`, `Acceptor`.
- `Relay`, `RelayConfig`, `ExactOrigins`, `TextStream` and the relay subprotocol constants.
- `local_get`, `EXTENSION_HEADER`, the proxy marker constants and `marker_of`, `ReadError`, `Error`.

## Depends on

- `protocol`: the daemon port and its request and response types.
- `atomicfile`: the hosts file.
