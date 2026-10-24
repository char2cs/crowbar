import { describe, it, expect } from 'vitest'
import {
  splitLegacyEditorContent,
  upgradeLegacyBuffers,
  validateLoadedBuffers,
  type Snapshot,
} from '@/features/panes/utils/persisted-layout'
import type { PaneContent } from '@/features/panes/types/pane-content'
import { documentUri } from '@/features/editor/documents/document-ref'
import { workspaceRef } from '@/lib/api/documents'
import type { WorkspaceLayout } from '@/lib/persistence/schemas'

describe('validateLoadedBuffers', () => {
  const ROOT = { adoptInto: 'root' }
  const snapshot = {
    buffers: [
      { id: 'nt-1', type: 'newTab', path: '', name: 'New Tab' },
      { id: 'e-1', type: 'editor', path: '/a.ts', name: 'a.ts' },
    ],
    panes: {
      root: { id: 'root', editorTabIds: ['nt-1', 'e-1'], activeEditorTabId: 'nt-1' },
      split: { id: 'split', editorTabIds: ['nt-2'], activeEditorTabId: 'nt-2' },
    },
  } as unknown as Snapshot

  it('drops buffers whose content type no longer exists (e.g. the retired New Tab placeholder)', () => {
    expect(validateLoadedBuffers(snapshot, ROOT).buffers.map((b) => b.id)).toEqual(['e-1'])
  })

  it('strips their ids out of pane membership, so no id is left stranded', () => {
    expect(validateLoadedBuffers(snapshot, ROOT).panes.root.editorTabIds).toEqual(['e-1'])
  })

  it('repoints activeEditorTabId when it pointed at a dropped buffer', () => {
    const out = validateLoadedBuffers(snapshot, ROOT)
    expect(out.panes.root.activeEditorTabId).toBe('e-1')
    // Nothing left to activate — null, never a dangling id.
    expect(out.panes.split.activeEditorTabId).toBeNull()
  })

  it("drops a buffer that is a pane's only tab, leaving it empty", () => {
    const soleUnknown = {
      buffers: [{ id: 'nt-only', type: 'newTab', path: '', name: 'New Tab' }],
      panes: {
        root: { id: 'root', editorTabIds: ['nt-only'], activeEditorTabId: 'nt-only' },
      },
    } as unknown as Snapshot

    const out = validateLoadedBuffers(soleUnknown, ROOT)
    expect(out.buffers.map((b) => b.id)).toEqual([])
    expect(out.panes.root.editorTabIds).toEqual([])
    expect(out.panes.root.activeEditorTabId).toBeNull()
  })

  it('heals a pre-existing stranded id even when nothing was dropped (Finding 1)', () => {
    // No unknown-type buffers at all — the guard must still fire the heal,
    // not bail out early, since a pane holds an id with no backing buffer.
    const strandedOnly = {
      buffers: [{ id: 'e-1', type: 'editor', path: '/a.ts', name: 'a.ts' }],
      panes: {
        root: { id: 'root', editorTabIds: ['e-1', 'ghost'], activeEditorTabId: 'ghost' },
      },
    } as unknown as Snapshot

    const out = validateLoadedBuffers(strandedOnly, ROOT)
    expect(out.panes.root.editorTabIds).toEqual(['e-1'])
    expect(out.panes.root.activeEditorTabId).toBe('e-1')
  })

  it('drops a buffer no pane lists — a leak from before C2 held is not restored', () => {
    const leaked = {
      buffers: [
        { id: 'e-1', type: 'editor', path: '/a.ts', name: 'a.ts' },
        { id: 't-1', type: 'terminal', sessionId: 's', name: 'Terminal 1' },
      ],
      panes: {
        root: { id: 'root', editorTabIds: ['e-1'], activeEditorTabId: 'e-1' },
      },
    } as unknown as Snapshot

    expect(validateLoadedBuffers(leaked, ROOT).buffers.map((b) => b.id)).toEqual(['e-1'])
  })

  it('re-homes an unsaved buffer no pane lists instead of dropping it', () => {
    const leaked = {
      buffers: [
        { id: 'e-1', type: 'editor', path: '/a.ts', name: 'a.ts' },
        { id: 'e-2', type: 'editor', path: '/b.ts', isDirty: true },
        { id: 'e-3', type: 'editor', path: '/c.ts', isDirty: true },
      ],
      panes: {
        root: { id: 'root', editorTabIds: [], activeEditorTabId: null },
        other: { id: 'other', editorTabIds: ['e-1'], activeEditorTabId: 'e-1' },
      },
    } as unknown as Snapshot

    const out = validateLoadedBuffers(leaked, ROOT)

    expect(out.buffers.map((b) => b.id)).toEqual(['e-1', 'e-2', 'e-3'])
    expect(out.panes.root.editorTabIds).toEqual(['e-2', 'e-3'])
    expect(out.panes.root.activeEditorTabId).toBe('e-2')
  })

  it('keeps every unlisted buffer when asked to (a layout saved without views)', () => {
    const unlisted = {
      buffers: [{ id: 'e-1', type: 'editor', path: '/a.ts', name: 'a.ts' }],
      panes: { root: { id: 'root', editorTabIds: [], activeEditorTabId: null } },
    } as unknown as Snapshot

    const out = validateLoadedBuffers(unlisted, { adoptInto: 'root', keepUnlisted: true })

    expect(out.buffers.map((b) => b.id)).toEqual(['e-1'])
    expect(out.panes.root.editorTabIds).toEqual(['e-1'])
  })

  it('drops a buffer whose content type this build no longer has', () => {
    // A saved layout outlives the code that wrote it. `diff` was the old Monaco
    // commit-diff tab; a layout written before it was retired restores into a
    // renderer that is gone, and the tab paints blank with nothing to say it is
    // stale rather than broken. Graceful fallback, deliberately not migration.
    const stale = {
      buffers: [
        { id: 'old-1', type: 'diff', path: 'diff://commit/abc/all-files', name: 'Commit abc' },
        { id: 'e-1', type: 'editor', path: '/a.ts', name: 'a.ts' },
      ],
      panes: {
        root: { id: 'root', editorTabIds: ['old-1', 'e-1'], activeEditorTabId: 'old-1' },
      },
    } as unknown as Snapshot

    const out = validateLoadedBuffers(stale, ROOT)

    expect(out.buffers.map((b) => b.id)).toEqual(['e-1'])
    // The pane must not be left pointing at the id that just vanished.
    expect(out.panes.root.editorTabIds).toEqual(['e-1'])
    expect(out.panes.root.activeEditorTabId).toBe('e-1')
  })

  it('keeps every type this build DOES know, including the new commit diff', () => {
    const current = {
      buffers: [
        { id: 'c-1', type: 'commitDiff', wsId: 'w1', sha: 'abc1234', name: 'Commit abc1234' },
        { id: 'br-1', type: 'branchReview', wsId: 'w1', name: 'Branch Review' },
      ],
      panes: {
        root: { id: 'root', editorTabIds: ['c-1', 'br-1'], activeEditorTabId: 'c-1' },
      },
    } as unknown as Snapshot

    expect(validateLoadedBuffers(current, ROOT)).toEqual(current)
  })
})

describe('upgradeLegacyBuffers', () => {
  const legacy = (buffers: object[]) => upgradeLegacyBuffers(buffers as unknown as PaneContent[])

  it('loads an editor buffer saved before documents had an authority as a workspace one', () => {
    const [buffer] = legacy([{ id: 'e-1', type: 'editor', path: 'src/a.ts', workspaceId: 'w1' }])
    expect(buffer).toMatchObject({ authority: 'workspace', path: 'src/a.ts', workspaceId: 'w1' })
  })

  it('names a legacy preview’s source as the document of its own workspace', () => {
    const [md, html] = legacy([
      { id: 'm', type: 'markdownPreview', sourceFilePath: 'README.md', workspaceId: 'w1' },
      { id: 'h', type: 'htmlPreview', sourceFilePath: 'site/index.html', workspaceId: 'w2' },
    ])
    expect(md).toMatchObject({
      source: { authority: 'workspace', workspaceId: 'w1', path: 'README.md' },
    })
    expect(html).toMatchObject({
      source: { authority: 'workspace', workspaceId: 'w2', path: 'site/index.html' },
    })
  })

  it('leaves a current buffer as it is', () => {
    const host = { id: 'e-2', type: 'editor', authority: 'host', path: '/a.ts', workspaceId: '' }
    expect(legacy([host])).toEqual([host])
  })
})

describe('splitLegacyEditorContent', () => {
  const legacyLayout = (buffer: object) =>
    ({
      workspaceId: 'window',
      panes: {},
      buffers: [buffer],
      updatedAt: 7,
    }) as unknown as WorkspaceLayout
  const legacyBuffer = { id: 'e-1', type: 'editor', authority: 'workspace', workspaceId: 'w1' }

  it('moves legacy unsaved buffer text into drafts', () => {
    const { layout, drafts } = splitLegacyEditorContent(
      legacyLayout({
        ...legacyBuffer,
        path: 'a.ts',
        content: 'a',
        savedContent: 'b',
        tokens: [],
        fileMissing: true,
      }),
    )

    expect(drafts).toEqual([
      { uri: documentUri(workspaceRef('w1', 'a.ts')), text: 'a', savedText: 'b', updatedAt: 7 },
    ])
    expect(layout.buffers[0]).toMatchObject({ id: 'e-1', isDirty: true })
    for (const field of ['content', 'savedContent', 'tokens', 'fileMissing']) {
      expect(layout.buffers[0]).not.toHaveProperty(field)
    }
  })

  it('drops clean legacy buffer text without a draft', () => {
    const { layout, drafts } = splitLegacyEditorContent(
      legacyLayout({ ...legacyBuffer, path: 'a.ts', content: 'same', savedContent: 'same' }),
    )

    expect(drafts).toEqual([])
    expect(layout.buffers[0]).toEqual({ ...legacyBuffer, path: 'a.ts' })
  })
})
