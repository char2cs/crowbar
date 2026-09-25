/**
 * How many times the daemon's state may have changed as this client can know
 * it: every push frame received and every write sent. A read issued before
 * the count moved is as new as anything a later caller could have seen.
 */
let changes = 0

export function noteDaemonChange(): void {
  changes++
}

export function daemonChanges(): number {
  return changes
}
