/** Fire-and-forget a promise whose failure is not worth interrupting anyone over.
 *
 *  The callers are all lazy-loaded CLEANUP or background fills: freeing a closed
 *  buffer's blame data, stopping a chat whose tab just went away, prefetching a
 *  pane chunk. The user-visible work is already done by the time these run, and a
 *  module that will not load has no state left to clean up — so there is nothing
 *  useful to do with the error beyond saying so.
 *
 *  What is NOT acceptable is leaving the rejection floating, which is what a bare
 *  `void import('…').then(…)` does. Two ways that bites:
 *
 *  - In the app, an unhandled rejection reaches window.onunhandledrejection and is
 *    reported as if the app had crashed, for a cleanup nobody was waiting on.
 *  - Under test it surfaces as `EnvironmentTeardownError: Cannot load '…' after the
 *    environment was torn down`, failing an otherwise green run. Deliberately, these
 *    imports are never awaited, so closeBuffer routinely returns — and the test with
 *    it — while the module graph behind it is still resolving. Whether the load wins
 *    that race depends on machine load, which is the definition of a flaky gate.
 *
 *  Deliberately swallows rather than rethrows: rethrowing inside the catch would
 *  simply produce a second floating rejection. The DEV-only warning keeps a genuine
 *  bug in one of these callbacks visible while developing without adding noise to a
 *  production console.
 */
export function bestEffort(work: Promise<unknown>, what: string): void {
  void work.catch((err: unknown) => {
    if (import.meta.env.DEV) console.warn(`best-effort ${what} failed:`, err)
  })
}
