import type { StreamCallback } from './stream-callback'

/** The socket manager: one shared channel per endpoint, reconnecting under its subscribers. */
export interface WsManager {
  subscribe(endpoint: string, cb: StreamCallback): VoidFunction
  send(endpoint: string, data: unknown): void
  /** Resolves once `endpoint`'s socket is open. Rejects with the transport's failure when the one
   *  attempt it waits on (in flight, or the reconnect after backoff, 30s at most) closes first; a
   *  caller wanting longer awaits again. */
  opened(endpoint: string): Promise<void>
  /** `listener` hears each drop of `endpoint`'s open socket while it has subscribers. */
  onDrop(endpoint: string, listener: VoidFunction): VoidFunction
}
