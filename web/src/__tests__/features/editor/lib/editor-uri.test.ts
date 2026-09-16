import { describe, expect, it } from 'vitest'
import { fileUri, uriToFsPath } from '@/features/editor/lib/editor-uri'

describe('editor-uri', () => {
  it('builds a stable crowbar uri from a workspace + file path (keyed by file, not buffer)', () => {
    const a = fileUri('ws-1', '/repo/src/index.ts')
    const b = fileUri('ws-1', '/repo/src/index.ts')
    expect(a).toBe(b)
    expect(a).toMatch(/^crowbar:\/\/editor\//)
  })

  it('round-trips back to the fs path', () => {
    expect(uriToFsPath(fileUri('ws-1', '/repo/a b/c.ts'))).toBe('/repo/a b/c.ts')
  })

  it('distinguishes different files in the same workspace', () => {
    expect(fileUri('ws-1', '/a.ts')).not.toBe(fileUri('ws-1', '/b.ts'))
  })

  // Regression: Monaco models are one GLOBAL table (monaco.editor.getModel/
  // createModel), not scoped per ModelRegistry instance. Keying the uri by
  // path alone made two DIFFERENT workspaces' files at the same relative
  // path (two branches of the same repo, each its own worktree) collide on
  // one shared model — one workspace's edits, close, or disposal landed on
  // (or yanked out from under) the other's pane. Live-reported as syntax
  // highlighting flashing across unrelated panes and intermittent "Model is
  // disposed!" errors with no app-level stack.
  it('distinguishes the SAME relative path in two different workspaces', () => {
    const a = fileUri('workspace-a', 'src/inventory.ts')
    const b = fileUri('workspace-b', 'src/inventory.ts')
    expect(a).not.toBe(b)
    expect(uriToFsPath(a)).toBe('src/inventory.ts')
    expect(uriToFsPath(b)).toBe('src/inventory.ts')
  })

  it('round-trips correctly even when the workspace id and path both need encoding', () => {
    const uri = fileUri('ws with space/slash', '/repo/a b/c d.ts')
    expect(uriToFsPath(uri)).toBe('/repo/a b/c d.ts')
  })
})
