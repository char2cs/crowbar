// Stub
import type { Command, Keybinding } from '@/features/keymaps/types'

interface KeybindingRowProps {
  command: Command
  keybinding?: Keybinding
}

export function KeybindingRow(_props: KeybindingRowProps) {
  return null
}
export function keybindingTableMinWidth(): string {
  return 'min-w-[640px]'
}
