let changes = 0

export function noteDaemonChange(): void {
  changes++
}

export function daemonChanges(): number {
  return changes
}
