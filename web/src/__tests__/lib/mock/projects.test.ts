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

test('two rapid createMockProject calls produce unique IDs', () => {
  const p1 = createMockProject({ name: 'a', path: '/a' })
  const p2 = createMockProject({ name: 'b', path: '/b' })
  expect(p1.id).not.toBe(p2.id)
})
