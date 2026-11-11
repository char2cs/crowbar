import { describe, expect, it, vi } from 'vitest'
import { render } from '@testing-library/react'
import { createWorkspaceStore } from '@/features/workspace/stores/workspace-store'
import { WorkspaceStoreContext } from '@/features/workspace/stores/workspace-context'
import { ReviewDiffTab } from '@/features/git/components/review-diff-tab'

// ReviewDiffTab must read its wsId prop, never the ambient context (each retained WorkspaceView
// renders the pane tree under its own). The data hooks are asserted directly: they run before
// the files-loaded gate.
const filesSummaryCalls: unknown[] = []
const outlineCalls: unknown[] = []
vi.mock('@/features/git/hooks/use-review-files-summary', () => ({
  useReviewFilesSummary: (wsId: string | null, commit?: string) => {
    filesSummaryCalls.push([wsId, commit])
    return { files: [], loaded: true }
  },
}))
vi.mock('@/features/git/hooks/use-review-outline', () => ({
  useReviewOutline: (wsId: string | null, commit?: string) => {
    outlineCalls.push([wsId, commit])
    return { outline: [], loaded: true }
  },
}))

function renderTab(ambientWsId: string, propWsId: string) {
  const store = createWorkspaceStore(ambientWsId)
  return render(
    <WorkspaceStoreContext.Provider value={store}>
      <ReviewDiffTab onRetry={vi.fn()} wsId={propWsId} />
    </WorkspaceStoreContext.Provider>,
  )
}

describe('ReviewDiffTab — wsId scoping', () => {
  it('reads the DIFF for the wsId prop, not the ambient WorkspaceStoreContext', () => {
    filesSummaryCalls.length = 0
    outlineCalls.length = 0

    // Simulates a hidden keep-alive copy: this tab belongs to workspace
    // "ws-buffer" but is being rendered under a DIFFERENT workspace's ambient
    // context ("ws-ambient") because WorkspaceHost keeps that other workspace
    // mounted too.
    renderTab('ws-ambient', 'ws-buffer')

    expect(filesSummaryCalls).toEqual([['ws-buffer', undefined]])
    expect(outlineCalls).toEqual([['ws-buffer', undefined]])
  })
})
