import { beforeEach, describe, expect, it } from 'vitest'
import { useJumpListStore } from '@/features/editor/stores/jump-list-store'
import { workspaceRef } from '@/lib/api/documents'
import { sameDocument } from '@/features/editor/documents/document-ref'

const at = (name: string, line = 1, workspaceId = 'w1') => ({
  bufferId: `buf-${name}`,
  doc: workspaceRef(workspaceId, `/src/${name}.ts`),
  line,
  column: 1,
  offset: 0,
  scrollTop: 0,
  scrollLeft: 0,
})

const actions = () => useJumpListStore.getState().actions

describe('jump list — back/forward over file navigation history', () => {
  beforeEach(() => {
    actions().clear()
  })

  const visitAthenBthenC = () => {
    actions().pushEntry(at('a'))
    actions().pushEntry(at('b'))
  }

  it('cannot go back or forward with no history', () => {
    expect(actions().canGoBack()).toBe(false)
    expect(actions().canGoForward()).toBe(false)
  })

  it('enables back once a single file switch is recorded', () => {
    actions().pushEntry(at('a'))
    expect(actions().canGoBack()).toBe(true)
    expect(actions().canGoForward()).toBe(false)
  })

  it('walks back through every visited file and forward again', () => {
    visitAthenBthenC()

    expect(actions().goBack(at('c'))?.doc.path).toBe('/src/b.ts')
    expect(actions().goBack()?.doc.path).toBe('/src/a.ts')

    expect(actions().canGoBack()).toBe(false)
    expect(actions().canGoForward()).toBe(true)

    expect(actions().goForward()?.doc.path).toBe('/src/b.ts')
    expect(actions().goForward()?.doc.path).toBe('/src/c.ts')
    expect(actions().canGoForward()).toBe(false)
  })

  it('keeps a nearby line of another workspace’s same-path file as its own entry', () => {
    actions().pushEntry(at('a', 10))
    actions().pushEntry({ ...at('a', 12, 'w2'), bufferId: 'buf-a-w2' })

    expect(actions().goBack(at('c'))?.doc).toEqual(workspaceRef('w2', '/src/a.ts'))
    expect(actions().goBack()?.doc).toEqual(workspaceRef('w1', '/src/a.ts'))
  })

  it('truncates the forward branch when navigating somewhere new mid-history', () => {
    visitAthenBthenC()
    actions().goBack(at('c'))
    expect(actions().canGoForward()).toBe(true)

    actions().pushEntry(at('d'))

    expect(actions().canGoForward()).toBe(false)
    expect(useJumpListStore.getState().entries.map((e) => e.doc.path)).not.toContain('/src/c.ts')
  })

  describe('consumeNavigationTarget', () => {
    it('marks the buffer that back navigated to, exactly once', () => {
      visitAthenBthenC()
      actions().goBack(at('c'))

      expect(actions().consumeNavigationTarget('buf-b')).toBe(true)
      expect(actions().consumeNavigationTarget('buf-b')).toBe(false)
    })

    it('marks the buffer that forward navigated to', () => {
      visitAthenBthenC()
      actions().goBack(at('c'))
      actions().consumeNavigationTarget('buf-b')
      actions().goForward()

      expect(actions().consumeNavigationTarget('buf-c')).toBe(true)
    })

    it('does not suppress an unrelated buffer activation', () => {
      visitAthenBthenC()
      actions().goBack(at('c'))

      expect(actions().consumeNavigationTarget('buf-unrelated')).toBe(false)
      expect(actions().consumeNavigationTarget('buf-b')).toBe(true)
    })

    it('is inert when no history navigation is in flight', () => {
      expect(actions().consumeNavigationTarget('buf-a')).toBe(false)
    })
  })

  describe('retargetNavigation', () => {
    it('moves the pending marker to the id the file was reopened under', () => {
      visitAthenBthenC()
      actions().goBack(at('c'))

      actions().retargetNavigation('buf-b-reopened')

      expect(actions().consumeNavigationTarget('buf-b-reopened')).toBe(true)
      expect(actions().consumeNavigationTarget('buf-b')).toBe(false)
    })

    it('does nothing when no history navigation is in flight', () => {
      actions().retargetNavigation('buf-x')
      expect(actions().consumeNavigationTarget('buf-x')).toBe(false)
    })

    it('leaves history walkable: forward stays enabled and back keeps stepping', () => {
      actions().pushEntry(at('a'))
      actions().pushEntry(at('b'))
      actions().pushEntry(at('c'))

      const back = actions().goBack(at('d'))
      expect(back?.doc.path).toBe('/src/c.ts')

      actions().retargetNavigation('buf-c-reopened')

      expect(actions().consumeNavigationTarget('buf-c-reopened')).toBe(true)
      expect(actions().canGoForward()).toBe(true)
      expect(actions().goBack(at('c'))?.doc.path).toBe('/src/b.ts')
    })

    it('keeps the entry’s own workspace stamp — retargeting moves the marker, not the entry', () => {
      visitAthenBthenC()
      const back = actions().goBack(at('c'))
      actions().retargetNavigation('buf-b-reopened')

      expect(back?.doc).toEqual(workspaceRef('w1', '/src/b.ts'))
      const docs = useJumpListStore.getState().entries.map((e) => e.doc)
      expect(docs.every((doc) => sameDocument(doc, workspaceRef('w1', doc.path)))).toBe(true)
    })
  })
})
