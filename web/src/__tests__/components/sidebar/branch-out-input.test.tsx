import { beforeEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import { BranchOutInput } from '@/components/sidebar/branch-out-input'
import { treeUiStore } from '@/features/workspace/tree/tree-ui-store'
import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'
import * as chatCommands from '@/features/agent/api/agent-chat-commands'

const row = {
  id: 'thread-1',
  kind: 'chat',
  parentId: null,
  order: 0,
  label: 'A thread',
  ownsWorktree: false,
  workspaceId: 'ws-main',
  working: false,
  hasView: false,
} satisfies SidebarRow

describe('BranchOutInput', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
    treeUiStore.getState().stopBranching()
  })

  it('offers a generated name, and Enter makes the branch under the name typed', () => {
    const branchOut = vi.spyOn(chatCommands, 'branchOutChat').mockResolvedValue(undefined)
    treeUiStore.getState().startBranching(row.id)
    render(<BranchOutInput row={row} />)
    const input = screen.getByRole<HTMLInputElement>('textbox')
    expect(input.value).toMatch(/^chat-[0-9a-f]{8}$/)

    fireEvent.change(input, { target: { value: 'heron-work' } })
    fireEvent.keyDown(input, { key: 'Enter' })

    expect(branchOut).toHaveBeenCalledWith('ws-main', 'thread-1', 'heron-work')
    expect(treeUiStore.getState().branching).toBeNull()
  })

  it('Escape makes no branch', () => {
    const branchOut = vi.spyOn(chatCommands, 'branchOutChat').mockResolvedValue(undefined)
    treeUiStore.getState().startBranching(row.id)
    render(<BranchOutInput row={row} />)
    fireEvent.keyDown(screen.getByRole('textbox'), { key: 'Escape' })
    expect(branchOut).not.toHaveBeenCalled()
    expect(treeUiStore.getState().branching).toBeNull()
  })
})
