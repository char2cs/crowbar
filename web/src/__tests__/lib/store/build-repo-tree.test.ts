import { describe, expect, it } from 'vitest'
import { buildRepoTree, type RepoDTO, type WorkspaceDTO } from '@/lib/store/build-repo-tree'

const repo = (id: string, name: string): RepoDTO => ({
  id,
  projectId: 'p1',
  name,
  path: `/p/${name}`,
  defaultBranch: 'main',
  avatarLabel: name[0].toUpperCase(),
  avatarColor: 'avatar-emerald',
})

const ws = (id: string, repoId: string, over: Partial<WorkspaceDTO> = {}): WorkspaceDTO => ({
  id,
  repoId,
  projectId: 'p1',
  branch: 'main',
  status: 'new',
  locked: false,
  hasConflicts: false,
  added: 0,
  deleted: 0,
  mergeStrategy: 'merge',
  agentRunning: false,
  ...over,
})

describe('buildRepoTree', () => {
  it('groups workspaces under their repo and always sets a workspaces array', () => {
    const tree = buildRepoTree(
      [repo('r1', 'alpha'), repo('r2', 'beta')],
      [ws('w1', 'r1'), ws('w2', 'r1'), ws('w3', 'r2')],
    )
    expect(tree).toHaveLength(2)
    expect(tree[0].workspaces.map((w) => w.id)).toEqual(['w1', 'w2'])
    expect(tree[1].workspaces.map((w) => w.id)).toEqual(['w3'])
  })

  it('gives a repo with no workspaces an empty array (no crash in the sidebar)', () => {
    const tree = buildRepoTree([repo('r1', 'alpha')], [])
    expect(tree[0].workspaces).toEqual([])
  })

  it('maps agentRunning and locked into the sidebar status', () => {
    const tree = buildRepoTree(
      [repo('r1', 'alpha')],
      [
        ws('w1', 'r1', { agentRunning: true }),
        ws('w2', 'r1', { locked: true }),
        ws('w3', 'r1', { status: 'new' }),
      ],
    )
    const [a, b, c] = tree[0].workspaces
    expect(a.status).toBe('agent-running')
    expect(b.status).toBe('locked')
    expect(c.status).toBe('new')
  })
})
