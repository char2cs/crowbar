import { expect, test } from 'vitest'
import { getAllMockProjects, createMockProject, getMockProject } from '@/lib/mock/projects'

test('returns two initial projects', () => {
  expect(getAllMockProjects()).toHaveLength(2)
})

test('createMockProject adds project to store', () => {
  const before = getAllMockProjects().length
  createMockProject({ name: 'test-proj', path: '/tmp/test-proj' })
  expect(getAllMockProjects()).toHaveLength(before + 1)
})

test('getMockProject returns created project', () => {
  const proj = createMockProject({ name: 'lookup', path: '/tmp/lookup' })
  expect(getMockProject(proj.id)?.name).toBe('lookup')
})
