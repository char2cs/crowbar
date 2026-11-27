import { beforeEach, describe, expect, it } from 'vitest'
import { loadDockPrefs, saveDockPrefs, nextDock } from '@/features/console/lib/dock-prefs'

beforeEach(() => localStorage.clear())

describe('dock prefs', () => {
  it('falls back to a top overlay at the default size', () => {
    expect(loadDockPrefs()).toEqual({ dock: 'top', mode: 'overlay', size: 420 })
  })

  it('round-trips what was saved', () => {
    saveDockPrefs({ dock: 'left', mode: 'push', size: 333 })
    expect(loadDockPrefs()).toEqual({ dock: 'left', mode: 'push', size: 333 })
  })

  it('ignores stored garbage field by field', () => {
    localStorage.setItem('console-dock', 'middle')
    localStorage.setItem('console-mode', 'float')
    localStorage.setItem('console-size', 'wide')
    expect(loadDockPrefs()).toEqual({ dock: 'top', mode: 'overlay', size: 420 })
  })

  it('clamps a stored size that is too small', () => {
    localStorage.setItem('console-size', '3')
    expect(loadDockPrefs().size).toBe(160)
  })

  it('cycles top, right, bottom, left', () => {
    expect(nextDock('top')).toBe('right')
    expect(nextDock('right')).toBe('bottom')
    expect(nextDock('bottom')).toBe('left')
    expect(nextDock('left')).toBe('top')
  })
})
