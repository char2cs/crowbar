import { describe, it, expect } from 'vitest'
import { createWorkspaceStore } from '@/features/workspace/stores/workspace-store'

describe('file tree slice', () => {
  it('starts idle with no files', () => {
    const state = createWorkspaceStore('ws-A').getState()
    expect(state.fileTreeStatus).toBe('idle')
    expect(state.files).toEqual([])
  })

  it('keeps each workspace store’s tree to itself', () => {
    const a = createWorkspaceStore('ws-A')
    const b = createWorkspaceStore('ws-B')
    const tree = [{ name: 'a.ts', path: 'a.ts', isDir: false }]

    a.getState().fileTreeActions.setFiles(tree)

    expect(a.getState().files).toBe(tree)
    expect(b.getState().files).toEqual([])
  })
})
