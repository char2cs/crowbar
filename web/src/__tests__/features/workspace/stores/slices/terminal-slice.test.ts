import { describe, it, expect, beforeEach } from 'vitest'
import { createStore } from 'zustand'
import { immer } from 'zustand/middleware/immer'
import {
  createTerminalSlice,
  type TerminalSlice,
} from '@/features/workspace/stores/slices/terminal-slice'

function makeStore() {
  return createStore<TerminalSlice>()(
    immer((set, get) => createTerminalSlice(set as any, get as any, {} as any)),
  )
}

describe('terminal-slice', () => {
  let store: ReturnType<typeof makeStore>
  beforeEach(() => {
    store = makeStore()
  })

  it('starts with no sessions and default layout', () => {
    expect(store.getState().terminalSessionIds.size).toBe(0)
    expect(store.getState().terminalLayout.widthMode).toBe('full')
    expect(store.getState().terminalLayout.tabLayout).toBe('horizontal')
  })

  it('registerSession adds the session ID', () => {
    store.getState().terminalActions.registerSession('sess-1')
    expect(store.getState().terminalActions.hasSession('sess-1')).toBe(true)
  })

  it('unregisterSession removes the session ID', () => {
    store.getState().terminalActions.registerSession('sess-1')
    store.getState().terminalActions.unregisterSession('sess-1')
    expect(store.getState().terminalActions.hasSession('sess-1')).toBe(false)
  })

  it('hasSession returns false for unknown sessions', () => {
    expect(store.getState().terminalActions.hasSession('not-here')).toBe(false)
  })

  it('setWidthMode updates the layout', () => {
    store.getState().terminalActions.setWidthMode('editor')
    expect(store.getState().terminalLayout.widthMode).toBe('editor')
  })
})
