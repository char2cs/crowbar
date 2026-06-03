import type { SlashCommand } from '../types'

const SLASH_COMMANDS: SlashCommand[] = [
  { id: '/tdd', label: '/tdd' },
  { id: '/code-review', label: '/code-review' },
  { id: '/plan', label: '/plan' },
  { id: '/debug', label: '/debug' },
  { id: '/explain', label: '/explain' },
]

export function getSlashCommands(): SlashCommand[] {
  return SLASH_COMMANDS
}
