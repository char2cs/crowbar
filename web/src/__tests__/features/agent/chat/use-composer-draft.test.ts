// A chat's stashes have one owner, its stored list: a failed prompt the outbox
// stashes while the composer is open must survive the composer's own next write.
import { act, renderHook } from '@testing-library/react'
import { beforeEach, describe, expect, it } from 'vitest'
import { useComposerDraft } from '@/features/agent/chat/use-composer-draft'
import {
  loadPromptStashes,
  stashPrompt,
} from '@/features/agent/composer/lib/prompt-stash-persistence'

const stored = () => loadPromptStashes('w1', 'c1').map((stash) => stash.markdown)

function renderComposer() {
  return renderHook(() =>
    useComposerDraft({
      wsId: 'w1',
      chatId: 'c1',
      providerId: 'claude',
      catalogActive: false,
      history: { reset: () => {} },
    }),
  )
}

beforeEach(() => localStorage.clear())

describe('useComposerDraft — stashes written while the composer is open', () => {
  it('shows a stash added meanwhile and keeps it when stashing its own draft', () => {
    const { result } = renderComposer()

    act(() => {
      stashPrompt('w1', 'c1', 'the failed prompt')
    })
    expect(result.current.stashes.map((stash) => stash.markdown)).toEqual(['the failed prompt'])

    act(() => result.current.stashOrRestore('my draft'))

    expect(stored()).toEqual(['my draft', 'the failed prompt'])
  })

  it('keeps a stash added meanwhile when restoring another', () => {
    stashPrompt('w1', 'c1', 'older')
    const { result } = renderComposer()
    const older = result.current.stashes[0]

    act(() => {
      stashPrompt('w1', 'c1', 'the failed prompt')
    })
    act(() => result.current.restoreStash(older))

    expect(stored()).toEqual(['the failed prompt'])
  })
})
