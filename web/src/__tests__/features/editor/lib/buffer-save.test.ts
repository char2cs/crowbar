import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

// Only apiFetch (the real HTTP boundary every document write ends at) needs
// mocking: the body names the workspace a save lands in.
vi.mock('@/lib/api', async () => {
  const actual = await vi.importActual<typeof import('@/lib/api')>('@/lib/api')
  return { ...actual, apiFetch: vi.fn() }
})

// The active workspace is 'ws-active', never the buffers' own 'ws-owner': a save
// resolving its workspace through the active one would visibly write there.
vi.mock('@/features/workspace/stores/workspace-store-registry', async (importOriginal) => {
  const actual =
    await importOriginal<typeof import('@/features/workspace/stores/workspace-store-registry')>()
  return { ...actual, getActiveWorkspaceId: () => 'ws-active' }
})

const { formatDocumentWithLsp } = vi.hoisted(() => ({ formatDocumentWithLsp: vi.fn() }))
vi.mock('@/features/editor/lsp/format-buffer', () => ({ formatDocumentWithLsp }))

import { apiFetch } from '@/lib/api'
import {
  AUTOSAVE_DELAY_MS,
  saveActiveBuffer,
  saveAllDirtyBuffers,
  saveBuffer,
} from '@/features/editor/lib/buffer-save'
import {
  documentText,
  registerPendingWriter,
  replaceDocumentText,
  savedDocumentText,
} from '@/features/editor/documents/document-text'
import { useSettingsStore } from '@/features/settings/store'
import { BOTTOM_PANE_ID, ROOT_PANE_ID } from '@/features/panes/constants/pane'
import { toast } from '@/features/window/stores/toast-store'
import {
  windowPaneStore,
  resetWindowPaneStoreForTests,
} from '@/features/panes/stores/window-pane-store'
import type { EditorContent } from '@/features/panes/types/pane-content'
import { workspaceRef, type DocumentRef } from '@/lib/api/documents'
import { onGitRefreshRequested } from '@/features/git/stores/git-refresh'
import { blameKey, useGitBlameStore } from '@/features/git/stores/git-blame-store'
import { success } from '@/lib/loadable'
import { useFileWatcherStore } from '@/features/file-system/controllers/file-watcher-store'

const mockFetch = apiFetch as ReturnType<typeof vi.fn>

/** A buffer of `workspaceId`, forced dirty: openContent always opens one clean. */
function openDirtyBuffer(workspaceId: string | DocumentRef, path: string, content: string): string {
  const doc = typeof workspaceId === 'string' ? workspaceRef(workspaceId, path) : workspaceId
  const id = windowPaneStore.getState().bufferActions.openContent({
    type: 'editor',
    doc,
    name: path,
    content,
  })
  windowPaneStore.setState((s) => {
    s.buffers = s.buffers.map((b) =>
      b.id === id && b.type === 'editor' ? { ...b, isDirty: true } : b,
    )
    return s
  })
  return id
}

function bufferById(id: string): EditorContent {
  return windowPaneStore.getState().buffers.find((b) => b.id === id) as EditorContent
}

beforeEach(() => {
  vi.clearAllMocks()
  mockFetch.mockResolvedValue({})
  resetWindowPaneStoreForTests()
})

// A dirty buffer of ws-owner while ws-active is on screen: the write must land in ws-owner.
describe("buffer-save — Critical 1: saves target the BUFFER's own workspace, not the active one", () => {
  it("handleSave writes to the dirty buffer's own workspace", async () => {
    openDirtyBuffer('ws-owner', 'a.ts', 'edited content')

    await saveActiveBuffer()

    expect(mockFetch).toHaveBeenCalledTimes(1)
    const [url, init] = mockFetch.mock.calls[0] as [string, RequestInit]
    expect(url).toBe('/v0/documents/content')
    expect(JSON.parse(init.body as string)).toMatchObject({
      workspaceId: 'ws-owner',
      path: 'a.ts',
      content: 'edited content',
    })
  })

  it("saveActiveBuffer (Save As, an untitled buffer) writes to the buffer's own workspace", async () => {
    const id = windowPaneStore.getState().bufferActions.openContent({
      type: 'editor',
      doc: workspaceRef('ws-owner', 'untitled:Untitled-1'),
      name: 'Untitled-1',
      content: 'new file content',
      isVirtual: true,
    })
    vi.stubGlobal(
      'prompt',
      vi.fn(() => 'new-name.ts'),
    )

    await saveActiveBuffer()

    expect(mockFetch).toHaveBeenCalledTimes(1)
    const [, init] = mockFetch.mock.calls[0] as [string, RequestInit]
    expect(JSON.parse(init.body as string)).toMatchObject({ workspaceId: 'ws-owner' })
    expect(bufferById(id).path).toBe('new-name.ts')
    vi.unstubAllGlobals()
  })

  it('handleSaveAll writes each dirty buffer to ITS OWN workspace, never the active one', async () => {
    openDirtyBuffer('ws-owner', 'a.ts', 'from ws-owner')
    openDirtyBuffer('ws-active', 'b.ts', 'from ws-active')

    const savedCount = await saveAllDirtyBuffers()

    expect(savedCount).toBe(2)
    expect(mockFetch).toHaveBeenCalledTimes(2)
    const calls = mockFetch.mock.calls.map(
      ([, init]) =>
        JSON.parse((init as RequestInit).body as string) as { path: string; workspaceId: string },
    )
    // Each buffer's own write landed in ITS OWN workspace, not cross-wired —
    // the bug this locks in would have sent BOTH writes to whichever workspace
    // was merely active.
    expect(calls.find((c) => c.path === 'a.ts')?.workspaceId).toBe('ws-owner')
    expect(calls.find((c) => c.path === 'b.ts')?.workspaceId).toBe('ws-active')
  })
})

describe('buffer-save — a document outside every workspace', () => {
  it('writes to its own path and touches no workspace', async () => {
    const refreshed = vi.fn()
    const stop = onGitRefreshRequested('', refreshed)
    openDirtyBuffer({ authority: 'host', path: '/tmp/notes.md' }, 'notes.md', 'edited')

    expect(await saveActiveBuffer()).toBe(true)

    expect(mockFetch).toHaveBeenCalledTimes(1)
    const [, init] = mockFetch.mock.calls[0] as [string, RequestInit]
    expect(JSON.parse(init.body as string)).toEqual({ path: '/tmp/notes.md', content: 'edited' })
    expect(refreshed).not.toHaveBeenCalled()
    expect(useFileWatcherStore.getState().pendingSaves.has('/tmp/notes.md')).toBe(false)
    stop()
  })
})

function writesFor(path: string): string[] {
  return mockFetch.mock.calls
    .map(
      ([, init]) =>
        JSON.parse((init as RequestInit).body as string) as { path: string; content: string },
    )
    .filter((body) => body.path === path)
    .map((body) => body.content)
}

const A = workspaceRef('ws-owner', 'a.ts')
const B = workspaceRef('ws-owner', 'b.ts')

function textOf(ref: DocumentRef) {
  return { text: documentText(ref), saved: savedDocumentText(ref) }
}

function openCleanBuffer(workspaceId: string, path: string, content: string): string {
  return windowPaneStore.getState().bufferActions.openContent({
    type: 'editor',
    doc: workspaceRef(workspaceId, path),
    name: path,
    content,
  })
}

describe('buffer-save — P0-10: autosave is per buffer and never mislabels edits', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    useSettingsStore.setState((s) => ({ settings: { ...s.settings, autoSave: true } }))
  })
  afterEach(() => {
    vi.useRealTimers()
    useSettingsStore.setState((s) => ({ settings: { ...s.settings, autoSave: false } }))
  })

  it("editing buffer B does not cancel buffer A's pending autosave", async () => {
    const a = openCleanBuffer('ws-owner', 'a.ts', 'a0')
    const b = openCleanBuffer('ws-owner', 'b.ts', 'b0')

    replaceDocumentText(A, 'a1')
    replaceDocumentText(B, 'b1')
    await vi.advanceTimersByTimeAsync(AUTOSAVE_DELAY_MS)

    expect(writesFor('a.ts')).toEqual(['a1'])
    expect(writesFor('b.ts')).toEqual(['b1'])
    expect(bufferById(a).isDirty).toBe(false)
    expect(bufferById(b).isDirty).toBe(false)
  })

  it('an edit that lands while the write is in flight keeps the buffer dirty', async () => {
    const id = openCleanBuffer('ws-owner', 'a.ts', 'v0')
    let finishWrite: () => void = () => {}
    mockFetch.mockImplementationOnce(
      () => new Promise((resolve) => (finishWrite = () => resolve({}))),
    )

    replaceDocumentText(A, 'v1')
    const save = saveBuffer(id)
    await vi.advanceTimersByTimeAsync(0)
    replaceDocumentText(A, 'v2')
    finishWrite()
    await save

    expect(textOf(A)).toEqual({ text: 'v2', saved: 'v1' })
    expect(bufferById(id).isDirty).toBe(true)
    // ...and autosave catches the late edit up.
    await vi.advanceTimersByTimeAsync(AUTOSAVE_DELAY_MS)
    expect(writesFor('a.ts')).toEqual(['v1', 'v2'])
    expect(bufferById(id).isDirty).toBe(false)
  })

  it('a failed write leaves the buffer dirty with its previous saved baseline', async () => {
    const id = openCleanBuffer('ws-owner', 'a.ts', 'v0')
    mockFetch.mockRejectedValueOnce(new Error('disk full'))

    replaceDocumentText(A, 'v1')
    expect(await saveBuffer(id)).toBe(false)

    expect(textOf(A)).toEqual({ text: 'v1', saved: 'v0' })
    expect(bufferById(id).isDirty).toBe(true)
  })

  it('saves of one buffer are serialized, newest content last', async () => {
    const id = openCleanBuffer('ws-owner', 'a.ts', 'v0')
    const resolvers: Array<() => void> = []
    mockFetch.mockImplementation(() => new Promise((r) => resolvers.push(() => r({}))))

    replaceDocumentText(A, 'v1')
    const first = saveBuffer(id)
    await vi.advanceTimersByTimeAsync(0)
    replaceDocumentText(A, 'v2')
    const second = saveBuffer(id)
    await vi.advanceTimersByTimeAsync(0)
    // Only the first write is in flight until it settles.
    expect(resolvers).toHaveLength(1)
    resolvers[0]?.()
    await first
    await vi.advanceTimersByTimeAsync(0)
    resolvers[1]?.()
    await second

    expect(writesFor('a.ts')).toEqual(['v1', 'v2'])
    expect(savedDocumentText(A)).toBe('v2')
    expect(bufferById(id).isDirty).toBe(false)
  })
})

describe('buffer-save — the text a save writes', () => {
  afterEach(() => {
    useSettingsStore.setState((s) => ({ settings: { ...s.settings, formatOnSave: false } }))
  })

  it('writes edits a pending writer has not flushed yet', async () => {
    const id = openCleanBuffer('ws-owner', 'a.ts', 'saved')
    const stop = registerPendingWriter(A, () => replaceDocumentText(A, 'typed, not flushed'))

    expect(await saveBuffer(id)).toBe(true)

    expect(writesFor('a.ts')).toEqual(['typed, not flushed'])
    expect(bufferById(id).isDirty).toBe(false)
    stop()
  })

  it('format on save writes the formatted text and leaves the buffer clean', async () => {
    useSettingsStore.setState((s) => ({ settings: { ...s.settings, formatOnSave: true } }))
    const id = openDirtyBuffer('ws-owner', 'a.ts', 'const a=1')
    formatDocumentWithLsp.mockResolvedValue('const a = 1\n')

    expect(await saveBuffer(id)).toBe(true)

    expect(formatDocumentWithLsp).toHaveBeenCalledWith(A, 'const a=1')
    expect(writesFor('a.ts')).toEqual(['const a = 1\n'])
    expect(textOf(A)).toEqual({ text: 'const a = 1\n', saved: 'const a = 1\n' })
    expect(bufferById(id).isDirty).toBe(false)
  })

  it('drops a format result when an edit lands while the formatter runs', async () => {
    useSettingsStore.setState((s) => ({ settings: { ...s.settings, formatOnSave: true } }))
    const id = openDirtyBuffer('ws-owner', 'a.ts', 'const a=1')
    formatDocumentWithLsp.mockImplementation(() => {
      replaceDocumentText(A, 'const a=12')
      return Promise.resolve('const a = 1\n')
    })

    expect(await saveBuffer(id)).toBe(true)

    expect(writesFor('a.ts')).toEqual(['const a=12'])
    expect(textOf(A)).toEqual({ text: 'const a=12', saved: 'const a=12' })
  })
})

describe('buffer-save — Save As onto a document that is already open', () => {
  const FOO = workspaceRef('ws-owner', 'foo.ts')
  const UNTITLED = workspaceRef('ws-owner', 'untitled:Untitled-1')

  type Flags = { isVirtual?: boolean; isPreview?: boolean }
  function openIn(paneId: string, doc: DocumentRef, content: string, flags: Flags = {}): string {
    const spec = { type: 'editor' as const, doc, name: doc.path, content, ...flags }
    return windowPaneStore.getState().bufferActions.openContent(spec, { paneId })
  }

  /** Save As whose write stays in flight while `during` runs. */
  async function saveAsDuring(during: () => void): Promise<boolean> {
    let started = () => {}
    let finish = () => {}
    const begun = new Promise<void>((resolve) => (started = resolve))
    mockFetch.mockImplementationOnce(() => {
      started()
      return new Promise((resolve) => (finish = () => resolve({})))
    })
    const save = saveActiveBuffer()
    await begun
    during()
    finish()
    return save
  }

  function tabsOf(paneId: string) {
    const pane = windowPaneStore.getState().panes[paneId]
    return { tabs: pane?.editorTabIds, active: pane?.activeEditorTabId }
  }

  const holdersOfFoo = () =>
    windowPaneStore.getState().buffers.flatMap((b) => (b.path === 'foo.ts' ? [b.id] : []))

  beforeEach(() => vi.stubGlobal('prompt', () => 'foo.ts'))
  afterEach(() => {
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  it('a clean target takes the saved text, and the untitled tab becomes it in place', async () => {
    const target = openIn(BOTTOM_PANE_ID, FOO, 'old')
    const before = openIn(ROOT_PANE_ID, A, 'a')
    const untitled = openIn(ROOT_PANE_ID, UNTITLED, 'new text', { isVirtual: true })
    const after = openIn(ROOT_PANE_ID, B, 'b')
    windowPaneStore.getState().paneActions.activateEditorTabInPane(ROOT_PANE_ID, untitled)
    useGitBlameStore.setState({ blame: { [blameKey('ws-owner', 'foo.ts')]: success([]) } })

    expect(await saveActiveBuffer()).toBe(true)

    expect(writesFor('foo.ts')).toEqual(['new text'])
    expect(holdersOfFoo()).toEqual([target])
    expect(textOf(FOO)).toEqual({ text: 'new text', saved: 'new text' })
    expect(bufferById(target).isDirty).toBe(false)
    expect(tabsOf(ROOT_PANE_ID)).toEqual({ tabs: [before, target, after], active: target })
    expect(bufferById(untitled)).toBeUndefined()
    expect(useGitBlameStore.getState().blame).toEqual({})
  })

  it('refuses a target with unsaved edits and writes nothing', async () => {
    const target = openIn(BOTTOM_PANE_ID, FOO, 'old')
    replaceDocumentText(FOO, 'their edits')
    const untitled = openIn(ROOT_PANE_ID, UNTITLED, 'new text', { isVirtual: true })
    const error = vi.spyOn(toast, 'error')

    expect(await saveActiveBuffer()).toBe(false)

    expect(mockFetch).not.toHaveBeenCalled()
    expect(error).toHaveBeenCalledWith('Save or discard changes in foo.ts first')
    expect(holdersOfFoo()).toEqual([target])
    expect(textOf(FOO)).toEqual({ text: 'their edits', saved: 'old' })
    expect(textOf(UNTITLED)).toEqual({ text: 'new text', saved: 'new text' })
    expect(tabsOf(ROOT_PANE_ID)).toEqual({ tabs: [untitled], active: untitled })
  })

  it('closes the untitled tab and focuses the target when its pane already holds it', async () => {
    const target = openIn(ROOT_PANE_ID, FOO, 'old')
    openIn(ROOT_PANE_ID, UNTITLED, 'new text', { isVirtual: true })

    expect(await saveActiveBuffer()).toBe(true)

    expect(holdersOfFoo()).toEqual([target])
    expect(textOf(FOO)).toEqual({ text: 'new text', saved: 'new text' })
    expect(tabsOf(ROOT_PANE_ID)).toEqual({ tabs: [target], active: target })
  })

  it('a document opened while the write runs takes the saved text and the untitled tab', async () => {
    const untitled = openIn(ROOT_PANE_ID, UNTITLED, 'new text', { isVirtual: true })
    let target = ''

    expect(await saveAsDuring(() => (target = openIn(BOTTOM_PANE_ID, FOO, 'old')))).toBe(true)

    expect(holdersOfFoo()).toEqual([target])
    expect(textOf(FOO)).toEqual({ text: 'new text', saved: 'new text' })
    expect(bufferById(target).isDirty).toBe(false)
    expect(tabsOf(ROOT_PANE_ID)).toEqual({ tabs: [target], active: target })
    expect(bufferById(untitled)).toBeUndefined()
  })

  it('keeps edits typed into the untitled tab while the write runs', async () => {
    const target = openIn(BOTTOM_PANE_ID, FOO, 'old')
    openIn(ROOT_PANE_ID, UNTITLED, 'new text', { isVirtual: true })

    expect(await saveAsDuring(() => replaceDocumentText(UNTITLED, 'new text, and more'))).toBe(true)

    expect(holdersOfFoo()).toEqual([target])
    expect(textOf(FOO)).toEqual({ text: 'new text, and more', saved: 'new text' })
    expect(bufferById(target).isDirty).toBe(true)
    expect(tabsOf(ROOT_PANE_ID)).toEqual({ tabs: [target], active: target })
  })

  it('a target edited during the write keeps its edits and is flagged as changed on disk', async () => {
    const target = openIn(BOTTOM_PANE_ID, FOO, 'old')
    const untitled = openIn(ROOT_PANE_ID, UNTITLED, 'new text', { isVirtual: true })
    const warning = vi.spyOn(toast, 'warning')

    expect(await saveAsDuring(() => replaceDocumentText(FOO, 'their edits'))).toBe(true)

    expect(textOf(FOO)).toEqual({ text: 'their edits', saved: 'old' })
    expect(bufferById(target)).toMatchObject({ isDirty: true, hasExternalChange: true })
    expect(warning).toHaveBeenCalledWith('foo.ts changed on disk', expect.any(String))
    expect(tabsOf(ROOT_PANE_ID)).toEqual({ tabs: [target], active: target })
    expect(bufferById(untitled)).toBeUndefined()
  })

  it('a target edited to exactly the written text ends clean, with no warning', async () => {
    const target = openIn(BOTTOM_PANE_ID, FOO, 'old')
    const warning = vi.spyOn(toast, 'warning')
    openIn(ROOT_PANE_ID, UNTITLED, 'new text', { isVirtual: true })

    expect(await saveAsDuring(() => replaceDocumentText(FOO, 'new text'))).toBe(true)

    expect(warning).not.toHaveBeenCalled()
    expect(textOf(FOO)).toEqual({ text: 'new text', saved: 'new text' })
    expect(bufferById(target)).toMatchObject({ isDirty: false, hasExternalChange: false })
    expect(tabsOf(ROOT_PANE_ID)).toEqual({ tabs: [target], active: target })
  })

  it('keeps the untitled tab and all its text when both sides moved during the write', async () => {
    const target = openIn(BOTTOM_PANE_ID, FOO, 'old')
    const untitled = openIn(ROOT_PANE_ID, UNTITLED, 'new text', { isVirtual: true })

    const saved = await saveAsDuring(() => {
      replaceDocumentText(FOO, 'their edits')
      replaceDocumentText(UNTITLED, 'new text, and more')
    })

    expect(saved).toBe(false)
    expect(writesFor('foo.ts')).toEqual(['new text'])
    expect(textOf(FOO)).toEqual({ text: 'their edits', saved: 'old' })
    expect(bufferById(target).hasExternalChange).toBe(true)
    expect(documentText(UNTITLED)).toBe('new text, and more')
    expect(tabsOf(ROOT_PANE_ID)).toEqual({ tabs: [untitled], active: untitled })
  })

  it('a failed write onto an open document changes nothing and says why', async () => {
    const target = openIn(BOTTOM_PANE_ID, FOO, 'old')
    const untitled = openIn(ROOT_PANE_ID, UNTITLED, 'new text', { isVirtual: true })
    mockFetch.mockRejectedValueOnce(new Error('disk full'))
    const error = vi.spyOn(toast, 'error')

    expect(await saveActiveBuffer()).toBe(false)

    expect(error).toHaveBeenCalledWith('Failed to save file', expect.any(String))
    expect(holdersOfFoo()).toEqual([target])
    expect(textOf(FOO)).toEqual({ text: 'old', saved: 'old' })
    expect(textOf(UNTITLED)).toEqual({ text: 'new text', saved: 'new text' })
    expect(tabsOf(ROOT_PANE_ID)).toEqual({ tabs: [untitled], active: untitled })
    expect(tabsOf(BOTTOM_PANE_ID)).toEqual({ tabs: [target], active: target })
  })

  it('a failed new-file Save As says why and keeps the untitled tab', async () => {
    const untitled = openIn(ROOT_PANE_ID, UNTITLED, 'new text', { isVirtual: true })
    mockFetch.mockRejectedValueOnce(new Error('disk full'))
    const error = vi.spyOn(toast, 'error')

    expect(await saveActiveBuffer()).toBe(false)

    expect(error).toHaveBeenCalledWith('Failed to save file', expect.any(String))
    expect(bufferById(untitled).path).toBe('untitled:Untitled-1')
    expect(textOf(UNTITLED)).toEqual({ text: 'new text', saved: 'new text' })
  })

  it('Save As onto a preview tab leaves it kept, not a preview', async () => {
    const target = openIn(BOTTOM_PANE_ID, FOO, 'old', { isPreview: true })
    openIn(ROOT_PANE_ID, UNTITLED, 'new text', { isVirtual: true })

    expect(await saveActiveBuffer()).toBe(true)

    expect(bufferById(target).isPreview).toBe(false)
    expect(textOf(FOO)).toEqual({ text: 'new text', saved: 'new text' })
  })
})
