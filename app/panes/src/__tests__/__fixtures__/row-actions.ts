import type { RowActions } from '@/features/agent/transcript/rows/transcript-row'

export const noRowActions: RowActions = {
  retry: () => {},
  edit: () => {},
  openTerminal: () => {},
  newConversation: () => {},
  openChat: () => {},
}
