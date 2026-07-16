// Reference-count the CLIENT ATTACHMENTS to an attach-only PTY, keyed by its
// daemon connectionId (the shared PTY handle), NOT by tab sessionId.
//
// The hazard this closes: an attach-only terminal releases its transport when
// its xterm unmounts (a chat tab switch, a pane move, a workspace re-entry) so
// the next mount RE-ATTACHES and is redrawn — a detach, done blindly, is
// correct only while exactly one view holds that connection. During a pane
// move or a transient duplicate, two mounted views can briefly share ONE
// connectionId: the outgoing view's unmount would then detach a transport the
// still-live incoming view depends on, blanking it until an unrelated resize
// repaints it. Counting the attachments makes the LAST releaser — and only it —
// perform the detach.
//
// Keyed by connectionId because that is the shared resource a detach tears
// down; two views on the SAME PTY resolve to the SAME connectionId, and the
// count reflects how many live views that transport is feeding.
//
// Framework-free and process-global on purpose: attach/detach is a property of
// the daemon connection, not of any one React tree, so the ledger must outlive
// every component that touches it.

const attachments = new Map<string, number>()

/**
 * Record that one more live view is attached to `connectionId`. Call exactly
 * once per view, right after a successful resolve hands back the connectionId
 * (init or reconnect/swap), and only for attach-only terminals.
 */
export function retain(connectionId: string): void {
  attachments.set(connectionId, (attachments.get(connectionId) ?? 0) + 1)
}

/**
 * Record that one view detached from `connectionId`. Returns TRUE when the
 * caller is the last holder (count reached 0) and should therefore perform the
 * actual `terminalDetach` — and FALSE when other live views still hold it.
 *
 * An UNKNOWN connectionId (never retained, or already fully released) returns
 * TRUE: the safe default is to detach. Nothing here has claimed the transport,
 * so a caller that believes it owns one should be free to release it; this also
 * preserves the pre-refcount behavior for any path that detaches a connection
 * the counter never saw (e.g. a transport-drop replacement it never retained).
 */
export function release(connectionId: string): boolean {
  const count = attachments.get(connectionId)
  if (count === undefined) return true
  if (count <= 1) {
    attachments.delete(connectionId)
    return true
  }
  attachments.set(connectionId, count - 1)
  return false
}

/** Test-only: drop all counts so the process-global ledger can't leak between
 *  tests that share this module. Not part of the runtime contract. */
export function __resetAttachRefcountForTest(): void {
  attachments.clear()
}
