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
 * `{ gone: true }` means the session this tab was bound to no longer exists on
 * the daemon — it exited (or was evicted) while nothing was attached to hear
 * its exit frame. That is an END, for a shell tab as much as an agent view: a
 * tab never spawns a PTY because an existing one exited (invariant B7). Only a
 * tab that was never bound to a session gets a fresh one.
 */
export type ResolvedTerminal =
  { connectionId: string; reused: boolean } | { gone: true } | { unknown: true }

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

// The session this tab was bound to is not on the daemon: it ended. Drop the
// stale reconnect mapping so no later mount re-attempts the dead id.
function reportGone(args: ResolveArgs): ResolvedTerminal {
  clearReconnect(args.workspaceId, args.tabSessionId)
  return { gone: true }
}

// Decide whether to reuse, re-attach to, or freshly create the daemon PTY for a
// terminal tab. Reuse order: in-memory store > persisted+daemon-confirmed >
// fresh — and a bound session the daemon no longer has is gone, never replaced.
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
      const live = await listLive(args.listLiveSessions)
      // Could not ask. Change NOTHING — do not declare this PTY dead, and do not
      // spawn over it. The next reconnect asks again.
      if (live === null) return { unknown: true }
      if (!live.includes(args.storeConnectionId)) return reportGone(args)
      await terminalAttach(args.storeConnectionId, args.base)
    }
    return { connectionId: args.storeConnectionId, reused: true }
  }

  const persisted = loadReconnect(args.workspaceId, args.tabSessionId)
  if (persisted) {
    const live = await listLive(args.listLiveSessions)
    // Could not ask — keep the mapping and try again later, rather than clearing
    // the one record of a PTY that is probably still running.
    if (live === null) return { unknown: true }
    if (!live.includes(persisted)) return reportGone(args)
    await terminalAttach(persisted, args.base)
    return { connectionId: persisted, reused: true }
  }
  // Never bound to a session: a shell tab gets its first PTY; an agent view has
  // nothing to show.
  if (args.attachOnly) return { gone: true }
  const fresh = await args.createTerminal()
  return { connectionId: fresh, reused: false }
}
