import { test, expect } from 'vitest'
import { createMockWorkspace, getMockWorkspace } from '@/lib/mock/workspaces'

test('two rapid createMockWorkspace calls produce unique IDs', () => {
  const ws1 = createMockWorkspace('crowbar', 'feature/a')
  const ws2 = createMockWorkspace('crowbar', 'feature/b')
  expect(ws1.id).not.toBe(ws2.id)
})

test('createMockWorkspace is retrievable by the generated ID', () => {
  const ws = createMockWorkspace('quiver-core', 'main')
  expect(getMockWorkspace(ws.id)).toEqual(ws)
})
