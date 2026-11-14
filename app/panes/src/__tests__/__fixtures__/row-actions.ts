import type { RowActions } from '@/features/agent/transcript/rows/transcript-row'

/** Row actions for a test that presses none of them. */
export const noRowActions: RowActions = {
  retry: () => {},
  edit: () => {},
  openTerminal: () => {},
  newConversation: () => {},
  openChat: () => {},
}
