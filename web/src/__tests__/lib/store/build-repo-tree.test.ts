import { describe, expect, it } from 'vitest'
import { buildRepoTree, toSidebarRepo, toSidebarWorkspace } from '@/lib/store/build-repo-tree'
import type { RepoDTO, WorkspaceDTO } from '@/lib/types'

const repo = (id: string, name: string, over: Partial<RepoDTO> = {}): RepoDTO => ({
  id,
  projectId: 'p1',
  name,
  path: `/p/${name}`,
  defaultBranch: 'main',
  avatarLabel: name[0].toUpperCase(),
  avatarColor: 'avatar-emerald',
  avatarUrl: '',
  avatarEmoji: '',
  ...over,
})

const ws = (id: string, repoId: string, over: Partial<WorkspaceDTO> = {}): WorkspaceDTO => ({
  id,
  repoId,
  projectId: 'p1',
  branch: 'main',
  parentId: '',
  forkPointSha: '',
  status: 'new',
  working: false,
  lastError: '',
  added: 0,
  deleted: 0,
  mergeStrategy: 'merge',
  canMergeLocally: false,
  mergeConflicts: false,
  parentBranch: '',
  prUrl: '',
  prTitle: '',
  prTargetBranch: '',
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

  it('carries parentId through so the sidebar can nest children', () => {
    const tree = buildRepoTree(
      [repo('r1', 'alpha')],
      [ws('w1', 'r1'), ws('w2', 'r1', { parentId: 'w1' })],
    )
    const [parent, child] = tree[0].workspaces
    expect(parent.parentId).toBeUndefined()
    expect(child.parentId).toBe('w1')
  })

  it('gives a repo with no workspaces an empty array (no crash in the sidebar)', () => {
    const tree = buildRepoTree([repo('r1', 'alpha')], [])
    expect(tree[0].workspaces).toEqual([])
  })

  it('passes the §5 status straight through (locked/pr-conflicts/deleted are statuses)', () => {
    const tree = buildRepoTree(
      [repo('r1', 'alpha')],
      [
        ws('w1', 'r1', { status: 'locked' }),
        ws('w2', 'r1', { status: 'pr-conflicts' }),
        ws('w3', 'r1', { status: 'deleted' }),
        ws('w4', 'r1', { status: 'new' }),
      ],
    )
    const [a, b, c, d] = tree[0].workspaces
    expect(a.status).toBe('locked')
    expect(b.status).toBe('pr-conflicts')
    expect(c.status).toBe('deleted')
    expect(d.status).toBe('new')
  })

  it('maps `working` through as a separate flag (replaces agent-running)', () => {
    const tree = buildRepoTree(
      [repo('r1', 'alpha')],
      [ws('w1', 'r1', { working: true, status: 'new' })],
    )
    expect(tree[0].workspaces[0].working).toBe(true)
    expect(tree[0].workspaces[0].status).toBe('new')
  })

  it('carries canMergeLocally + parentBranch + prUrl onto the sidebar workspace', () => {
    const tree = buildRepoTree(
      [repo('r1', 'alpha')],
      [
        ws('w1', 'r1', {
          canMergeLocally: true,
          mergeConflicts: true,
          parentBranch: 'develop',
          prUrl: 'https://example.com/pr/1',
        }),
      ],
    )
    const w = tree[0].workspaces[0]
    expect(w.canMergeLocally).toBe(true)
    expect(w.mergeConflicts).toBe(true)
    expect(w.parentBranch).toBe('develop')
    expect(w.prUrl).toBe('https://example.com/pr/1')
  })

  it('maps avatarUrl from RepoDTO to Repo.avatarURL', () => {
    const repos = buildRepoTree([repo('r1', 'my-repo', { avatarUrl: '/v0/icon.png' })], [])
    expect(repos[0].avatarURL).toBe('/v0/icon.png')
  })

  it('prefers a custom emoji over the proxied icon url', () => {
    const repos = buildRepoTree(
      [repo('r1', 'my-repo', { avatarUrl: '/v0/icon.png', avatarEmoji: '🚀' })],
      [],
    )
    expect(repos[0].avatarURL).toBe('emoji:🚀')
  })

  it('leaves avatarURL undefined when neither avatarUrl nor avatarEmoji is set', () => {
    const repos = buildRepoTree([repo('r1', 'repo')], [])
    expect(repos[0].avatarURL).toBeUndefined()
  })

  it('pulls the isDefault workspace out of the tree and onto the repo', () => {
    const tree = buildRepoTree(
      [repo('r1', 'crowbar')],
      [
        ws('w-default', 'r1', { isDefault: true, branch: 'develop' }),
        ws('w-child', 'r1', { branch: 'feature/x' }),
      ],
    )
    expect(tree[0].workspaces.map((w) => w.id)).toEqual(['w-child'])
    expect(tree[0].defaultWorkspaceId).toBe('w-default')
  })

  it('leaves defaultWorkspaceId undefined when no workspace is the default', () => {
    const tree = buildRepoTree([repo('r1', 'crowbar')], [ws('w1', 'r1')])
    expect(tree[0].defaultWorkspaceId).toBeUndefined()
  })
})

function makeWs(over: Partial<WorkspaceDTO> & { id: string }): WorkspaceDTO {
  return {
    repoId: 'r1', projectId: 'p1', branch: 'main', parentId: '',
    forkPointSha: '', status: 'new', working: false, lastError: '', added: 0,
    deleted: 0, mergeStrategy: 'merge', canMergeLocally: false, mergeConflicts: false,
    parentBranch: '', prUrl: '', prTitle: '', prTargetBranch: '', ...over,
  } as WorkspaceDTO
}
const baseRepo: RepoDTO = { id: 'r1', projectId: 'p1', name: 'crowbar' } as RepoDTO

describe('toSidebarWorkspace heldByPath', () => {
  it('maps heldByPath from the DTO', () => {
    const w = toSidebarWorkspace(ws('w1', 'r1', { heldByPath: '/repo' }))
    expect(w.heldByPath).toBe('/repo')
  })

  it('maps heldByPath to "" when absent (overwrites a stale value on Retry)', () => {
    const w = toSidebarWorkspace(ws('w1', 'r1'))
    expect(w.heldByPath).toBe('')
  })
})

describe('toSidebarRepo defaultBranch', () => {
  it('sets defaultBranch from the isDefault workspace', () => {
    const out = toSidebarRepo(baseRepo, [
      makeWs({ id: 'd', branch: 'develop', isDefault: true }),
      makeWs({ id: 'c', branch: 'feature/x' }),
    ])
    expect(out.defaultBranch).toBe('develop')
    expect(out.defaultWorkspaceId).toBe('d')
  })

  it('leaves defaultBranch undefined when there is no default workspace', () => {
    const out = toSidebarRepo(baseRepo, [makeWs({ id: 'c', branch: 'feature/x' })])
    expect(out.defaultBranch).toBeUndefined()
  })
})
