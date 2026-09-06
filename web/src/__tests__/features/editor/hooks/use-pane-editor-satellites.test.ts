import { describe, it, expect, beforeEach, vi } from 'vitest'
import { act, renderHook } from '@testing-library/react'

// Same race as the already-fixed use-workspace-effects.ts: a workspace's
// owning chat id is recorded ASYNCHRONOUSLY by the sidebar's own chat-list
// fetch, independently of (and often slower than) the workspace's own
// hydration. The LSP diagnostics effect in use-pane-editor-satellites.ts used
// to call straight into LspClient's `ensureSubscribed`/`wsBase`, which throw
// on a null owning chat id — a buffer becoming a pane's active model (tab
// restoration on a cold activation) crashed via the nearest error boundary.
// `useLspScopeReady` is the gate that gives that effect a piece of REACT
// STATE to wait on instead.

const { wsIdHolder } = vi.hoisted(() => ({ wsIdHolder: { current: 'ws-race' as string | null } }))

vi.mock('@/features/workspace/stores/workspace-store-registry', () => ({
  getActiveWorkspaceId: () => wsIdHolder.current,
}))

import { useLspScopeReady } from '@/features/editor/hooks/use-pane-editor-satellites'
import {
  setWorkspaceScope,
  recordWorkspaceScope,
  __resetWorkspaceScopesForTest,
} from '@/lib/workspace-scope'

describe('useLspScopeReady', () => {
  beforeEach(() => {
    __resetWorkspaceScopesForTest()
    wsIdHolder.current = 'ws-race'
  })

  it('is false while a non-home workspace has no owning chat id recorded', () => {
    setWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'ws-race' })
    const { result } = renderHook(() => useLspScopeReady())
    expect(result.current).toBe(false)
  })

  it('flips to true once the owning chat id arrives, without a remount', () => {
    setWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'ws-race' })
    const { result } = renderHook(() => useLspScopeReady())
    expect(result.current).toBe(false)

    act(() => {
      recordWorkspaceScope({
        projectId: 'p1',
        repoId: 'r1',
        wsId: 'ws-race',
        owningChatId: 'chat-race',
      })
    })

    expect(result.current).toBe(true)
  })

  it('is true immediately when the scope was already recorded before mount (no race)', () => {
    setWorkspaceScope({
      projectId: 'p1',
      repoId: 'r1',
      wsId: 'ws-race',
      owningChatId: 'chat-race',
    })
    const { result } = renderHook(() => useLspScopeReady())
    expect(result.current).toBe(true)
  })

  // The home (project-level) workspace has no worktree and therefore no chat
  // and no LSP surface at all — it must never wait on an id that will never
  // arrive.
  it('is true immediately for the home workspace', () => {
    setWorkspaceScope({ projectId: 'p1', repoId: '', wsId: 'home-ws' })
    wsIdHolder.current = 'home-ws'
    const { result } = renderHook(() => useLspScopeReady())
    expect(result.current).toBe(true)
  })

  it('is true when there is no active workspace at all', () => {
    wsIdHolder.current = null
    const { result } = renderHook(() => useLspScopeReady())
    expect(result.current).toBe(true)
  })

  // A wait for a workspace the user has since navigated away from must not
  // resolve stale — re-rendering with the NEW active id re-subscribes to that
  // id's own scope instead of the abandoned one.
  it('does not resolve for a workspace the user has since navigated away from', () => {
    setWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'ws-race' })
    const { result, rerender } = renderHook(() => useLspScopeReady())
    expect(result.current).toBe(false)

    wsIdHolder.current = 'ws-other'
    setWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'ws-other' })
    rerender()
    expect(result.current).toBe(false)

    act(() => {
      recordWorkspaceScope({
        projectId: 'p1',
        repoId: 'r1',
        wsId: 'ws-race',
        owningChatId: 'chat-race',
      })
    })
    expect(result.current).toBe(false)

    act(() => {
      recordWorkspaceScope({
        projectId: 'p1',
        repoId: 'r1',
        wsId: 'ws-other',
        owningChatId: 'chat-other',
      })
    })
    expect(result.current).toBe(true)
  })
})
