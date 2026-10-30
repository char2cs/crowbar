import { beforeEach, describe, expect, it, vi } from 'vitest'
import { performImportBranches } from '@/components/sidebar/lib/import-branches'
import { rowsFromOperations } from '@/components/sidebar/lib/rows-from-operations'
import { getInitialState, useSidebarStore } from '@/lib/store/sidebar'
import { useOperationsStore } from '@/lib/store/operations'
import { ApiError } from '@/lib/errors/api-error'
import * as api from '@/lib/api'

const importBranches = vi.spyOn(api, 'importBranches')

beforeEach(() => {
  importBranches.mockReset()
  useOperationsStore.setState({ ops: {}, latestFor: {} })
  useSidebarStore.setState({
    ...getInitialState(),
    repos: [
      {
        id: 'repo-1',
        projectId: 'proj-1',
        name: 'crowbar',
        avatarLabel: 'C',
        avatarColor: 'bg-indigo-700',
        defaultWorkspaceId: 'ws-home',
        defaultOwningChatId: 'home-row',
        defaultBranch: 'main',
        workspaces: [],
        folders: [],
        chats: [],
      },
    ],
  })
})

const opRows = () =>
  rowsFromOperations(Object.values(useOperationsStore.getState().ops), { projectId: 'proj-1' })

describe('performImportBranches', () => {
  it('import sends one minted chat id per branch and the lock flag', () => {
    importBranches.mockReturnValue(new Promise(() => {}))

    performImportBranches('repo-1', ['feature-a', 'feature-b', 'release'], ['release'])

    expect(importBranches).toHaveBeenCalledTimes(2)
    const [locked, open] = importBranches.mock.calls
    expect(locked?.slice(0, 4)).toEqual([
      'proj-1',
      'repo-1',
      [{ name: 'release', chatId: expect.any(String) }],
      true,
    ])
    expect(open?.slice(0, 4)).toEqual([
      'proj-1',
      'repo-1',
      [
        { name: 'feature-a', chatId: expect.any(String) },
        { name: 'feature-b', chatId: expect.any(String) },
      ],
      false,
    ])
    const chatIds = [...locked![2], ...open![2]].map((b) => b.chatId)
    expect(new Set(chatIds).size).toBe(3)
    // Each branch is drawn at once, by the id the daemon will create its chat under.
    expect(opRows().map((r) => [r.id, r.label])).toEqual(
      expect.arrayContaining([
        [open![2][0]!.chatId, 'feature-a'],
        [open![2][1]!.chatId, 'feature-b'],
        [locked![2][0]!.chatId, 'release'],
      ]),
    )
  })

  it('a refused import (409) shows its reason on each branch row', async () => {
    importBranches.mockRejectedValue(new ApiError('chat id already taken', 409, 'id_conflict'))

    performImportBranches('repo-1', ['feature-a', 'feature-b'])

    await vi.waitFor(() =>
      expect(opRows()).toMatchObject([
        { label: 'feature-a', operation: { state: 'failed', problem: { code: 'id_conflict' } } },
        {
          label: 'feature-b',
          operation: { state: 'failed', problem: { message: 'chat id already taken' } },
        },
      ]),
    )
  })

  it('importing an empty branch list sends nothing', () => {
    performImportBranches('repo-1', [])
    expect(importBranches).not.toHaveBeenCalled()
  })
})
