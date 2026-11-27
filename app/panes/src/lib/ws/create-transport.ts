import { isTauri } from '@/lib/crowbar-bridge'
import { splitHostPath } from '@/lib/host-path'

import { wsUrl } from './url'
import { TauriWebSocket } from './tauri-transport'
import { inShell } from './in-shell'
import { ShellWebSocket } from './shell-web-socket'
import type { WsLike } from './ws-like'

const REMOTE_READ_IDLE_TIMEOUT_MS = 60_000

export function createTransport(
  endpoint: string,
  isLocalHost: (hostId: string) => boolean,
): WsLike {
  const { hostId, path } = splitHostPath(endpoint)
  const local = isLocalHost(hostId)
  if (inShell()) return new ShellWebSocket(path, hostId)
  if (isTauri()) {
    if (local) return new TauriWebSocket(path)
    return new TauriWebSocket(path, { hostId, idleTimeoutMs: REMOTE_READ_IDLE_TIMEOUT_MS })
  }
  return new WebSocket(wsUrl(path, hostId)) as WsLike
}
