import { beforeEach, describe, expect, it, vi } from 'vitest'
import { performImportBranches } from '@/components/sidebar/lib/import-branches'
import { operationRows } from '@/__tests__/__fixtures__/tree-rows'
import { resetOperations } from '@/__tests__/__fixtures__/operations'
import { ApiError } from '@/lib/errors/api-error'
import * as api from '@/lib/api'
import { seedRepos } from '@/__tests__/__fixtures__/sidebar-entities'

const importBranches = vi.spyOn(api, 'importBranches')

beforeEach(() => {
  importBranches.mockReset()
  resetOperations()
  seedRepos([
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
  ])
})

const opRows = () => operationRows('proj-1')

describe('performImportBranches', () => {
  it('a mixed lock choice is one import, each branch carrying its own lock', () => {
    importBranches.mockReturnValue(new Promise(() => {}))

    performImportBranches('repo-1', ['feature-a', 'release'], ['release'])

    expect(importBranches).toHaveBeenCalledTimes(1)
    const [projectId, repoId, sent] = importBranches.mock.calls[0]!
    expect([projectId, repoId]).toEqual(['proj-1', 'repo-1'])
    expect(sent).toEqual([
      { name: 'feature-a', chatId: expect.any(String), lock: false },
      { name: 'release', chatId: expect.any(String), lock: true },
    ])
    expect(new Set(sent.map((b) => b.chatId)).size).toBe(2)
    // Each branch is drawn at once, by the id the daemon will create its chat under.
    expect(opRows().map((r) => [r.id, r.label])).toEqual([
      [sent[0]!.chatId, 'feature-a'],
      [sent[1]!.chatId, 'release'],
    ])
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
