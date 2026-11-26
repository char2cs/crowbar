# daemonapi

Layer L1 (logic, gpui-free). See `docs/rust-style.md` (crate layers).

## Owns

Talking to one Crowbar daemon through `protocol::DaemonTransport`: a request and its reply
envelope (`{success, data, error, code}`), the route builders, client-minted ids, multipart
uploads, and the daemon's `/v0/stream` dialled through the shell's loopback relay and read as
frames. It is not `transport`: transport implements the port (sockets, TLS, relay); this crate
speaks the daemon's API through it.

## Public interface

`lib.rs` re-exports types only; what were free functions are associated functions of the type
they serve.

- `Client`: `new(transport, host)`, `send(Call)` (the envelope's `data`), `bytes(Call)`,
  `read(path)` and `fetch(Call)` (the `data` as the type the route promises).
- `Call`: `get`, `post`, `put`, `patch`, `delete`, then `json`, `yaml`, `file` (a one-file
  multipart form), `idempotent`, `discarding_work`.
- `Route`: the daemon paths, `project`, `repos`, `scope`, `chats`, `folders`,
  `document_content`, `browse`; displays as the path and converts into a `Call`'s path.
- `Envelope`: `Envelope::unwrap(path, response)` for callers that send their own requests.
- `Uuid`: `Uuid::random()`, a client-minted id that displays in its hyphenated form.
- `Error` (Request, Refused, Body, Json, Stream) and `Result`.
- The stream: `Dial` and `Opening`, `RelayDial`, `Connection`, `Frame` (with `Frame::decode`),
  `CORE_ORIGIN`.
- Daemon data: `EntryKind`, `Switch`, `Problem`.
- Feature `fake`: `FakeDaemon` (a transport answering canned routes, recording each `Seen`
  request), for the unit tests here and in `store`. A `cfg(test)` fake would be invisible to
  `store`, so it is a feature (recorded in the design doc).

## Depends on

- `protocol`: the `DaemonTransport` port, host ids and relay scopes.
- External: `tokio`, `tokio-tungstenite` and `futures-util` (the stream), `getrandom` (ids),
  `serde`/`serde_json`, `thiserror`, `tracing`.
