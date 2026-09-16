import { beforeEach, describe, expect, it } from 'vitest'
import { useFileTreeStore } from '@/features/file-explorer/file-explorer/stores/file-explorer-tree-store'

// Live-reported: "folders that were opened get lost when switching contexts."
// This state used to be one flat Set shared by every workspace, so switching
// which workspace was active either borrowed another workspace's unrelated
// path strings (looks collapsed) or had its own expansion silently
// overwritten by another workspace's toggles. These pin the per-workspace
// scoping that fixed it.
describe('useFileTreeStore expanded-path scoping', () => {
  beforeEach(() => {
    useFileTreeStore.setState({ expandedPathsByWorkspace: {}, selectedFiles: new Set() })
  })

  it('expanding a folder in one workspace does not expand the same path in another', () => {
    useFileTreeStore.getState().toggleFolder('ws-a', 'src')

    expect(useFileTreeStore.getState().isExpanded('ws-a', 'src')).toBe(true)
    expect(useFileTreeStore.getState().isExpanded('ws-b', 'src')).toBe(false)
  })

  it('collapsing in one workspace does not touch another workspace toggled the same path', () => {
    useFileTreeStore.getState().toggleFolder('ws-a', 'src')
    useFileTreeStore.getState().toggleFolder('ws-b', 'src')

    useFileTreeStore.getState().toggleFolder('ws-a', 'src') // collapse A

    expect(useFileTreeStore.getState().isExpanded('ws-a', 'src')).toBe(false)
    expect(useFileTreeStore.getState().isExpanded('ws-b', 'src')).toBe(true)
  })

  it('collapseAll only clears the named workspace', () => {
    useFileTreeStore.getState().toggleFolder('ws-a', 'src')
    useFileTreeStore.getState().toggleFolder('ws-b', 'src')

    useFileTreeStore.getState().collapseAll('ws-a')

    expect(useFileTreeStore.getState().isExpanded('ws-a', 'src')).toBe(false)
    expect(useFileTreeStore.getState().isExpanded('ws-b', 'src')).toBe(true)
  })

  it('collapsePath only prunes the named workspace subtree', () => {
    useFileTreeStore.getState().toggleFolder('ws-a', 'src/lib')
    useFileTreeStore.getState().toggleFolder('ws-b', 'src/lib')

    useFileTreeStore.getState().collapsePath('ws-a', 'src')

    expect(useFileTreeStore.getState().isExpanded('ws-a', 'src/lib')).toBe(false)
    expect(useFileTreeStore.getState().isExpanded('ws-b', 'src/lib')).toBe(true)
  })

  it('expandAll only expands the named workspace tree', () => {
    const files = [{ name: 'src', path: 'src', isDir: true, children: [] }]
    useFileTreeStore.getState().expandAll('ws-a', files)

    expect(useFileTreeStore.getState().isExpanded('ws-a', 'src')).toBe(true)
    expect(useFileTreeStore.getState().isExpanded('ws-b', 'src')).toBe(false)
  })

  it('setExpandedPaths/getExpandedPaths round-trip per workspace', () => {
    useFileTreeStore.getState().setExpandedPaths('ws-a', new Set(['src', 'src/lib']))

    expect([...useFileTreeStore.getState().getExpandedPaths('ws-a')].sort()).toEqual([
      'src',
      'src/lib',
    ])
    expect(useFileTreeStore.getState().getExpandedPaths('ws-b').size).toBe(0)
  })

  it('getExpandedPaths returns a stable empty reference for an untouched workspace', () => {
    // Referential stability matters: a selector returning a fresh Set every
    // call would re-trigger every effect/memo keyed on it every render.
    const first = useFileTreeStore.getState().getExpandedPaths('ws-never-touched')
    const second = useFileTreeStore.getState().getExpandedPaths('ws-never-touched')
    expect(first).toBe(second)
  })

  it('expandToPath expands only the named workspace ancestors', () => {
    useFileTreeStore.getState().expandToPath('ws-a', 'src/lib/util.ts')

    expect(useFileTreeStore.getState().isExpanded('ws-a', 'src')).toBe(true)
    expect(useFileTreeStore.getState().isExpanded('ws-a', 'src/lib')).toBe(true)
    expect(useFileTreeStore.getState().isExpanded('ws-b', 'src')).toBe(false)
  })
})
