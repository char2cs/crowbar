import { beforeEach, describe, expect, it, vi } from 'vitest'

const { syncBufferWithDisk, warning } = vi.hoisted(() => ({
  syncBufferWithDisk: vi.fn(async () => {}),
  warning: vi.fn(),
}))
vi.mock('@/features/workspace/lib/external-buffer-sync', () => ({ syncBufferWithDisk }))
vi.mock('@/features/window/stores/toast-store', () => ({ toast: { warning } }))
vi.mock('@/lib/persistence/workspace-layout', () => ({
  saveWorkspaceLayout: vi.fn().mockResolvedValue(undefined),
}))

import {
  resetWindowPaneStoreForTests,
  windowPaneStore,
} from '@/features/panes/stores/window-pane-store'
import { reloadTabFromDisk } from '@/features/tabs/lib/reload-tab'

function open(): string {
  return windowPaneStore.getState().bufferActions.openContent({
    type: 'editor',
    path: 'src/a.ts',
    name: 'a.ts',
    content: 'saved',
    workspaceId: 'ws-1',
  })
}

describe('reloadTabFromDisk (P0-9)', () => {
  beforeEach(() => {
    resetWindowPaneStoreForTests()
    syncBufferWithDisk.mockClear()
    warning.mockClear()
  })

  it('re-reads a clean buffer from disk, in place', () => {
    const id = open()
    reloadTabFromDisk(id)
    expect(syncBufferWithDisk).toHaveBeenCalledWith('ws-1', 'src/a.ts')
    expect(windowPaneStore.getState().buffers.map((b) => b.id)).toEqual([id])
  })

  it('never marks unsaved edits as saved — a dirty buffer is refused, untouched', () => {
    const id = open()
    windowPaneStore.setState((s) => {
      const buf = s.buffers[0]
      if (buf.type === 'editor') {
        buf.content = 'edited'
        buf.isDirty = true
      }
      return s
    })

    reloadTabFromDisk(id)

    expect(syncBufferWithDisk).not.toHaveBeenCalled()
    expect(warning).toHaveBeenCalled()
    const buf = windowPaneStore.getState().buffers[0]
    expect(buf.type === 'editor' && buf.isDirty && buf.content === 'edited').toBe(true)
  })
})
