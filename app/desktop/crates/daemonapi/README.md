# daemonapi

Layer L1 (logic, gpui-free). See `docs/plans/2026-11-18-rust-tree.md` section 4.7.

## Owns

Talking to one Crowbar daemon through `protocol::DaemonTransport`: a request and its reply
envelope (`{success, data, error, code}`), the route builders, client-minted ids, multipart
uploads, and the daemon's `/v0/stream` dialled through the shell's loopback relay and read as
frames. It is not `transport`: transport implements the port (sockets, TLS, relay); this crate
speaks the daemon's API through it.

## Public interface

- `DaemonClient`: `new(transport, host)`, `send(Call)` (the envelope's `data`), `bytes(Call)`.
- `Call`: `get`, `post`, `put`, `patch`, `delete`, then `json`, `yaml`, `form`, `idempotent`,
  `discarding_work`.
- `Error` (Request, Refused, Body, Json, Stream) and `Result`.
- Routes and reading: `routes` (project, repos, scope, chats, folders), `envelope::unwrap`,
  `routes::document::content`, `routes::query::encode`, `read::read`, `decode::typed`,
  `new_id::uuid`,
  `multipart::file_field`.
- The stream: `Dial` and `Opening`, `RelayDial`, `Connection`, `Frame` and `frame::decode`,
  `CORE_ORIGIN`.
- Daemon data: `EntryKind`, `Switch`, `Problem`.
- Feature `fake`: `FakeDaemon` (a transport answering canned routes, recording each `Seen`
  request) and `local()`, for the unit tests here and in `store`.

## Depends on

- `protocol`: the `DaemonTransport` port, host ids and relay scopes.
- External: `tokio`, `tokio-tungstenite` and `futures-util` (the stream), `getrandom` (ids),
  `serde`/`serde_json`, `thiserror`, `tracing`.
