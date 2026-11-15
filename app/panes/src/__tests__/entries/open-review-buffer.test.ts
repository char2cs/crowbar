import { afterEach, describe, expect, it } from 'vitest'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { ROOT_PANE_ID } from '@/features/panes/constants/pane'
import { makePane } from '@/features/panes/lib/view-state'
import { openReviewBuffer } from '@/entries/open-review-buffer'

afterEach(() => windowPaneStore.setState({ buffers: [] }))

describe('openReviewBuffer', () => {
  it('opens the branch review of the workspace in the root pane', async () => {
    const { panes } = windowPaneStore.getState()
    windowPaneStore.setState({ panes: { ...panes, [ROOT_PANE_ID]: makePane(ROOT_PANE_ID, null) } })
    await openReviewBuffer('ws1', null)
    const review = windowPaneStore.getState().buffers.find((b) => b.type === 'branchReview')
    expect(review).toMatchObject({ type: 'branchReview', wsId: 'ws1' })
  })
})
