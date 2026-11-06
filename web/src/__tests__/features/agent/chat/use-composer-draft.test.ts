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

describe('useComposerDraft — the unsent draft outlives the page', () => {
  /** An editor holding `text`, recording what the composer restores into it. */
  function fakeEditor(text = '') {
    const editor = {
      text,
      restored: [] as string[],
      getMarkdown: () => editor.text,
      restoreMarkdown: (markdown: string) => {
        editor.restored.push(markdown)
        editor.text = markdown
      },
      setMarkdown: (markdown: string) => {
        editor.text = markdown
      },
      clear: () => {
        editor.text = ''
      },
    }
    return editor
  }

  function mount(editor: ReturnType<typeof fakeEditor>, wsId = 'w1') {
    return renderHook(() => {
      const draft = useComposerDraft({
        wsId,
        chatId: 'c1',
        providerId: 'claude',
        catalogActive: false,
        history: { reset: () => {} },
      })
      // Attached during render, as the editor's ref is before any effect runs.
      ;(draft.editorRef as { current: unknown }).current ??= editor
      return draft
    })
  }

  it('keeps the draft through a reload and puts it back, in another workspace too', () => {
    const typing = fakeEditor()
    const first = mount(typing)
    typing.text = 'half a thought'
    act(() => void window.dispatchEvent(new Event('pagehide')))
    first.unmount()

    const fresh = fakeEditor()
    mount(fresh, 'w2')

    expect(fresh.restored).toEqual(['half a thought'])
  })

  it('forgets a draft once the composer is cleared by a send', () => {
    const typing = fakeEditor()
    const first = mount(typing)
    typing.text = 'about to send'
    act(() => void window.dispatchEvent(new Event('pagehide')))
    act(() => first.result.current.clear())
    first.unmount()

    const fresh = fakeEditor()
    mount(fresh)

    expect(fresh.restored).toEqual([])
  })
})
