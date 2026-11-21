import { afterEach, describe, expect, it } from 'vitest'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { ROOT_PANE_ID } from '@/features/panes/constants/pane'
import { makePane } from '@/features/panes/lib/view-state'
import { openReviewBuffer, revealInActiveWorkspace } from '@/entries/open-review-buffer'
import {
  getOrCreateWorkspaceStore,
  setActiveWorkspaceId,
} from '@/features/workspace/stores/workspace-store-registry'

afterEach(() => {
  windowPaneStore.setState({ buffers: [] })
  setActiveWorkspaceId(null)
})

interface Case {
  name: string
  saved: unknown
  reveal: (wsId: string) => void
  expected: { revealFilePath: string | null; revealFileNonce?: number; view?: string }
}

const cases: Case[] = [
  {
    name: 'opens the branch review with nothing revealed',
    saved: null,
    reveal: () => undefined,
    expected: { revealFilePath: null },
  },
  {
    name: 'opens on Files and reveals the file the shell names in the saved state',
    saved: { reveal: 'src/a.ts' },
    reveal: () => undefined,
    expected: { revealFilePath: 'src/a.ts', view: 'files' },
  },
  {
    name: 'reveals a file in a review already open, again for the same file',
    saved: null,
    reveal: (wsId) => {
      setActiveWorkspaceId(wsId)
      revealInActiveWorkspace('b.ts')
      revealInActiveWorkspace('b.ts')
    },
    expected: { revealFilePath: 'b.ts', revealFileNonce: 2, view: 'files' },
  },
  {
    name: "reveals the shell's review.reveal in the workspace the pane shows",
    saved: null,
    reveal: (wsId) => {
      setActiveWorkspaceId(wsId)
      revealInActiveWorkspace('c.ts')
    },
    expected: { revealFilePath: 'c.ts', view: 'files' },
  },
]

describe('openReviewBuffer', () => {
  it.each(cases.map((c, i) => [c.name, c, `ws${i}`] as const))('%s', async (_, c, wsId) => {
    const { panes } = windowPaneStore.getState()
    windowPaneStore.setState({ panes: { ...panes, [ROOT_PANE_ID]: makePane(ROOT_PANE_ID, null) } })
    await openReviewBuffer(wsId, c.saved, () => undefined)
    c.reveal(wsId)
    const { view, ...reveal } = c.expected
    const { branchReview } = getOrCreateWorkspaceStore(wsId).getState()
    expect(branchReview).toMatchObject(reveal)
    const review = windowPaneStore.getState().buffers.find((b) => b.type === 'branchReview')
    expect(review).toMatchObject({ type: 'branchReview', wsId, ...(view ? { view } : {}) })
  })
})
