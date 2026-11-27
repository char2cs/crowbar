export type ShellMenuEntry =
  | { kind: 'item'; label: string; enabled: boolean }
  | { kind: 'submenu'; label: string; enabled: boolean; entries: ShellMenuEntry[] }
  | { kind: 'separator' }
