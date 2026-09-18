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
    expect(state().renamingRole).toBeNull()
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
    expect(state().renamingRole).toBeNull()
  })

  // The id-collision resolution `sidebar-row.tsx`'s `inlineRenameDisabled`
  // doc describes: `startRenaming` checks the DOM, once, for a tree-rendered
  // (non-Recents) instance of the row.
  describe('renamingRole', () => {
    test('resolves to "tree" when a non-Recents treeitem for the id is mounted', () => {
      const el = document.createElement('div')
      el.setAttribute('role', 'treeitem')
      el.setAttribute('data-sidebar-row-id', 'row-1')
      document.body.appendChild(el)
      try {
        state().startRenaming('row-1')
        expect(state().renamingRole).toBe('tree')
      } finally {
        el.remove()
      }
    })

    test('resolves to "recents" when the only mounted instance carries data-sidebar-recents-row', () => {
      const el = document.createElement('div')
      el.setAttribute('role', 'treeitem')
      el.setAttribute('data-sidebar-row-id', 'row-1')
      el.setAttribute('data-sidebar-recents-row', '')
      document.body.appendChild(el)
      try {
        state().startRenaming('row-1')
        expect(state().renamingRole).toBe('recents')
      } finally {
        el.remove()
      }
    })

    test('resolves to "recents" when no instance for the id is mounted at all', () => {
      state().startRenaming('row-1')
      expect(state().renamingRole).toBe('recents')
    })

    test('prefers the tree instance when both a tree and a Recents copy are mounted', () => {
      const tree = document.createElement('div')
      tree.setAttribute('role', 'treeitem')
      tree.setAttribute('data-sidebar-row-id', 'row-1')
      const recents = document.createElement('div')
      recents.setAttribute('role', 'treeitem')
      recents.setAttribute('data-sidebar-row-id', 'row-1')
      recents.setAttribute('data-sidebar-recents-row', '')
      document.body.append(tree, recents)
      try {
        state().startRenaming('row-1')
        expect(state().renamingRole).toBe('tree')
      } finally {
        tree.remove()
        recents.remove()
      }
    })
  })
})
