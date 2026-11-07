import { beforeEach, describe, expect, it, vi } from 'vitest'
import { act, fireEvent, render, screen } from '@testing-library/react'
import { BranchOutInput } from '@/components/sidebar/branch-out-input'
import { treeUiStore } from '@/features/workspace/tree/tree-ui-store'
import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'
import * as chatCommands from '@/features/agent/api/agent-chat-commands'
import { ApiError } from '@/lib/errors/api-error'

const row = {
  id: 'thread-1',
  kind: 'chat',
  parentId: null,
  order: 0,
  label: 'A thread',
  ownsWorktree: false,
  workspaceId: 'ws-main',
  canBranchOut: true,
  working: false,
  hasView: false,
} satisfies SidebarRow

function open() {
  treeUiStore.getState().startBranching(row.id)
  render(<BranchOutInput row={row} />)
  return screen.getByRole('textbox')
}

describe('BranchOutInput', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
    treeUiStore.getState().stopBranching()
  })

  it('Enter makes the branch under the name typed, and closes the input', async () => {
    const branchOut = vi.spyOn(chatCommands, 'branchOutChat').mockResolvedValue(undefined)
    const input = open()
    expect(input).toHaveValue('')
    expect(input).toHaveAttribute('placeholder', 'branch-name')

    fireEvent.change(input, { target: { value: 'feature-x' } })
    await act(async () => fireEvent.keyDown(input, { key: 'Enter' }))

    expect(branchOut).toHaveBeenCalledWith('ws-main', 'thread-1', 'feature-x')
    expect(treeUiStore.getState().branching).toBeNull()
  })

  it('says why the daemon refused a name and stays open to fix it', async () => {
    const taken = new ApiError('A branch named "main" already exists', 409, 'conflict')
    const branchOut = vi
      .spyOn(chatCommands, 'branchOutChat')
      .mockRejectedValueOnce(taken)
      .mockResolvedValueOnce(undefined)
    const input = open()
    fireEvent.change(input, { target: { value: 'main' } })
    await act(async () => fireEvent.keyDown(input, { key: 'Enter' }))

    expect(screen.getByRole('alert')).toHaveTextContent('A branch named "main" already exists')
    expect(input).toHaveAttribute('aria-invalid', 'true')
    expect(treeUiStore.getState().branching).toBe('thread-1')

    fireEvent.change(input, { target: { value: 'feature-x' } })
    await act(async () => fireEvent.keyDown(input, { key: 'Enter' }))
    expect(branchOut).toHaveBeenLastCalledWith('ws-main', 'thread-1', 'feature-x')
    expect(treeUiStore.getState().branching).toBeNull()
  })

  for (const [how, leave] of [
    ['Escape', (input: HTMLElement) => fireEvent.keyDown(input, { key: 'Escape' })],
    ['blur', (input: HTMLElement) => fireEvent.blur(input)],
    ['an empty name', (input: HTMLElement) => fireEvent.keyDown(input, { key: 'Enter' })],
  ] as const) {
    it(`${how} makes no branch and closes the input`, () => {
      const branchOut = vi.spyOn(chatCommands, 'branchOutChat').mockResolvedValue(undefined)
      const input = open()
      if (how !== 'an empty name') fireEvent.change(input, { target: { value: 'feature-x' } })
      leave(input)
      expect(branchOut).not.toHaveBeenCalled()
      expect(treeUiStore.getState().branching).toBeNull()
    })
  }
})
