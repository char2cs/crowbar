# DaemonResponse: whole bodies only

Status: DRAFT. Gap found in the round 1 review (finding M11), not fixed in code.

## Problem

`DaemonTransport::request` answers with one `DaemonResponse` whose body is a `Vec<u8>`. The transport buffers the daemon's answer up to a limit (1 GiB local, the remote limit for paired hosts) before the pane sees a byte. Large downloads (attachments, archives, diffs of big repositories) therefore use memory proportional to their size, in the transport and again in the shell, and show no progress.

The Tauri originals streamed local bodies. Nothing in the shell calls for streaming yet, so the gap is recorded, not papered over with a flag.

## Constraints

- Panes read answers through the custom protocol, which hands the platform a response; each platform webview API takes a whole body or a stream differently.
- The owner of state is the transport: a streaming body must be pulled by the shell, never pushed into an unbounded queue.

## Proposed direction

1. `DaemonResponse` keeps its head (status, headers) and gains a body that is either whole or a bounded-channel stream owned by the transport.
2. `Responder` answers once with the head; the body is then read through a handle that applies back-pressure and is cancelled when the webview request is dropped.
3. The webview crate adapts the stream to each platform (WebKit and WebView2 accept streamed bodies; others buffer).
4. Until then, whole-body answers stay capped, and pane code that needs large transfers uses the WebSocket relay.

## Decision needed

Whether any planned feature needs a body larger than the cap before the streaming design is built. If none, leave this plan open.
