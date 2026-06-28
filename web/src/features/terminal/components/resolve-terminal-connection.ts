import { terminalAttach, terminalHasTransport } from '@/lib/crowbar-bridge'
import { loadReconnect, clearReconnect } from '../lib/terminal-reconnect-map'

interface ResolveArgs {
  workspaceId: string
  tabSessionId: string
  storeConnectionId: string | undefined
  base: string
  listLiveSessions: () => Promise<string[]>
  createTerminal: () => Promise<string>
}

// Decide whether to reuse, re-attach to, or freshly create the daemon PTY for a
// terminal tab. Reuse order: in-memory store > persisted+daemon-confirmed > fresh.
export async function resolveTerminalConnection(
  args: ResolveArgs,
): Promise<{ connectionId: string; reused: boolean }> {
  // In-memory store still knows the connectionId, but a workspace switch
  // detached (closed) its transport. Reuse the id, but RE-ATTACH if the WS is
  // gone — otherwise terminalListen/Write/Resize no-op and the pane is dead.
  if (args.storeConnectionId) {
    if (!terminalHasTransport(args.storeConnectionId)) {
      await terminalAttach(args.storeConnectionId, args.base)
    }
    return { connectionId: args.storeConnectionId, reused: true }
  }

  const persisted = loadReconnect(args.workspaceId, args.tabSessionId)
  if (persisted) {
    const live = await args.listLiveSessions().catch(() => [] as string[])
    if (live.includes(persisted)) {
      await terminalAttach(persisted, args.base)
      return { connectionId: persisted, reused: true }
    }
    clearReconnect(args.workspaceId, args.tabSessionId) // stale — daemon no longer has it
  }
  const fresh = await args.createTerminal()
  return { connectionId: fresh, reused: false }
}
