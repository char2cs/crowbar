import { act, cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createWorkspaceStore } from '@/features/workspace/stores/workspace-store'
import { WorkspaceStoreContext } from '@/features/workspace/stores/workspace-context'
import { ReviewDiffTab } from '@/features/git/components/review-diff-tab'
import { getDefaultSettingsSnapshot } from '@/features/settings/config/default-settings'
import { useSettingsStore } from '@/features/settings/store'
import type { GitDiff } from '@/features/git/types/git-types'

// Live-reported: the Split/Inline toggle reset to Split every time the tab
// was reopened. It's a display preference like sidebarPosition/theme, so it
// belongs in the settings store (persisted) rather than component state.
const oneFile: GitDiff = {
  file_path: 'src/pricing.ts',
  is_new: false,
  is_deleted: false,
  is_renamed: false,
  lines: [],
}
vi.mock('@/features/git/hooks/use-review-files-summary', () => ({
  useReviewFilesSummary: () => ({ files: [oneFile], loaded: true }),
}))
vi.mock('@/features/git/hooks/use-review-outline', () => ({
  useReviewOutline: () => ({ outline: [], loaded: true }),
}))
// The real ReviewCodeView pulls in the full diff-rendering/highlight-worker
// stack — irrelevant to this toggle, which lives entirely in the toolbar
// above it.
vi.mock('@/features/git/components/diff/review-code-view', () => ({
  ReviewCodeView: () => null,
}))

function renderTab() {
  const store = createWorkspaceStore('ws-1')
  return render(
    <WorkspaceStoreContext.Provider value={store}>
      <ReviewDiffTab onRetry={vi.fn()} wsId="ws-1" />
    </WorkspaceStoreContext.Provider>,
  )
}

describe('ReviewDiffTab — diff view mode persistence', () => {
  beforeEach(() => {
    act(() => {
      useSettingsStore.setState({ settings: getDefaultSettingsSnapshot() })
    })
  })

  afterEach(() => {
    cleanup()
  })

  it('defaults to split and clicking Inline persists it to the settings store', async () => {
    const user = userEvent.setup()
    renderTab()

    expect(screen.getByRole('button', { name: 'Side-by-side diff' })).toHaveAttribute(
      'aria-pressed',
      'true',
    )

    await user.click(screen.getByRole('button', { name: 'Inline diff' }))

    expect(useSettingsStore.getState().settings.diffViewMode).toBe('unified')
    expect(screen.getByRole('button', { name: 'Inline diff' })).toHaveAttribute(
      'aria-pressed',
      'true',
    )
  })

  it('opens already on Inline when that was the last persisted choice', async () => {
    await act(async () => {
      await useSettingsStore.getState().updateSetting('diffViewMode', 'unified')
    })

    renderTab()

    expect(screen.getByRole('button', { name: 'Inline diff' })).toHaveAttribute(
      'aria-pressed',
      'true',
    )
    expect(screen.getByRole('button', { name: 'Side-by-side diff' })).toHaveAttribute(
      'aria-pressed',
      'false',
    )
  })
})
