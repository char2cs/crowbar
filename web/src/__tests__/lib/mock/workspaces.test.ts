import { test, expect } from 'vitest'
import { createMockWorkspace, getMockWorkspace } from '@/lib/mock/workspaces'

test('two rapid createMockWorkspace calls produce unique IDs', () => {
  const ws1 = createMockWorkspace('crowbar', 'feature/a', 'feature-development')
  const ws2 = createMockWorkspace('crowbar', 'feature/b', 'feature-development')
  expect(ws1.id).not.toBe(ws2.id)
})

test('createMockWorkspace is retrievable by the generated ID', () => {
  const ws = createMockWorkspace('quiver-core', 'main', 'feature-development')
  expect(getMockWorkspace(ws.id)).toEqual(ws)
})
