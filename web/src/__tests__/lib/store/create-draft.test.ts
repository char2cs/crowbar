import { beforeEach, describe, expect, it } from 'vitest'
import { useCreateDraftStore, type CreateDraft } from '@/lib/store/create-draft'

const draft = (over: Partial<CreateDraft> = {}): CreateDraft => ({
  kind: 'branch',
  projectId: 'p1',
  repoId: 'r1',
  workspaceId: 'ws-1',
  parentId: 'row-1',
  order: 2,
  ownsWorktree: true,
  ...over,
})

beforeEach(() => useCreateDraftStore.setState({ draft: null }))

describe('create-draft', () => {
  it('start opens the naming input; a second start replaces it', () => {
    useCreateDraftStore.getState().start(draft())
    useCreateDraftStore.getState().start(draft({ parentId: 'row-2' }))
    expect(useCreateDraftStore.getState().draft).toEqual(draft({ parentId: 'row-2' }))
  })

  it('cancel clears the draft', () => {
    useCreateDraftStore.getState().start(draft())
    useCreateDraftStore.getState().cancel()
    expect(useCreateDraftStore.getState().draft).toBeNull()
  })
})
