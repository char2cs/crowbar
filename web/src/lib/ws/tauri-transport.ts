// WebSocket-shim over the Tauri unix-socket Channel bridge (D1).
//
// On the desktop app the browser `WebSocket` constructor cannot reach the
// daemon: its only endpoint is the `crowbar://` unix-socket proxy, and every
// scheme but ws/wss is rejected. There, Rust is the WebSocket client — it dials
// the daemon socket, performs the HTTP→WS upgrade for a given `/v0/...` path,
// and bridges frames over a Tauri Channel (see desktop/src-tauri/src/ws_bridge.rs).
//
// `TauriWebSocket` presents the subset of the `WebSocket` interface the
// `wsManager` relies on (onopen/onmessage/onclose/onerror, send, close,
// readyState + the CONNECTING/OPEN/CLOSED constants) so the manager stays
// transport-agnostic. Frames arrive RAW (the whole DTO text) and are surfaced as
// `{ data: text }` to mirror the native `MessageEvent` shape the manager parses.

import { Channel, invoke } from '@tauri-apps/api/core'

// Mirrors ws_bridge::WS_CLOSE_SENTINEL — pushed down the Channel when the daemon
// closes the stream (restart/timeout/error). The NUL prefix cannot collide with
// a real JSON DTO frame, so the shim treats it as a close, not a message.
const WS_CLOSE_SENTINEL = '\u0000crowbar-ws-close'

export class TauriWebSocket {
  static readonly CONNECTING = 0
  static readonly OPEN = 1
  static readonly CLOSED = 3

  onopen: (() => void) | null = null
  onmessage: ((event: { data: string }) => void) | null = null
  onclose: (() => void) | null = null
  onerror: ((event: unknown) => void) | null = null

  readyState: number = TauriWebSocket.CONNECTING

  private readonly connId: string
  // Set by close() so a teardown issued while the socket is still CONNECTING is
  // honoured once ws_open resolves. The connId is minted client-side before
  // ws_open is invoked, but Rust only registers the connection (UnixStream +
  // reader/writer tasks) AFTER the async dial+upgrade completes. A ws_close
  // racing ahead of that registration is a no-op on the Rust side, so without
  // this flag the now-registered connection — daemon FD + 2 tokio tasks — would
  // leak for every StrictMode double-mount or rapid scope switch. We defer the
  // real ws_close to the ws_open .then() instead.
  private closed = false

  constructor(path: string) {
    this.connId = crypto.randomUUID()

    // The Channel carries each raw text frame the Rust reader forwards; surface
    // it as a MessageEvent-like `{ data }` so the manager's onmessage parses it
    // exactly as it would a native frame.
    const channel = new Channel<string>()
    channel.onmessage = (text) => {
      // A daemon-side close (restart/timeout/error) arrives as the sentinel:
      // surface it as a close so wsManager reconnects and the §6 cache re-seeds.
      if (text === WS_CLOSE_SENTINEL) {
        if (this.readyState === TauriWebSocket.CLOSED) return
        this.readyState = TauriWebSocket.CLOSED
        this.onclose?.()
        return
      }
      this.onmessage?.({ data: text })
    }

    invoke('ws_open', { connId: this.connId, path, onMessage: channel })
      .then(() => {
        // Closed while CONNECTING: the Rust connection is only now registered,
        // so issue the deferred ws_close to tear it down. Per the WebSocket
        // contract a close before open yields no onopen — just a clean teardown.
        if (this.closed) {
          void invoke('ws_close', { connId: this.connId })
          return
        }
        this.readyState = TauriWebSocket.OPEN
        this.onopen?.()
      })
      .catch((err) => {
        this.readyState = TauriWebSocket.CLOSED
        if (this.closed) return
        this.onerror?.(err)
        this.onclose?.()
      })
  }

  send(data: string): void {
    void invoke('ws_send', { connId: this.connId, data })
  }

  close(): void {
    const wasConnecting = this.readyState === TauriWebSocket.CONNECTING
    this.readyState = TauriWebSocket.CLOSED
    this.closed = true
    // While CONNECTING the Rust connection isn't registered yet, so a ws_close
    // now would be a no-op; the ws_open .then() handler issues it once the
    // connection exists. Only call ws_close eagerly when there is something to
    // close (open, or a prior reconnect race).
    if (!wasConnecting) {
      void invoke('ws_close', { connId: this.connId })
    }
    this.onclose?.()
  }
}
