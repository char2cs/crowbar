import { loadReconnect, clearReconnect } from '../lib/terminal-reconnect-map'

interface ResolveArgs {
  workspaceId: string
  tabSessionId: string
  /** The daemon session this tab is bound to in memory, if any. */
  storeConnectionId: string | undefined
  listLiveSessions: () => Promise<string[]>
  createTerminal: () => Promise<string>
  /** The view's own id is the PTY it shows and nothing is spawned: an agent chat pane's terminal
   *  views one CLI process, and a spawned shell there would wear the agent's frame. */
  attachOnly?: boolean
}

// `created`: spawned just now for an unbound tab. `gone`: the bound session exited unheard, an end
// for any view, since no tab replaces an exited PTY. `unknown`: the daemon couldn't be asked, so
// change nothing and let the next attempt ask.
export type ResolvedTerminal =
  { sessionId: string; created: boolean } | { gone: true } | { unknown: true }

// `null` means we could not ask, which must not read as "gone" and end a tab over a live PTY. An
// empty list is authoritative: the daemon restores persisted sessions before serving requests.
async function listLive(list: () => Promise<string[]>): Promise<string[] | null> {
  try {
    return await list()
  } catch {
    return null
  }
}

/** The bound session (attach-only id, else in-memory binding, else persisted reconnect) if the
 *  daemon still has it, a fresh one for a never-bound tab, never a replacement for a lost one. */
export async function resolveTerminalSession(args: ResolveArgs): Promise<ResolvedTerminal> {
  const bound = args.attachOnly
    ? args.tabSessionId
    : (args.storeConnectionId ?? loadReconnect(args.workspaceId, args.tabSessionId))
  if (bound) {
    const live = await listLive(args.listLiveSessions)
    if (live === null) return { unknown: true }
    if (!live.includes(bound)) {
      // It ended. Drop the stale mapping so no later mount re-attempts the dead id.
      clearReconnect(args.workspaceId, args.tabSessionId)
      return { gone: true }
    }
    return { sessionId: bound, created: false }
  }
  return { sessionId: await args.createTerminal(), created: true }
}
