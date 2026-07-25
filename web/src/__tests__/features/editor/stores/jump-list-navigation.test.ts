import { beforeEach, describe, expect, it } from 'vitest'
import { useJumpListStore } from '@/features/editor/stores/jump-list-store'

const at = (name: string, line = 1) => ({
  bufferId: `buf-${name}`,
  workspaceId: 'w1',
  filePath: `/src/${name}.ts`,
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

  // Mirrors the recorder in use-navigation-history: leaving a buffer pushes the
  // buffer being left, so after visiting A -> B -> C the stack holds [A, B] and
  // the user is "present" at C.
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

    // Going back from the present records where we are, so forward can return.
    expect(actions().goBack(at('c'))?.filePath).toBe('/src/b.ts')
    expect(actions().goBack()?.filePath).toBe('/src/a.ts')

    // At the oldest entry: back exhausted, forward available.
    expect(actions().canGoBack()).toBe(false)
    expect(actions().canGoForward()).toBe(true)

    expect(actions().goForward()?.filePath).toBe('/src/b.ts')
    expect(actions().goForward()?.filePath).toBe('/src/c.ts')
    expect(actions().canGoForward()).toBe(false)
  })

  it('truncates the forward branch when navigating somewhere new mid-history', () => {
    visitAthenBthenC()
    actions().goBack(at('c'))
    expect(actions().canGoForward()).toBe(true)

    // The user navigates to a new file instead of going forward.
    actions().pushEntry(at('d'))

    expect(actions().canGoForward()).toBe(false)
    expect(useJumpListStore.getState().entries.map((e) => e.filePath)).not.toContain('/src/c.ts')
  })

  describe('consumeNavigationTarget', () => {
    it('marks the buffer that back navigated to, exactly once', () => {
      visitAthenBthenC()
      actions().goBack(at('c'))

      // The recorder observes the activation caused by goBack and must skip it,
      // otherwise history could never be walked twice.
      expect(actions().consumeNavigationTarget('buf-b')).toBe(true)
      // Second read is a no-op: a later genuine visit to the same buffer records.
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
      // The real target is still pending, so it is still suppressed when it lands.
      expect(actions().consumeNavigationTarget('buf-b')).toBe(true)
    })

    it('is inert when no history navigation is in flight', () => {
      expect(actions().consumeNavigationTarget('buf-a')).toBe(false)
    })
  })

  // The handshake names a BUFFER id, but navigating to an entry whose tab was
  // closed has to reopen the file — and that mints a NEW buffer id. The marker
  // then names a buffer that will never be activated, the recorder fails to
  // recognise the activation it does see, and records it as a fresh navigation
  // (truncating the forward branch and resetting currentIndex). The marker has
  // to be able to follow the buffer it is waiting for.
  describe('retargetNavigation', () => {
    it('moves the pending marker to the id the file was reopened under', () => {
      visitAthenBthenC()
      actions().goBack(at('c'))

      actions().retargetNavigation('buf-b-reopened')

      expect(actions().consumeNavigationTarget('buf-b-reopened')).toBe(true)
      // The stale id must no longer be honoured.
      expect(actions().consumeNavigationTarget('buf-b')).toBe(false)
    })

    it('does nothing when no history navigation is in flight', () => {
      actions().retargetNavigation('buf-x')
      expect(actions().consumeNavigationTarget('buf-x')).toBe(false)
    })

    it('leaves history walkable: forward stays enabled and back keeps stepping', () => {
      // A -> B -> C -> D, present at D; C's tab has since been closed.
      actions().pushEntry(at('a'))
      actions().pushEntry(at('b'))
      actions().pushEntry(at('c'))

      const back = actions().goBack(at('d'))
      expect(back?.filePath).toBe('/src/c.ts')

      // Reopened from disk under a brand-new buffer id.
      actions().retargetNavigation('buf-c-reopened')

      // The recorder recognises the activation and records nothing.
      expect(actions().consumeNavigationTarget('buf-c-reopened')).toBe(true)
      expect(actions().canGoForward()).toBe(true)
      // Back again steps to B — not to D, the file we started from.
      expect(actions().goBack(at('c'))?.filePath).toBe('/src/b.ts')
    })

    it('keeps the entry’s own workspace stamp — retargeting moves the marker, not the entry', () => {
      visitAthenBthenC()
      const back = actions().goBack(at('c'))
      actions().retargetNavigation('buf-b-reopened')

      expect(back?.workspaceId).toBe('w1')
      expect(useJumpListStore.getState().entries.every((e) => e.workspaceId === 'w1')).toBe(true)
    })
  })
})
