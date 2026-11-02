import { beforeEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import { OperationSidebarRow } from '@/components/sidebar/operation-sidebar-row'
import type { SidebarRow as SidebarRowType } from '@/components/sidebar/types/sidebar-row'
import * as runOperation from '@/core/entities/run-operation'

vi.mock('@tanstack/react-router', () => ({ useNavigate: () => vi.fn() }))

const failedImport: SidebarRowType = {
  id: 'chat-a',
  kind: 'branch',
  parentId: 'home-row',
  order: Number.MAX_SAFE_INTEGER,
  label: 'bad/x',
  ownsWorktree: true,
  workspaceId: null,
  working: false,
  hasView: false,
  operation: {
    opId: 'op-1',
    state: 'failed',
    problem: { code: 'internal', message: 'fatal: could not create directory: Not a directory' },
  },
}

beforeEach(() => vi.restoreAllMocks())

describe('a create drawn from its operation', () => {
  it('shows the branch name and the reason an import failed', () => {
    render(<OperationSidebarRow row={failedImport} depth={0} />)
    expect(screen.getByText('bad/x')).toBeInTheDocument()
    expect(screen.getByText('failed')).toBeInTheDocument()
    expect(screen.getByText(/Not a directory/)).toBeInTheDocument()
  })

  it('Dismiss dismisses the operation', () => {
    const dismiss = vi.spyOn(runOperation, 'dismissOperation').mockResolvedValue(undefined)
    render(<OperationSidebarRow row={failedImport} depth={0} />)
    fireEvent.click(screen.getByRole('button', { name: 'Dismiss' }))
    expect(dismiss).toHaveBeenCalledExactlyOnceWith('op-1')
  })

  it('a running create draws a spinner and no reason', () => {
    const running = {
      ...failedImport,
      operation: { opId: 'op-1', state: 'running' as const, problem: null },
    }
    render(<OperationSidebarRow row={running} depth={0} />)
    expect(screen.getByText('bad/x')).toBeInTheDocument()
    expect(screen.queryByText('failed')).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Dismiss' })).not.toBeInTheDocument()
  })

  // An unlabeled empty input read as a chat box: what got typed became the branch name.
  it('a draft shows the branch-name placeholder on its naming input', () => {
    const draft: SidebarRowType = { ...failedImport, operation: undefined, draft: true, label: '' }
    render(<OperationSidebarRow row={draft} depth={0} />)
    expect(screen.getByRole('textbox')).toHaveAttribute('placeholder', 'branch-name')
  })
})
