import { terminalAttach, terminalHasTransport } from '@/lib/crowbar-bridge'
import { loadReconnect, clearReconnect } from '../lib/terminal-reconnect-map'

interface ResolveArgs {
  workspaceId: string
  tabSessionId: string
  storeConnectionId: string | undefined
  base: string
  listLiveSessions: () => Promise<string[]>
  createTerminal: () => Promise<string>
  /**
   * Attach-only: NEVER spawn a PTY. When the session this tab is bound to is not
   * live on the daemon, resolve to `{ gone: true }` instead of calling
   * createTerminal.
   *
   * This exists for the agent chat pane, whose terminal is not a shell tab but a
   * VIEW ONTO ONE SPECIFIC vendor-CLI process. Spawning is not a graceful
   * fallback there — it is a corruption: the pane silently becomes a bare login
   * shell wearing the agent's frame, and that shell gets persisted into the
   * reconnect map as if it were the agent. A shell tab, by contrast, is
   * fungible: "your PTY is gone, here is a new one" is exactly right, so the
   * default (unset) stays spawn-on-miss and ordinary terminals are untouched.
   */
  attachOnly?: boolean
}

/**
 * The resolver's outcome.
 *
 * `{ gone: true }` is reachable ONLY under attachOnly: the tab's session is not
 * live on the daemon and spawning a replacement is forbidden. The caller must
 * render its own "this session has ended" state — there is nothing to attach to.
 */
export type ResolvedTerminal = { connectionId: string; reused: boolean } | { gone: true }

// List the daemon's live sessions, tolerating a transiently-empty result right
// after a daemon restart (socket-rebind window / startup restore): an empty list
// is retried once after a short delay before being trusted. Used by BOTH reuse
// branches so neither falls to a spurious fresh-create during the restart window.
async function listLiveWithRetry(list: () => Promise<string[]>): Promise<string[]> {
  let live = await list().catch(() => [] as string[])
  if (live.length === 0) {
    await new Promise<void>((resolve) => setTimeout(resolve, 400))
    live = await list().catch(() => [] as string[])
  }
  return live
}

// The single "the session we wanted is not on the daemon" exit. Under attachOnly
// it reports the session gone (and drops the now-stale reconnect mapping so no
// later mount re-attempts the dead id); otherwise it does what a shell tab wants
// and spawns a fresh PTY.
async function spawnOrReportGone(args: ResolveArgs): Promise<ResolvedTerminal> {
  if (args.attachOnly) {
    clearReconnect(args.workspaceId, args.tabSessionId)
    return { gone: true }
  }
  const fresh = await args.createTerminal()
  return { connectionId: fresh, reused: false }
}

// Decide whether to reuse, re-attach to, or freshly create the daemon PTY for a
// terminal tab. Reuse order: in-memory store > persisted+daemon-confirmed > fresh
// (or, under attachOnly, > gone).
export async function resolveTerminalConnection(args: ResolveArgs): Promise<ResolvedTerminal> {
  // In-memory store still knows the connectionId. If the WS transport is alive,
  // reuse immediately. If not, validate against the daemon before re-attaching:
  // the PTY may be gone (Phase 2 suspend / daemon restart), in which case fall
  // through rather than attaching to a dead PTY.
  if (args.storeConnectionId) {
    if (!terminalHasTransport(args.storeConnectionId)) {
      const live = await listLiveWithRetry(args.listLiveSessions)
      if (!live.includes(args.storeConnectionId)) {
        // PTY no longer exists on the daemon.
        return spawnOrReportGone(args)
      }
      await terminalAttach(args.storeConnectionId, args.base)
    }
    return { connectionId: args.storeConnectionId, reused: true }
  }

  const persisted = loadReconnect(args.workspaceId, args.tabSessionId)
  if (persisted) {
    const live = await listLiveWithRetry(args.listLiveSessions)
    if (live.includes(persisted)) {
      await terminalAttach(persisted, args.base)
      return { connectionId: persisted, reused: true }
    }
    clearReconnect(args.workspaceId, args.tabSessionId) // stale — daemon no longer has it
  }
  return spawnOrReportGone(args)
}
