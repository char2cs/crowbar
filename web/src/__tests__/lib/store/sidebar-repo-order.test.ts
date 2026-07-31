/**
 * Repos hold a manual order, and it has to survive the way they arrive.
 *
 * The failure this pins is not visible in a fresh snapshot: the REST list comes
 * back already sorted, so a reorder looks like it worked. It is the NEXT frame
 * that breaks it — one repo arriving on the entity stream, appended to the end
 * of the array whatever index it carries — which is a reorder that quietly
 * undoes itself a second after the drop.
 */
import { beforeEach, describe, expect, it } from 'vitest'
import { buildRepoTree, sortReposByOrder } from '@/lib/store/build-repo-tree'
import { useSidebarStore, type Repo } from '@/lib/store/sidebar'
import type { RepoDTO } from '@/lib/types'

const repo = (id: string, order?: number, projectId = 'p1'): Repo => ({
  id,
  projectId,
  name: id,
  avatarLabel: id[0].toUpperCase(),
  avatarColor: 'bg-indigo-700',
  workspaces: [],
  ...(order !== undefined && { order }),
})

const dto = (id: string, order: number, projectId = 'p1'): RepoDTO => ({
  id,
  projectId,
  name: id,
  path: `/${id}`,
  defaultBranch: 'main',
  avatarLabel: id[0].toUpperCase(),
  avatarColor: 'bg-indigo-700',
  avatarUrl: '',
  avatarEmoji: '',
  order,
})

const ids = () => useSidebarStore.getState().repos.map((r) => r.id)

beforeEach(() => {
  useSidebarStore.setState({
    repos: [],
    collapsedRepos: new Set<string>(),
    collapsedWorkspaces: new Set<string>(),
    collapsedProjects: new Set<string>(),
  })
})

describe('the sort', () => {
  it('orders by index, with a missing order last', () => {
    expect(
      sortReposByOrder([repo('c', 2), repo('a', 0), repo('nope'), repo('b', 1)]).map((r) => r.id),
    ).toEqual(['a', 'b', 'c', 'nope'])
  })

  it('keeps arrival order among ties, so an undragged level never reshuffles', () => {
    expect(sortReposByOrder([repo('x', 0), repo('y', 0), repo('z', 0)]).map((r) => r.id)).toEqual([
      'x',
      'y',
      'z',
    ])
  })

  it('is applied when the tree is built from the entity cache, which has no order of its own', () => {
    expect(buildRepoTree([dto('c', 2), dto('a', 0), dto('b', 1)], []).map((r) => r.id)).toEqual([
      'a',
      'b',
      'c',
    ])
  })
})

describe('a repo arriving on the stream', () => {
  it('lands at its persisted index, not at the end', () => {
    useSidebarStore.setState({ repos: [repo('a', 0), repo('b', 1), repo('c', 2)] })

    useSidebarStore.getState().mergeRepos([repo('first', -1)])

    expect(ids()).toEqual(['first', 'a', 'b', 'c'])
  })

  it('leaves a level alone when it only brings new workspaces', () => {
    const before = [repo('a', 0), repo('b', 1)]
    useSidebarStore.setState({ repos: before })

    useSidebarStore
      .getState()
      .mergeRepos([{ ...repo('b', 1), workspaces: [{ id: 'w', branch: 'w', age: '' }] }])

    expect(ids()).toEqual(['a', 'b'])
  })
})

describe('an optimistic reorder', () => {
  it('writes the index as well as the position, so the next frame cannot undo it', () => {
    useSidebarStore.setState({ repos: [repo('a', 0), repo('b', 1), repo('c', 2)] })

    useSidebarStore.getState().applyPlacement({
      repos: [
        { id: 'c', projectId: 'p1', order: 0 },
        { id: 'a', projectId: 'p1', order: 1 },
        { id: 'b', projectId: 'p1', order: 2 },
      ],
    })
    expect(ids()).toEqual(['c', 'a', 'b'])

    // A later repo frame re-sorts the level; the move has to survive it.
    useSidebarStore.getState().mergeRepos([repo('d', 3)])

    expect(ids()).toEqual(['c', 'a', 'b', 'd'])
  })
})
