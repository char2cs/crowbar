/** One entry of a menu the shell pops up natively. */
export type ShellMenuEntry =
  | { kind: 'item'; label: string; enabled: boolean }
  | { kind: 'submenu'; label: string; enabled: boolean; entries: ShellMenuEntry[] }
  | { kind: 'separator' }
