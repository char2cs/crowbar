import { expect, test } from 'vitest'
import { shouldRedirectUnknownWorkspace } from '@/lib/store/workspace-route-guard'
import type { Repo } from '@/lib/store/sidebar'

const REPOS: Repo[] = [
  {
    id: 'repo-1',
    name: 'crowbar',
    avatarLabel: 'C',
    avatarColor: 'bg-indigo-700',
    defaultWorkspaceId: 'ws-default',
    workspaces: [
      { id: 'ws-base', branch: 'develop', status: 'locked', age: '—' },
      { id: 'ws-child', branch: 'feature/x', parentId: 'ws-base', status: 'new', age: '—' },
    ],
  },
]

test('does not redirect while the list is still loading', () => {
  expect(shouldRedirectUnknownWorkspace('idle', [], 'ghost')).toBe(false)
  expect(shouldRedirectUnknownWorkspace('loading', [], 'ghost')).toBe(false)
})

test('does not redirect on fetch error (transient backend failure)', () => {
  expect(shouldRedirectUnknownWorkspace('error', REPOS, 'ghost')).toBe(false)
})

test('does not redirect when the workspace exists', () => {
  expect(shouldRedirectUnknownWorkspace('success', REPOS, 'ws-base')).toBe(false)
  expect(shouldRedirectUnknownWorkspace('success', REPOS, 'ws-child')).toBe(false)
})

test('redirects when the list is loaded and the id is unknown', () => {
  expect(shouldRedirectUnknownWorkspace('success', REPOS, 'ghost')).toBe(true)
  expect(shouldRedirectUnknownWorkspace('success', [], 'ghost')).toBe(true)
})

test('no id means nothing to guard', () => {
  expect(shouldRedirectUnknownWorkspace('success', REPOS, undefined)).toBe(false)
})

test('does not redirect for the repo default workspace id (excluded from the tree)', () => {
  expect(shouldRedirectUnknownWorkspace('success', REPOS, 'ws-default')).toBe(false)
})
