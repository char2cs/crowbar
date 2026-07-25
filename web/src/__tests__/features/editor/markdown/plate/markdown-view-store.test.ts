import { beforeEach, describe, expect, it } from 'vitest'
import {
  useMarkdownViewStore,
  selectMarkdownView,
} from '@/features/editor/markdown/plate/markdown-view-store'

describe('markdown view store', () => {
  beforeEach(() => useMarkdownViewStore.setState({ views: {} }))

  it('defaults to rich for an unknown buffer', () => {
    expect(selectMarkdownView('b1')(useMarkdownViewStore.getState())).toBe('rich')
  })

  it('setView records the mode', () => {
    useMarkdownViewStore.getState().setView('b1', 'source')
    expect(selectMarkdownView('b1')(useMarkdownViewStore.getState())).toBe('source')
  })

  it('toggleView flips rich<->source from the default', () => {
    useMarkdownViewStore.getState().toggleView('b1') // rich -> source
    expect(selectMarkdownView('b1')(useMarkdownViewStore.getState())).toBe('source')
    useMarkdownViewStore.getState().toggleView('b1') // source -> rich
    expect(selectMarkdownView('b1')(useMarkdownViewStore.getState())).toBe('rich')
  })

  it('clearView removes the entry', () => {
    useMarkdownViewStore.getState().setView('b1', 'source')
    useMarkdownViewStore.getState().clearView('b1')
    expect(useMarkdownViewStore.getState().views.b1).toBeUndefined()
  })
})
