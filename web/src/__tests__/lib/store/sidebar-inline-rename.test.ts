import { describe, expect, test, beforeEach } from 'vitest'
import {
  getInitialInlineRenameState,
  useSidebarInlineRenameStore,
} from '@/lib/store/sidebar-inline-rename'

const state = () => useSidebarInlineRenameStore.getState()

beforeEach(() => {
  useSidebarInlineRenameStore.setState(getInitialInlineRenameState())
})

describe('useSidebarInlineRenameStore', () => {
  test('starts with nothing renaming', () => {
    expect(state().renamingRowId).toBeNull()
  })

  test('startRenaming sets the row id', () => {
    state().startRenaming('row-1')
    expect(state().renamingRowId).toBe('row-1')
  })

  test('startRenaming a second row replaces the first — only one at a time', () => {
    state().startRenaming('row-1')
    state().startRenaming('row-2')
    expect(state().renamingRowId).toBe('row-2')
  })

  test('stopRenaming clears it', () => {
    state().startRenaming('row-1')
    state().stopRenaming()
    expect(state().renamingRowId).toBeNull()
  })
})
