import { afterEach, describe, expect, it, vi } from 'vitest'
import * as hover from '@/components/sidebar/lib/sidebar-drag-hover'
import { dispatchDrop } from '@/components/sidebar/lib/sidebar-drop-dispatch'
import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'

const row: SidebarRow = {
  id: 'a',
  kind: 'branch',
  parentId: null,
  order: 0,
  label: 'a',
  ownsWorktree: true,
  workspaceId: 'a',
  working: false,
  hasView: false,
}

afterEach(() => {
  vi.restoreAllMocks()
})

describe('dispatchDrop', () => {
  it.each([
    ['lives in Recents', true, true],
    ['is a tree row', false, false],
    ['publishes no Recents flag', undefined, false],
  ])('tells onDrop whether the row released over %s', (_, inRecents, expected) => {
    vi.spyOn(hover, 'hitTest').mockReturnValue({
      kind: 'row',
      row: { ...row, mode: 'into', inRecents },
      rect: { top: 0, bottom: 36, left: 0, right: 200, width: 200, height: 36 },
    } as ReturnType<typeof hover.hitTest>)
    const onDrop = vi.fn()

    dispatchDrop([], 10, 10, { subjectsFor: () => [row], onDrop, onPaneDrop: vi.fn() })

    expect(onDrop).toHaveBeenCalledWith([], row, 'into', expected)
  })
})
