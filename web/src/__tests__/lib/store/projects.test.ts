import { beforeEach, expect, test } from 'vitest'
import { useProjectStore } from '@/lib/store/projects'

beforeEach(() => {
  useProjectStore.setState(useProjectStore.getInitialState())
})

test('active project defaults to rabbyte', () => {
  expect(useProjectStore.getState().activeProjectId).toBe('rabbyte')
})

test('setActiveProject changes active project', () => {
  useProjectStore.getState().setActiveProject('personal')
  expect(useProjectStore.getState().activeProjectId).toBe('personal')
})

test('addProject appends a project', () => {
  const before = useProjectStore.getState().projects.length
  useProjectStore.getState().addProject({ id: 'x', name: 'X', path: '/tmp/x', lastActivity: new Date() })
  expect(useProjectStore.getState().projects).toHaveLength(before + 1)
})
