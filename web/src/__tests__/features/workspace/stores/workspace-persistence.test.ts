import { describe, it, expect, beforeEach, vi } from 'vitest'
import { saveToLocalStorage, loadFromLocalStorage } from '@/features/workspace/stores/workspace-persistence'

describe('workspace-persistence', () => {
  beforeEach(() => {
    localStorage.clear()
    vi.restoreAllMocks()
  })

  it('saveToLocalStorage writes a JSON string to localStorage', () => {
    saveToLocalStorage('ws-1', { recentFiles: [] })
    const raw = localStorage.getItem('workspace:ws-1:state')
    expect(raw).not.toBeNull()
    expect(JSON.parse(raw!).recentFiles).toEqual([])
  })

  it('loadFromLocalStorage returns the saved snapshot', () => {
    saveToLocalStorage('ws-2', { recentFiles: [] })
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
