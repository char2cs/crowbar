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

  constructor(path: string) {
    this.connId = crypto.randomUUID()

    // The Channel carries each raw text frame the Rust reader forwards; surface
    // it as a MessageEvent-like `{ data }` so the manager's onmessage parses it
    // exactly as it would a native frame.
    const channel = new Channel<string>()
    channel.onmessage = (text) => {
      this.onmessage?.({ data: text })
    }

    invoke('ws_open', { connId: this.connId, path, onMessage: channel })
      .then(() => {
        this.readyState = TauriWebSocket.OPEN
        this.onopen?.()
      })
      .catch((err) => {
        this.readyState = TauriWebSocket.CLOSED
        this.onerror?.(err)
        this.onclose?.()
      })
  }

  send(data: string): void {
    void invoke('ws_send', { connId: this.connId, data })
  }

  close(): void {
    this.readyState = TauriWebSocket.CLOSED
    void invoke('ws_close', { connId: this.connId })
    this.onclose?.()
  }
}
