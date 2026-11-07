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
      { empty: false, slashQuery: null },
    )

    expect(event.preventDefault).toHaveBeenCalled()
    expect(t.recall).toHaveBeenCalledWith('run the tests')
  })
})
