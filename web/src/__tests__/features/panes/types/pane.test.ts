import { describe, expect, it } from 'vitest'
import type { PaneGroup } from '@/features/panes/types/pane'

describe('PaneGroup', () => {
  it('holds exactly one chat, never a list', () => {
    const pane: PaneGroup = {
      id: 'pane-1',
      type: 'group',
      chatId: 'chat-1',
      runnerId: 'runner-1',
      editorTabIds: [],
      activeEditorTabId: null,
      editorOpen: false,
    }
    expect(pane.chatId).toBe('chat-1')
    expect(pane).not.toHaveProperty('bufferIds')
  })

  it('an empty pane has chatId null, not a newTab buffer', () => {
    const pane: PaneGroup = {
      id: 'pane-1',
      type: 'group',
      chatId: null,
      runnerId: null,
      editorTabIds: [],
      activeEditorTabId: null,
      editorOpen: false,
    }
    expect(pane.chatId).toBeNull()
  })
})
