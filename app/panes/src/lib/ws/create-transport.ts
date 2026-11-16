import { isTauri } from '@/lib/crowbar-bridge'
import { splitHostPath } from '@/lib/host-path'

import { wsUrl } from './url'
import { TauriWebSocket } from './tauri-transport'
import { inShell } from './in-shell'
import { openShellSocket } from './shell-socket'
import type { WsLike } from './ws-like'

// PLATFORM: the daemon pings each 30 s; a remote stream silent for two pings is half-open (network change, sleep).
const REMOTE_READ_IDLE_TIMEOUT_MS = 60_000

/** The live transport for the active environment: the shell's relay socket in the GPUI shell, the
 *  native WebSocket in the browser, the unix-socket-bridged shim on desktop (where `crowbar://` has
 *  no native WebSocket). A host-qualified endpoint dials that host; the local daemon is a unix
 *  socket away, so only a remote stream gets the idle timeout. */
export function createTransport(endpoint: string, isLocalHost: (hostId: string) => boolean): WsLike {
  const { hostId, path } = splitHostPath(endpoint)
  const local = isLocalHost(hostId)
  if (inShell()) {
    if (!local) throw new Error(`host "${hostId}" has no relay for this pane`)
    return openShellSocket(path) as WsLike
  }
  if (isTauri()) {
    if (local) return new TauriWebSocket(path)
    return new TauriWebSocket(path, { hostId, idleTimeoutMs: REMOTE_READ_IDLE_TIMEOUT_MS })
  }
  return new WebSocket(wsUrl(path, hostId)) as WsLike
}
