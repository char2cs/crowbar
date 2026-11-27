import { describe, it, expect, beforeEach, vi } from 'vitest'
import { loadFromLocalStorage } from '@/features/workspace/stores/workspace-persistence'

describe('workspace-persistence', () => {
  beforeEach(() => {
    localStorage.clear()
    vi.restoreAllMocks()
  })

  it('loadFromLocalStorage returns the saved snapshot', () => {
    localStorage.setItem('workspace:ws-2:state', JSON.stringify({ recentFiles: [] }))
    const snap = loadFromLocalStorage('ws-2')
    expect(snap?.recentFiles).toEqual([])
  })

  it('loadFromLocalStorage returns null for unknown wsId', () => {
    expect(loadFromLocalStorage('ws-unknown')).toBeNull()
  })

  it('loadFromLocalStorage returns null for corrupt data', () => {
    localStorage.setItem('workspace:ws-corrupt:state', '{{not json}}')
    expect(loadFromLocalStorage('ws-corrupt')).toBeNull()
  })
})
