import { describe, expect, it } from 'vitest'
import { projectNameToHue } from '@/components/layout/sidebar-project-header'

describe('projectNameToHue', () => {
  it('returns 0 for an empty string', () => {
    expect(projectNameToHue('')).toBe(0)
  })

  it('returns a value in [0, 359] for any string', () => {
    for (const name of ['crowbar', 'a', 'quiver.desktop', 'Z', '123', 'hello world']) {
      const h = projectNameToHue(name)
      expect(h).toBeGreaterThanOrEqual(0)
      expect(h).toBeLessThan(360)
    }
  })

  it('is deterministic — same input always returns same output', () => {
    expect(projectNameToHue('crowbar')).toBe(projectNameToHue('crowbar'))
    expect(projectNameToHue('quiver')).toBe(projectNameToHue('quiver'))
  })

  it('produces different hues for different project names', () => {
    const hues = ['crowbar', 'quiver', 'rabbyte', 'alpha'].map(projectNameToHue)
    const unique = new Set(hues)
    expect(unique.size).toBe(hues.length)
  })
})
