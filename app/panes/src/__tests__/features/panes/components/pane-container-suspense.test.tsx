import { createElement } from 'react'
import { useStore } from 'zustand'
import { act, render, screen } from '@testing-library/react'
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { WorkspaceStoreContext } from '@/features/workspace/stores/workspace-context'
import { createWorkspaceStore } from '@/features/workspace/stores/workspace-store'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { resetWindowPaneStoreForTests } from '@/__tests__/__fixtures__/window-pane-store'
import { ROOT_PANE_ID } from '@/features/panes/constants/pane'

const { reviewResource } = vi.hoisted(() => {
  function createDeferredResource<T>() {
    let status: 'pending' | 'resolved' = 'pending'
    let result: T
    let resolveFn!: (v: T) => void
    const promise = new Promise<T>((res) => {
      resolveFn = res
    })
    promise.then((v) => {
      status = 'resolved'
      result = v
    })
    return {
      read(): T {
        if (status === 'pending') throw promise
        return result
      },
      resolve: (v: T) => resolveFn(v),
    }
  }
  return { reviewResource: createDeferredResource<true>() }
})

vi.mock('@/features/git/components/branch-review-pane', () => ({
  BranchReviewPane: () => {
    reviewResource.read()
    return createElement('div', { 'data-testid': 'review-pane-content' }, 'Review Loaded')
  },
}))

vi.mock('@/features/panes/components/terminal-pane', () => ({
  TerminalPane: ({ sessionId }: { sessionId?: string }) =>
    createElement('div', { 'data-testid': 'terminal-marker', 'data-session-id': sessionId ?? '' }),
}))

vi.mock('@/features/tabs/components/tab-bar', () => ({
  default: () => null,
}))

import { PaneContainer } from '@/features/panes/components/pane-container'

function PaneHost() {
  const panes = useStore(windowPaneStore, (s) => s.panes)
  const pane = panes[ROOT_PANE_ID]
  if (!pane) return null
  return createElement(PaneContainer, { pane })
}

function seedStore() {
  const store = createWorkspaceStore('w1')
  const { openContent } = windowPaneStore.getState().bufferActions
  const terminalId = openContent({
    type: 'terminal',
    sessionId: 'pty-1',
    name: 'Terminal 1',
    workspaceId: 'w1',
  })
  const reviewId = openContent({ type: 'branchReview', wsId: 'w1', name: 'Branch Review' })
  return { store, terminalId, reviewId }
}

describe('PaneContainer Suspense boundary', () => {
  beforeEach(() => {
    resetWindowPaneStoreForTests()
  })

  it('keeps the always-mounted terminal pane mounted while a sibling lazy pane suspends, and after it resolves', async () => {
    const { store } = seedStore()

    await act(async () => {
      render(
        createElement(WorkspaceStoreContext.Provider, { value: store }, createElement(PaneHost)),
      )
    })

    const terminalBefore = screen.getByTestId('terminal-marker')
    expect(terminalBefore).toHaveAttribute('data-session-id', 'pty-1')
    expect(screen.queryByTestId('review-pane-content')).not.toBeInTheDocument()

    await act(async () => {
      reviewResource.resolve(true)
    })

    expect(await screen.findByTestId('review-pane-content')).toHaveTextContent('Review Loaded')
    expect(screen.getByTestId('terminal-marker')).toBe(terminalBefore)
  })
})
