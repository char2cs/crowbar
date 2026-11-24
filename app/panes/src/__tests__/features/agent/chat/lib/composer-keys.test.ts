import type { KeyboardEvent } from 'react'
import { describe, expect, it, vi } from 'vitest'
import { handleComposerKey } from '@/features/agent/chat/lib/composer-keys'

function targets(searchOlder: (query: string) => string | undefined) {
  return {
    slash: { open: false, items: [], close: vi.fn(), move: vi.fn() },
    history: { recallOlder: vi.fn(), recallNewer: vi.fn(), searchOlder },
    canStop: false,
    stop: vi.fn(),
    retract: vi.fn(() => false),
    stashOrRestore: vi.fn(),
    selectSlashItem: vi.fn(),
    send: vi.fn(),
    enterSends: true,
    recall: vi.fn(),
  }
}

function key(init: Partial<KeyboardEvent<HTMLDivElement>>) {
  return {
    key: '',
    shiftKey: false,
    metaKey: false,
    ctrlKey: false,
    altKey: false,
    preventDefault: vi.fn(),
    stopPropagation: vi.fn(),
    ...init,
  } as unknown as KeyboardEvent<HTMLDivElement>
}

describe('handleComposerKey', () => {
  it('ctrl+r recalls the newest earlier prompt with the words typed, not a page reload', () => {
    const t = targets((query) => (query === 'tests' ? 'run the tests' : undefined))
    const event = key({ key: 'r', ctrlKey: true })

    handleComposerKey(
      t,
      event,
      () => 'tests',
      { atStart: false, atEnd: true },
      { empty: false, slashQuery: null, mentionQuery: null },
    )

    expect(event.preventDefault).toHaveBeenCalled()
    expect(t.recall).toHaveBeenCalledWith('run the tests')
  })

  describe('Enter in the blank chat document (enterSends off)', () => {
    const enter = (init: Partial<KeyboardEvent<HTMLDivElement>> = {}) =>
      key({ key: 'Enter', nativeEvent: { isComposing: false }, ...init } as never)
    const press = (t: ReturnType<typeof targets>, event: KeyboardEvent<HTMLDivElement>) =>
      handleComposerKey(
        t,
        event,
        () => 'draft',
        { atStart: false, atEnd: true },
        { empty: false, slashQuery: 'co', mentionQuery: null },
      )

    it('never sends on plain Enter, leaving it as a new line', () => {
      const t = { ...targets(() => undefined), enterSends: false }
      const event = enter()
      press(t, event)
      expect(t.send).not.toHaveBeenCalled()
      expect(event.preventDefault).not.toHaveBeenCalled()
    })

    it('still accepts the highlighted picker item', () => {
      const item = { id: 'compact', insertText: '/compact ' } as never
      const t = { ...targets(() => undefined), enterSends: false }
      t.slash = { ...t.slash, highlighted: item } as never
      press(t, enter())
      expect(t.selectSlashItem).toHaveBeenCalledWith(item)
    })

    it('sends on Cmd+Enter', () => {
      const t = { ...targets(() => undefined), enterSends: false }
      press(t, enter({ metaKey: true }))
      expect(t.send).toHaveBeenCalledWith('draft')
    })
  })

  describe('Enter on a command typed in full', () => {
    const enter = () => key({ key: 'Enter', nativeEvent: { isComposing: false } } as never)
    const withPicker = (draft: string) => {
      const t = targets(() => undefined)
      t.slash = { ...t.slash, highlighted: { insertText: '/compact ' } } as never
      handleComposerKey(
        t,
        enter(),
        () => draft,
        { atStart: false, atEnd: true },
        { empty: false, slashQuery: 'compact', mentionQuery: null },
      )
      return t
    }

    it('sends it, as the picker’s run hint says, instead of completing it again', () => {
      const t = withPicker('/compact')
      expect(t.send).toHaveBeenCalledWith('/compact')
      expect(t.selectSlashItem).not.toHaveBeenCalled()
    })

    it('still completes a command typed in part', () => {
      const t = withPicker('/comp')
      expect(t.selectSlashItem).toHaveBeenCalled()
      expect(t.send).not.toHaveBeenCalled()
    })
  })
})
