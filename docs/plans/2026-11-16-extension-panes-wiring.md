# Extension panes: what the shell serves today and what waits for the daemon

The daemon on this branch has no extension host: there is no pane registry, no status feed and
no call route. They exist only on `enhancement/rust-extensions-migration`
(`api/internal/core/extension/panes.go`, `POST /ext/:extId/call`, the event bridge of
`docs/plans/2026-11-14-extension-host-protocol.md`). The shell therefore cannot register extension
pane types or fill `ExtensionBundles` from a real source yet, and does not pretend to.

## Served now

- `request` is answered at once with a `response` error `Unavailable` ("no extension backend serves
  <method>"), so the pane SDK promise rejects. The SDK also rejects any request that gets no answer
  within 30 s (`REQUEST_TIMEOUT_MS`).
- `subscribe` is answered with a `protocol.error` frame; `unsubscribe` is accepted silently.
- `settings.get`, `settings.subscribe`, `composer.*` and `attachment.open` are removed from the
  protocol and the SDK: no shipped pane used them, and the `settings` frame already pushes every
  shell setting. They return with the extension contract that needs them.
- A malformed, oversized or unknown frame is answered with `protocol.error` and the pane keeps
  running; eight bad frames in a row fail the pane. `register` and every other control frame are
  never dropped from the inbox; only `pane.setState`, `pane.setTitle` and hover reports are
  dropped once 256 are waiting, and the pane is told how many.

## Still unwired (kept on purpose, tracked here)

`ExtensionBundles`, `PaneHost::extension_status`, the extension web view kind, the scoped relay
tokens and the `x-crowbar-extension` header are built and unit-tested but have no producer in the
shipped binary.

## To wire when the extension host lands on this branch

1. On startup and on every status change, read the daemon's extension list (id, state, panes with
   entry and lifecycle) through `DaemonTransport`.
2. For each enabled extension: `ExtensionBundles::insert(id, source)` and
   `registry_mut().register(PaneType { kind: Web, ... })` per declared pane, with the extension
   scope from the pane type id (`ext.<id>.<pane>`).
3. On a status change call `PaneHost::extension_status` (it sends `extension.status`, shows the
   error in the pane frame while unavailable and re-sends `mount` when the extension is back).
4. Replace the `request` refusal with a proxy to `POST /ext/:extId/call` and `subscribe` with the
   event bridge, forwarding `extension.event` frames.
5. Add an end-to-end test against a fake `DaemonTransport` that lists one extension.
