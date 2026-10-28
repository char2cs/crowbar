import { describe, expect, it } from 'vitest'
import type { Repo } from '@/lib/store/sidebar'
import { workspaceNames } from '@/lib/workspace/workspace-branch'

function repo(name: string, workspaces: { id: string; branch: string }[]): Repo {
  return { id: name, name, workspaces } as unknown as Repo
}

const repos = [
  repo('shop', [
    { id: 'ws-1', branch: 'feat/a' },
    { id: 'ws-2', branch: 'main' },
    { id: 'ws-4', branch: '' },
  ]),
  repo('billing', [{ id: 'ws-3', branch: 'main' }]),
]

describe('workspaceNames', () => {
  it('names a workspace by its branch', () => {
    expect(workspaceNames(repos, ['ws-1', 'ws-2'])).toEqual(
      new Map([
        ['ws-1', 'feat/a'],
        ['ws-2', 'main'],
      ]),
    )
  })

  it('qualifies a branch two repos share with its repo', () => {
    expect(workspaceNames(repos, ['ws-2', 'ws-3'])).toEqual(
      new Map([
        ['ws-2', 'shop/main'],
        ['ws-3', 'billing/main'],
      ]),
    )
  })

  it('falls back to the id for a workspace with no branch or none known', () => {
    expect(workspaceNames(repos, ['ws-4', 'ws-gone'])).toEqual(
      new Map([
        ['ws-4', 'ws-4'],
        ['ws-gone', 'ws-gone'],
      ]),
    )
  })
})
