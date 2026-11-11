import { act, render } from '@testing-library/react'
import { beforeEach, describe, expect, it } from 'vitest'
import { SidebarRowGlyph } from '@/components/sidebar/sidebar-row-glyph'
import type { SidebarRow as SidebarRowType } from '@/components/sidebar/types/sidebar-row'
import { streamOperation, resetOperations } from '@/__tests__/__fixtures__/operations'

// A branch row's id is its chat's; the operation names the workspace.
const row: SidebarRowType = {
  id: 'chat-1',
  kind: 'branch',
  parentId: null,
  order: 0,
  label: 'feature/a',
  ownsWorktree: true,
  workspaceId: 'ws1',
  working: false,
  hasView: false,
}

beforeEach(() => resetOperations())

describe('SidebarRowGlyph', () => {
  it('a workspace row spins while an operation naming it runs', () => {
    const { container } = render(
      <SidebarRowGlyph row={row} isProjectHome={false} readOnlyHeader={false} expanded />,
    )
    const spinner = () => container.querySelector('[data-flicker-spinner]')
    expect(spinner()).toBeNull()

    const op = {
      id: 'op1',
      kind: 'git.push',
      subject: { kind: 'workspace' as const, id: 'ws1' },
      projectId: 'p1',
      label: 'Push feature/a',
      state: 'running' as const,
      items: [],
      dismissed: false,
      version: 2,
      createdAt: '2026-10-28T12:00:00Z',
      updatedAt: '2026-10-28T12:00:00Z',
    }
    act(() => streamOperation(op))
    expect(spinner()).toBeInTheDocument()

    act(() => streamOperation({ ...op, state: 'succeeded', version: 3 }))
    expect(spinner()).toBeNull()
  })
})
