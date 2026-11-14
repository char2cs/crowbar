import { describe, expect, it, vi } from 'vitest'
import { render } from '@testing-library/react'
import { SidebarRow } from '@/components/sidebar/sidebar-row'
import type { SidebarRow as SidebarRowType } from '@/components/sidebar/types/sidebar-row'

vi.mock('@/features/window/stores/toast-store', () => ({
  toast: { error: vi.fn(), success: vi.fn(), info: vi.fn(), show: vi.fn() },
}))

/**
 * A row held in the removal tray stays on screen, transformed in place, with
 * whatever is nested under it still drawn beneath it — so it has to keep
 * publishing the id every DOM-keyed consumer resolves a row by
 * (`row-context-menu.tsx`, `sidebar-tree-chrome.tsx`). Dropping the attribute
 * made a held FOLDER read as a row that is not there at all: its children were
 * still drawn, one indent deeper, parented by `data-sidebar-drop-parent` to an
 * id no rendered row published, and a right-click on the held row opened no
 * menu.
 */

const heldFolder: SidebarRowType = {
  id: 'folder-1',
  kind: 'folder',
  parentId: null,
  order: 0,
  label: 'homefolder-RN',
  ownsWorktree: false,
  workspaceId: null,
  working: false,
  hasView: false,
  removal: { entryId: 'entry-1', deadlineAt: Date.now() + 8000, extra: 0 },
}

describe('a row held for removal', () => {
  it('still publishes its own row id', () => {
    const { container } = render(<SidebarRow row={heldFolder} depth={0} onOpen={vi.fn()} />)
    expect(container.querySelector('[data-sidebar-row-id="folder-1"]')).not.toBeNull()
  })

  it('draws at the same depth its untransformed row would', () => {
    const { container: held } = render(<SidebarRow row={heldFolder} depth={2} onOpen={vi.fn()} />)
    const { removal: _removal, ...atRest } = heldFolder
    const { container: rest } = render(<SidebarRow row={atRest} depth={2} onOpen={vi.fn()} />)
    const indentOf = (c: HTMLElement) =>
      (c.firstElementChild as HTMLElement | null)?.style.marginInlineStart
    expect(indentOf(held)).toBe(indentOf(rest))
  })

  // It is on its way out: it must not accept a drop. The identity attribute is
  // not a drag attribute, and restoring one must not restore the other.
  it('is still not a drop target', () => {
    const { container } = render(<SidebarRow row={heldFolder} depth={0} onOpen={vi.fn()} />)
    expect(container.querySelector('[data-sidebar-folder-drop]')).toBeNull()
    expect(container.querySelector('[data-sidebar-drop-parent]')).toBeNull()
  })

  it('keeps its Keep control', () => {
    const { container } = render(<SidebarRow row={heldFolder} depth={0} onOpen={vi.fn()} />)
    expect(container.querySelector('[aria-label="Keep homefolder-RN"]')).not.toBeNull()
  })
})
