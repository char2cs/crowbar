import type { StreamCallback } from './stream-callback'

export interface WsManager {
  subscribe(endpoint: string, cb: StreamCallback): VoidFunction
  send(endpoint: string, data: unknown): void
  opened(endpoint: string): Promise<void>
  onDrop(endpoint: string, listener: VoidFunction): VoidFunction
}
