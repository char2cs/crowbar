import { expect, test } from 'vitest'
import { getPostDeleteNavigationTarget } from '@/lib/store/sidebar'
import type { Repo } from '@/lib/store/sidebar'

// BUG-007: after drag-to-delete of the ACTIVE workspace the app must navigate
// to the parent, else the repo's base (locked) workspace, else null (caller
// falls back to /projects).

function repoWith(workspaces: Repo['workspaces']): Repo[] {
  return [
    {
      id: 'repo-1',
      name: 'crowbar',
      avatarLabel: 'C',
      avatarColor: 'bg-indigo-700',
      workspaces,
    },
  ]
}

test('targets the parent workspace when it survives the delete', () => {
  const repos = repoWith([
    { id: 'base', branch: 'develop', status: 'locked', age: '—' },
    { id: 'parent', branch: 'feat/parent', parentId: 'base', status: 'new', age: '—' },
    { id: 'child', branch: 'feat/child', parentId: 'parent', status: 'new', age: '—' },
  ])
  expect(getPostDeleteNavigationTarget(repos, 'child')).toBe('parent')
})

test('falls back to the repo base workspace for a root-level workspace', () => {
  const repos = repoWith([
    { id: 'base', branch: 'develop', status: 'locked', age: '—' },
    { id: 'root-ws', branch: 'feat/root', status: 'new', age: '—' },
  ])
  expect(getPostDeleteNavigationTarget(repos, 'root-ws')).toBe('base')
})

test('descendants of the deleted workspace are not navigation targets', () => {
  const repos = repoWith([
    { id: 'base', branch: 'develop', status: 'locked', age: '—' },
    { id: 'parent', branch: 'feat/parent', parentId: 'base', status: 'new', age: '—' },
    { id: 'child', branch: 'feat/child', parentId: 'parent', status: 'new', age: '—' },
  ])
  // Deleting `parent` also deletes `child`; the surviving target is `base`.
  expect(getPostDeleteNavigationTarget(repos, 'parent')).toBe('base')
})

test('locked descendants survive the delete and can be targets', () => {
  const repos = repoWith([
    { id: 'parent', branch: 'feat/parent', status: 'new', age: '—' },
    { id: 'locked-child', branch: 'develop', parentId: 'parent', status: 'locked', age: '—' },
  ])
  expect(getPostDeleteNavigationTarget(repos, 'parent')).toBe('locked-child')
})

test('returns null when no workspace in the repo survives', () => {
  const repos = repoWith([{ id: 'only', branch: 'feat/only', status: 'new', age: '—' }])
  expect(getPostDeleteNavigationTarget(repos, 'only')).toBeNull()
})

test('returns null for an unknown workspace id', () => {
  expect(getPostDeleteNavigationTarget(repoWith([]), 'ghost')).toBeNull()
})
