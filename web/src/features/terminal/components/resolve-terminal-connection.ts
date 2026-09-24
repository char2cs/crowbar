import { loadReconnect, clearReconnect } from '../lib/terminal-reconnect-map'

interface ResolveArgs {
  workspaceId: string
  tabSessionId: string
  /** The daemon session this tab is bound to in memory, if any. */
  storeConnectionId: string | undefined
  listLiveSessions: () => Promise<string[]>
  createTerminal: () => Promise<string>
  /**
   * Attach-only: NEVER spawn a PTY, not even for a tab that has none yet. Used
   * by the agent chat pane, whose terminal is a VIEW ONTO ONE SPECIFIC vendor-CLI
   * process: a spawned shell there would be a bare login shell wearing the
   * agent's frame.
   */
  attachOnly?: boolean
}

/**
 * The resolver's outcome.
 *
 * `{ sessionId, created }` names the daemon session to attach this view to —
 * `created` when it was spawned just now for a tab that had none.
 *
 * `{ gone: true }` means the session this tab was bound to no longer exists on
 * the daemon — it exited (or was evicted) while nothing was attached to hear
 * its exit frame. That is an END, for a shell tab as much as an agent view: a
 * tab never spawns a PTY because an existing one exited (invariant B7).
 *
 * `{ unknown: true }` means the daemon could not be asked. That is not a death:
 * change nothing, and let the next attempt ask again.
 */
export type ResolvedTerminal =
  { sessionId: string; created: boolean } | { gone: true } | { unknown: true }

// List the daemon's live sessions — or answer `null`, meaning WE COULD NOT ASK.
//
// That distinction is the whole point: a request that merely failed (a busy
// daemon, a socket hiccup, a suspended webview) is not "the session is gone",
// and reading it as one would end a tab over a PTY that is alive and working.
// An EMPTY list, on the other hand, is authoritative — the daemon restores its
// persisted sessions before it serves a single request.
async function listLive(list: () => Promise<string[]>): Promise<string[] | null> {
  try {
    return await list()
  } catch {
    return null
  }
}

/**
 * Decide which daemon PTY session a terminal view attaches to: the one it is
 * bound to (in memory, else the persisted reconnect mapping) if the daemon still
 * has it, a fresh one for a tab that was never bound — and never a replacement
 * for a bound session the daemon no longer has.
 */
export async function resolveTerminalSession(args: ResolveArgs): Promise<ResolvedTerminal> {
  const bound = args.storeConnectionId ?? loadReconnect(args.workspaceId, args.tabSessionId)
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
  if (args.attachOnly) return { gone: true }
  return { sessionId: await args.createTerminal(), created: true }
}
