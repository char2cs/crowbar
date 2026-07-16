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
  //
  // "Alive" here is trustworthy, not assumed: terminalHasTransport reflects a transport
  // that is actively removed the moment it dies. Phase 0 closed the gap where a half-open
  // socket stayed in the map with no drop surfaced — the resolver would then reuse a corpse
  // (return reused:true onto a socket that delivers nothing). Now such a socket is retired
  // by Rust and reported dropped, so hasTransport goes false and we take the re-attach path.
  //
  // REUSE-WITHOUT-ATTACH IS ONLY SAFE FOR A SHELL TAB. A shell tab's xterm is kept mounted
  // across pane splits and tab moves (pane-container holds `terminal` buffers behind
  // visibility:hidden), so a surviving transport keeps feeding the SAME xterm that already
  // holds the screen — reusing it in place is right.
  //
  // An attach-only view (an agent chat) is the opposite, and MUST re-attach even when a
  // live transport is present. Crowbar does not portal terminals, so an attach-only xterm
  // is BRAND-NEW on every (re)mount and holds nothing. The daemon serializes its screen
  // model to a client at ATTACH and nowhere else, so reusing a transport without attaching
  // leaves that fresh xterm blank; and if the surviving transport is a corpse — an attach
  // refcount that skipped its detach-on-unmount, a half-open socket — terminalWrite silently
  // drops every keystroke against it: the "agent chat goes dead after a remount" bug. So an
  // attach-only resolve always validates the PTY and calls terminalAttach, which pulls the
  // ground-state redraw AND (re)establishes a live transport. reused:true still reflects that
  // the SESSION was reused (no fresh spawn), only the transport was re-attached.
  if (args.storeConnectionId) {
    const canReuseInPlace = !args.attachOnly && terminalHasTransport(args.storeConnectionId)
    if (!canReuseInPlace) {
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
