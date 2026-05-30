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

test('addWorkspace stores parentId when provided', () => {
  useSidebarStore.getState().addWorkspace('crowbar', 'ws-child', 'feature/child', 'ws-develop')
  const ws = useSidebarStore.getState().repos
    .find(r => r.id === 'crowbar')!.workspaces
    .find(w => w.id === 'ws-child')!
  expect(ws.parentId).toBe('ws-develop')
})

test('addWorkspace stores no parentId when omitted', () => {
  useSidebarStore.getState().addWorkspace('crowbar', 'ws-root', 'feature/root')
  const ws = useSidebarStore.getState().repos
    .find(r => r.id === 'crowbar')!.workspaces
    .find(w => w.id === 'ws-root')!
  expect(ws.parentId).toBeUndefined()
})

test('renameWorkspace updates branch on matching workspace', () => {
  useSidebarStore.getState().renameWorkspace('ws3', 'feature/renamed')
  const ws = useSidebarStore.getState().repos
    .flatMap(r => r.workspaces).find(w => w.id === 'ws3')!
  expect(ws.branch).toBe('feature/renamed')
})

test('renameWorkspace leaves other workspaces unchanged', () => {
  useSidebarStore.getState().renameWorkspace('ws3', 'feature/renamed')
  const ws = useSidebarStore.getState().repos
    .flatMap(r => r.workspaces).find(w => w.id === 'ws1')!
  expect(ws.branch).toBe('enhancement/scaffold')
})

test('reparentWorkspace changes parentId', () => {
  useSidebarStore.getState().reparentWorkspace('ws2', 'ws3')
  const ws = useSidebarStore.getState().repos
    .flatMap(r => r.workspaces).find(w => w.id === 'ws2')!
  expect(ws.parentId).toBe('ws3')
})

test('reparentWorkspace to undefined makes workspace a repo root', () => {
  useSidebarStore.getState().reparentWorkspace('ws3', undefined)
  const ws = useSidebarStore.getState().repos
    .flatMap(r => r.workspaces).find(w => w.id === 'ws3')!
  expect(ws.parentId).toBeUndefined()
})

test('reparentWorkspace rejects cycles: descendant cannot become ancestor', () => {
  // ws3 is a child of ws-develop; making ws-develop a child of ws3 would cycle
  useSidebarStore.getState().reparentWorkspace('ws-develop', 'ws3')
  const ws = useSidebarStore.getState().repos
    .flatMap(r => r.workspaces).find(w => w.id === 'ws-develop')!
  expect(ws.parentId).toBeUndefined() // unchanged
})

test('reparentWorkspace rejects cross-repo moves', () => {
  // qc1 is in quiver-core; ws3 is in crowbar
  useSidebarStore.getState().reparentWorkspace('ws3', 'qc1')
  const ws = useSidebarStore.getState().repos
    .flatMap(r => r.workspaces).find(w => w.id === 'ws3')!
  expect(ws.parentId).toBe('ws-develop') // unchanged
})
