import { beforeEach, expect, test } from 'vitest'
import { useSidebarStore } from '@/lib/store/sidebar'

beforeEach(() => {
  useSidebarStore.setState(useSidebarStore.getInitialState())
})

test('addWorkspace appends to the correct repo', () => {
  useSidebarStore.getState().addWorkspace('crowbar', 'ws-new', 'feature/test')
  const repo = useSidebarStore.getState().repos.find(r => r.id === 'crowbar')!
  expect(repo.workspaces.some(w => w.id === 'ws-new')).toBe(true)
})

test('addWorkspace does not affect other repos', () => {
  useSidebarStore.getState().addWorkspace('crowbar', 'ws-new', 'feature/test')
  const other = useSidebarStore.getState().repos.find(r => r.id === 'quiver-core')!
  expect(other.workspaces.some(w => w.id === 'ws-new')).toBe(false)
})

test('deleteWorkspace removes from repo', () => {
  useSidebarStore.getState().deleteWorkspace('ws3')
  const repo = useSidebarStore.getState().repos.find(r => r.id === 'crowbar')!
  expect(repo.workspaces.some(w => w.id === 'ws3')).toBe(false)
})

test('addChat appends a new chat entry', () => {
  useSidebarStore.getState().addChat({ id: 'c-test', title: 'New', age: 'just now' })
  const chats = useSidebarStore.getState().chats
  expect(chats.some(c => c.id === 'c-test')).toBe(true)
})

test('toggleRepo flips collapsed state', () => {
  useSidebarStore.getState().toggleRepo('crowbar')
  expect(useSidebarStore.getState().collapsedRepos.has('crowbar')).toBe(true)
  useSidebarStore.getState().toggleRepo('crowbar')
  expect(useSidebarStore.getState().collapsedRepos.has('crowbar')).toBe(false)
})
