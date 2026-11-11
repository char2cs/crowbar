import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'
import { act, render, screen } from '@testing-library/react'
import type { Value } from 'platejs'
import { workspaceRef, type DocumentRef } from '@/lib/api/documents'
import { MONACO_LOAD_MS } from '@/__tests__/__fixtures__/documents'
import { getEditorHost, loadEditorHost } from '@/features/editor/host/editor-host'
import {
  applyExternalText,
  documentText,
  replaceDocumentText,
} from '@/features/editor/documents/document-text'
import { findDocumentBuffer } from '@/features/editor/documents/open-document'
import { MarkdownEditorPane } from '@/features/editor/markdown/plate/markdown-editor-pane'
import { parseMarkdown } from '@/features/editor/markdown/plate/markdown-serialization'
import { useMarkdownViewStore } from '@/features/editor/markdown/plate/markdown-view-store'
import {
  resetWindowPaneStoreForTests,
  windowPaneStore,
} from '@/features/panes/stores/window-pane-store'
import type { EditorContent } from '@/features/panes/types/pane-content'
import { toast } from '@/features/window/stores/toast-store'
import { __resetWorkspaceScopesForTest, recordWorkspaceScope } from '@/lib/workspace-scope'
import {
  loadProjects,
  pairRemoteHost,
  projectOn,
  resetHostWorld,
} from '@/__tests__/__fixtures__/remote-host'
import { resetEntities, seedRepos } from '@/__tests__/__fixtures__/sidebar-entities'

// Buffer fixtures keyed by bufferId, so each test picks the shape it needs with no shared mutable
// state; hoisted for the `vi.mock` factory below.
const { BUFFER_CONTENT, THROW_SENTINEL } = vi.hoisted(() => {
  const THROW_SENTINEL = '__THROW_ON_DESERIALIZE__'
  return {
    THROW_SENTINEL,
    BUFFER_CONTENT: {
      b1: '# Hello\n\nWorld **bold**.\n',
      // The round trip is not byte-stable for GFM tables (`| --- |` becomes `| - |`), so this
      // would falsely dirty a pristine file if flush wrote unconditionally.
      'b-table': '| a | b |\n| --- | --- |\n| 1 | 2 |\n',
      // A second, distinguishable document: switching `.md` tabs is a prop update (no key), so this
      // content must appear and `b1`'s text must never be written to `b2`.
      b2: '# Second\n\nAnother **document**.\n',
      'b-throw': THROW_SENTINEL,
      // An empty and a whitespace-only file: Plate's `init` synthesizes a paragraph, so a baseline
      // from `initialValue` would make the first flush write a zero-width space.
      'b-empty': '',
      'b-blank': '\n',
      // Real-world frontmatter ahead of prose; fed whole into Plate, the `---` block would become a
      // thematic break + setext heading.
      'b-frontmatter':
        '---\ntitle: Plate Live Check\ntags: [verification, markdown]\n---\n\n# Hello\n\nWorld.\n',
      // Frontmatter + a GFM table body: pristine mount/unmount of this must
      // perform NO write, exercising the same non-byte-stable-round-trip
      // guard as `b-table` but with a frontmatter block also present.
      'b-frontmatter-table': '---\ntitle: Table Doc\n---\n| a | b |\n| --- | --- |\n| 1 | 2 |\n',
      // A README-style raw HTML header block with an external anchor. remark
      // parses the whole block as ONE mdast `html` node, so this is what
      // renders through HtmlElement — the only place anchors appear inside
      // markup the editor holds as an opaque string it cannot annotate.
      'b-html':
        '<div align="center">\n  <h1>Crowbar</h1>\n  <a href="https://example.com/docs">Docs</a>\n</div>\n\n# Heading\n',
      // The same kind of block linking its host's own dev server, in a workspace on another host.
      'b-remote-html': '<div>\n  <a href="http://localhost:3000/">Preview</a>\n</div>\n',
      // A hand-wrapped doc: soft line breaks, `_` emphasis and a compact table, none of which a
      // fresh serialize writes back as-is.
      'b-wrapped':
        '# Notes\n\nA _wrapped_ paragraph\nover two lines.\n\n|a|b|\n|-|-|\n|1|2|\n\nLast one.\n',
    } as Record<string, string>,
  }
})

// Each fixture is an open document with its tab, so the pane reads and writes
// it through the same seam the app uses; reading `documentText` also runs the
// pane's pending write, the way a save does.
// `b-remote-html` lives in a workspace on another host.
const wsOf = (bufferId: string) => (bufferId === 'b-remote-html' ? 'w-remote' : 'ws-1')
const docOf = (bufferId: string): DocumentRef => workspaceRef(wsOf(bufferId), `${bufferId}.md`)

function openFixtures(): void {
  resetWindowPaneStoreForTests()
  windowPaneStore.setState({
    buffers: Object.entries(BUFFER_CONTENT).map(([id, text]): EditorContent => {
      getEditorHost()!.documents.open(docOf(id), 'markdown', text)
      return {
        id,
        type: 'editor',
        authority: 'workspace',
        workspaceId: wsOf(id),
        path: `${id}.md`,
        name: `${id}.md`,
        isDirty: false,
        isVirtual: false,
      }
    }),
  })
}

const isDirty = (bufferId: string) => findDocumentBuffer(docOf(bufferId))?.isDirty

// The single approved shell-opener seam. Mocked here so the raw-HTML anchor
// test can assert the click reached the OS browser without launching one.
const openExternalUrl = vi.fn()
vi.mock('@/lib/external-open', () => ({
  openExternalUrl: (url: string) => openExternalUrl(url),
}))

// Wrap `parseMarkdown` (pass-through except for the sentinel) so the ErrorBoundary test
// can force a deserialize throw without touching real parser internals.
vi.mock('@/features/editor/markdown/plate/markdown-serialization', async (importOriginal) => {
  const actual =
    await importOriginal<typeof import('@/features/editor/markdown/plate/markdown-serialization')>()
  return {
    ...actual,
    parseMarkdown: (md: string) => {
      if (md === THROW_SENTINEL) throw new Error('boom: forced deserialize failure')
      return actual.parseMarkdown(md)
    },
  }
})

// The live editor, so a test can make a REAL edit to `editor.children` (what `flush()` reads):
// Slate typing can't be simulated in jsdom (`setSelectionRange` on a contenteditable throws).
let capturedEditor: { children: Value } | null = null
vi.mock('platejs/react', async (importOriginal) => {
  const actual = await importOriginal<typeof import('platejs/react')>()
  return {
    ...actual,
    usePlateEditor: (opts: Parameters<typeof actual.usePlateEditor>[0]) => {
      const editor = actual.usePlateEditor(opts)
      capturedEditor = editor as unknown as { children: Value }
      return editor
    },
  }
})

beforeAll(() => loadEditorHost(), MONACO_LOAD_MS)

beforeEach(() => {
  openExternalUrl.mockClear()
  capturedEditor = null
  openFixtures()
})

afterEach(() => {
  vi.useRealTimers()
  useMarkdownViewStore.setState({ views: {} })
})

describe('MarkdownEditorPane', () => {
  it('renders the markdown content as rich text', async () => {
    render(<MarkdownEditorPane paneId="p1" bufferId="b1" />)
    // The heading text and the bold run are present in the rendered rich output.
    expect(await screen.findByText('Hello')).toBeInTheDocument()
    expect(screen.getByText('bold')).toBeInTheDocument()
  })

  // A locked branch refuses every write, so typing is refused up front, not at save.
  it('is read-only on a locked workspace', async () => {
    seedRepos([
      {
        id: 'r1',
        projectId: 'p1',
        name: 'repo',
        avatarLabel: 'R',
        avatarColor: 'r',
        workspaces: [],
        defaultWorkspaceId: 'ws-1',
        defaultWorkspaceStatus: 'locked',
      },
    ])
    render(<MarkdownEditorPane paneId="p1" bufferId="b1" />)
    const heading = await screen.findByText('Hello')

    expect(heading.closest('[contenteditable]')).toHaveAttribute('contenteditable', 'false')
    resetEntities()
  })

  it('does not write when a reader flushes a pristine editor', async () => {
    render(<MarkdownEditorPane paneId="p1" bufferId="b1" />)
    await screen.findByText('Hello')

    // Selection/cursor-only `onChange` noise and a same-content re-serialize
    // must never write — a pristine flush is a true no-op.
    expect(documentText(docOf('b1'))).toBe(BUFFER_CONTENT.b1)
    expect(isDirty('b1')).toBe(false)
  })

  it('a reader of the text gets an edit the editor has not flushed yet', async () => {
    render(<MarkdownEditorPane paneId="p1" bufferId="b1" />)
    await screen.findByText('Hello')
    expect(capturedEditor).not.toBeNull()

    capturedEditor!.children = parseMarkdown('# Hello\n\nWorld **changed**.\n').value

    expect(documentText(docOf('b1'))).toContain('changed')
    expect(isDirty('b1')).toBe(true)
  })

  it('flushes changed content to the correct buffer on unmount', async () => {
    const { unmount } = render(<MarkdownEditorPane paneId="p1" bufferId="b1" />)
    await screen.findByText('Hello')
    expect(capturedEditor).not.toBeNull()

    capturedEditor!.children = parseMarkdown('# Hello\n\nWorld **changed**.\n').value
    unmount()

    expect(documentText(docOf('b1'))).toContain('changed')
    expect(documentText(docOf('b2'))).toBe(BUFFER_CONTENT.b2)
  })

  // Opening a pristine file and unmounting without an edit must not write, even when the round trip
  // re-canonicalizes its content (GFM tables): with autoSave on that would rewrite it on disk.
  it('does not write on unmount when the editor is pristine, even for a GFM table', async () => {
    const { unmount } = render(<MarkdownEditorPane paneId="p1" bufferId="b-table" />)
    await screen.findByText('a')

    unmount()

    expect(documentText(docOf('b-table'))).toBe(BUFFER_CONTENT['b-table'])
  })

  // Frontmatter never goes through Plate's round trip (it would become a thematic break + setext
  // heading): the flushed leading block is byte-identical to the original, even after a body edit.
  it('preserves the frontmatter block byte-identically after a real body edit', async () => {
    render(<MarkdownEditorPane paneId="p1" bufferId="b-frontmatter" />)
    await screen.findByText('Hello')
    expect(capturedEditor).not.toBeNull()

    capturedEditor!.children = parseMarkdown('# Hello\n\nWorld **changed**.\n').value

    const md = documentText(docOf('b-frontmatter'))!
    expect(
      md.startsWith('---\ntitle: Plate Live Check\ntags: [verification, markdown]\n---\n'),
    ).toBe(true)
    expect(md).toContain('changed')
  })

  // A save rewrites only what the user edited: a soft line break elsewhere must not become `\`.
  it('writes an edit to one paragraph without touching the rest of the file', async () => {
    render(<MarkdownEditorPane paneId="p1" bufferId="b-wrapped" />)
    await screen.findByText('Notes')

    const children = capturedEditor!.children
    capturedEditor!.children = children.map((node, i) =>
      i === children.length - 1 ? { type: 'p', children: [{ text: 'Last one, edited.' }] } : node,
    ) as Value

    expect(documentText(docOf('b-wrapped'))).toBe(
      '# Notes\n\nA _wrapped_ paragraph\nover two lines.\n\n|a|b|\n|-|-|\n|1|2|\n\nLast one, edited.\n',
    )
  })

  // As the GFM-table pristine flush above, with frontmatter too: opening and closing a
  // frontmatter+table document without editing it writes nothing.
  it('does not write on mount+unmount of a pristine frontmatter+table document', async () => {
    const { unmount } = render(<MarkdownEditorPane paneId="p1" bufferId="b-frontmatter-table" />)
    await screen.findByText('a')

    unmount()

    expect(documentText(docOf('b-frontmatter-table'))).toBe(BUFFER_CONTENT['b-frontmatter-table'])
  })

  // pane-container renders the active buffer with no `key`, so a `.md` → `.md` tab switch is a
  // prop update, not a remount: the editor must follow `bufferId`, never writing A's text to B.
  describe('when the bufferId prop changes (md -> md tab switch)', () => {
    it('shows the new buffer content, not the previous one', async () => {
      const { rerender } = render(<MarkdownEditorPane paneId="p1" bufferId="b1" />)
      await screen.findByText('Hello')

      rerender(<MarkdownEditorPane paneId="p1" bufferId="b2" />)

      expect(await screen.findByText('Second')).toBeInTheDocument()
      expect(screen.queryByText('Hello')).toBeNull()
    })

    it('never writes the previous buffer’s text into the new buffer', async () => {
      const { rerender } = render(<MarkdownEditorPane paneId="p1" bufferId="b1" />)
      await screen.findByText('Hello')

      rerender(<MarkdownEditorPane paneId="p1" bufferId="b2" />)
      await screen.findByText('Second')

      // The user now edits what they believe (correctly) is b2.
      capturedEditor!.children = [
        ...capturedEditor!.children,
        { type: 'p', children: [{ text: 'typed into b2' }] },
      ] as Value

      const b2 = documentText(docOf('b2'))
      expect(b2).toContain('typed into b2')
      expect(b2).toContain('Second')
      // The smoking gun: b1's document reaching b1's neighbour on disk.
      expect(b2).not.toContain('Hello')
      expect(documentText(docOf('b1'))).toBe(BUFFER_CONTENT.b1)
    })
  })

  // Plate's `init` replaces empty children with a synthesized paragraph, so a baseline from
  // `initialValue` would make the first flush write a U+200B over a file the user only opened.
  describe('an empty document', () => {
    it('does not write on mount+unmount of an empty file', async () => {
      const { unmount } = render(<MarkdownEditorPane paneId="p1" bufferId="b-empty" />)
      // Waits on the "Write…" placeholder (Slate's native one, shown only for
      // a pristine wholly-empty document — see the placeholder note in
      // markdown-editor-pane.tsx) rather than `getByRole('textbox')`:
      // BlockSelectionPlugin (block-menu-kit.tsx) also mounts a hidden
      // `<input>` for its own copy/cut/paste handling, which — like any
      // untyped `<input>` — carries an implicit `textbox` role too, so that
      // query now matches two elements.
      await screen.findByText('Write…')

      unmount()

      expect(documentText(docOf('b-empty'))).toBe('')
    })

    it('does not write on mount+unmount of a whitespace-only file', async () => {
      const { unmount } = render(<MarkdownEditorPane paneId="p1" bufferId="b-blank" />)
      await screen.findByText('Write…')

      unmount()

      expect(documentText(docOf('b-blank'))).toBe('\n')
    })

    // Asserted on `editor.children` rather than on test-env equivalence: the
    // core plugins drop NodeIdPlugin under NODE_ENV=test, so the mount
    // transform is only partly exercised here. What must hold either way is
    // that the editor's ACTUAL children — whatever Plate synthesized — are what
    // the baseline is measured from, so a flush of the untouched document is a
    // no-op.
    it('measures its baseline from the editor children Plate actually synthesized', async () => {
      render(<MarkdownEditorPane paneId="p1" bufferId="b-empty" />)
      await screen.findByText('Write…')

      // Plate replaced the empty parsed value with a synthesized paragraph…
      expect(capturedEditor!.children.length).toBeGreaterThan(0)
      // …and flushing that untouched synthesized document still writes nothing.
      expect(documentText(docOf('b-empty'))).toBe('')
    })
  })

  // The document's text is replaced under a mounted editor (a second pane on the file, an external
  // reload after checkout); the stale parsed copy must never be flushed back over the new text.
  describe('when the document text changes underneath the editor', () => {
    const reloaded = '# Reloaded\n\nFrom disk.\n'

    it('adopts external text while the editor is pristine', async () => {
      render(<MarkdownEditorPane paneId="p1" bufferId="b1" />)
      await screen.findByText('Hello')

      // Slate re-renders a microtask after the change, not 150 ms later.
      await act(async () => applyExternalText(docOf('b1'), reloaded, { saved: true }))

      expect(screen.getByText('Reloaded')).toBeInTheDocument()
      expect(screen.queryByText('Hello')).toBeNull()
    })

    it('adopts an edit made in another pane while the editor is pristine', async () => {
      render(<MarkdownEditorPane paneId="p1" bufferId="b1" />)
      await screen.findByText('Hello')

      act(() => replaceDocumentText(docOf('b1'), '# Other pane\n\nTyped there.\n'))

      expect(await screen.findByText('Other pane')).toBeInTheDocument()
      expect(screen.queryByText('Hello')).toBeNull()
    })

    it('adopting new content is not an edit — it must not write back', async () => {
      render(<MarkdownEditorPane paneId="p1" bufferId="b1" />)
      await screen.findByText('Hello')

      act(() => applyExternalText(docOf('b1'), reloaded, { saved: true }))
      await screen.findByText('Reloaded')

      expect(documentText(docOf('b1'))).toBe(reloaded)
      expect(isDirty('b1')).toBe(false)
    })

    it('adopts a new frontmatter block along with the body', async () => {
      render(<MarkdownEditorPane paneId="p1" bufferId="b1" />)
      await screen.findByText('Hello')

      act(() =>
        applyExternalText(
          docOf('b1'),
          '---\ntitle: Checked Out\n---\n\n# Reloaded\n\nFrom disk.\n',
          { saved: true },
        ),
      )
      await screen.findByText('Reloaded')

      // The banner reflects the NEW frontmatter…
      expect(screen.getByText(/title: Checked Out/)).toBeInTheDocument()
      // …and a later edit re-attaches it byte-identically.
      capturedEditor!.children = parseMarkdown('# Reloaded\n\nEdited after reload.\n').value
      const md = documentText(docOf('b1'))!
      expect(md.startsWith('---\ntitle: Checked Out\n---\n')).toBe(true)
      expect(md).toContain('Edited after reload')
    })

    it('keeps in-progress local edits when the editor is dirty (no clobber)', async () => {
      render(<MarkdownEditorPane paneId="p1" bufferId="b1" />)
      await screen.findByText('Hello')

      capturedEditor!.children = parseMarkdown('# Hello\n\nWorld **edited here**.\n').value
      const edited = capturedEditor!.children
      act(() => applyExternalText(docOf('b1'), reloaded, { saved: true }))

      expect(capturedEditor!.children).toBe(edited)
      const md = documentText(docOf('b1'))
      expect(md).toContain('edited here')
      expect(md).not.toContain('Reloaded')
      expect(isDirty('b1')).toBe(true)
    })

    it('ignores its own write echoing back through the document', async () => {
      render(<MarkdownEditorPane paneId="p1" bufferId="b1" />)
      await screen.findByText('Hello')
      vi.useFakeTimers()

      capturedEditor!.children = parseMarkdown('# Hello\n\nWorld **changed**.\n').value
      const childrenBefore = capturedEditor!.children
      const written = documentText(docOf('b1'))
      // The same bytes come back as the document's text. Re-parsing them would
      // reset the caret mid-typing, so the document must be left exactly as-is.
      act(() => vi.advanceTimersByTime(300))

      expect(capturedEditor!.children).toBe(childrenBefore)
      expect(documentText(docOf('b1'))).toBe(written)
    })
  })

  it('falls back to source view when the rich editor throws during construction', async () => {
    await act(async () => {
      render(<MarkdownEditorPane paneId="p1" bufferId="b-throw" />)
    })

    await screen.findByText(/switching to source view/i)

    expect(useMarkdownViewStore.getState().views['b-throw']).toBe('source')
  })
})

// A raw HTML block's anchors sit in markup the editor holds as one opaque string, so their clicks
// are delegated from the block. A click must never navigate the WKWebView (it would replace the
// whole app): an external href reaches the OS default browser instead.
describe('MarkdownEditorPane raw-HTML anchors', () => {
  it('sends an external raw-HTML link to the OS browser and cancels the webview navigation', async () => {
    render(<MarkdownEditorPane paneId="p1" bufferId="b-html" />)

    const anchor = await screen.findByText('Docs')
    expect(anchor.closest('.markdown-html-block')).not.toBeNull()

    // dispatchEvent returns false exactly when preventDefault was called, which
    // is the assertion that matters: an uncancelled click is the webview hijack.
    const event = new MouseEvent('click', { bubbles: true, cancelable: true })
    const notCancelled = anchor.dispatchEvent(event)

    expect(notCancelled).toBe(false)
    expect(event.defaultPrevented).toBe(true)
    expect(openExternalUrl).toHaveBeenCalledWith('https://example.com/docs')
  })

  // A README on another host that links its own localhost means that host, not this computer.
  it('does not open a loopback link from a workspace on another host here', async () => {
    pairRemoteHost('h1', 'Build box')
    loadProjects(projectOn('p', 'h1'))
    recordWorkspaceScope({ projectId: 'p', repoId: 'r', wsId: 'w-remote' })
    const info = vi.spyOn(toast, 'info').mockReturnValue('')
    try {
      render(<MarkdownEditorPane paneId="p1" bufferId="b-remote-html" />)

      const anchor = await screen.findByText('Preview')
      anchor.dispatchEvent(new MouseEvent('click', { bubbles: true, cancelable: true }))

      expect(info.mock.calls.at(-1)?.[0]).toMatch(/^This link is on Build box/)
      expect(openExternalUrl).not.toHaveBeenCalled()
    } finally {
      info.mockRestore()
      resetHostWorld()
      __resetWorkspaceScopesForTest()
    }
  })

  it('leaves clicks elsewhere in the block alone', async () => {
    render(<MarkdownEditorPane paneId="p1" bufferId="b-html" />)

    const heading = await screen.findByText('Crowbar')
    const event = new MouseEvent('click', { bubbles: true, cancelable: true })
    const notCancelled = heading.dispatchEvent(event)

    // Non-anchor content has no navigation to cancel and nothing to open.
    expect(notCancelled).toBe(true)
    expect(openExternalUrl).not.toHaveBeenCalled()
  })
})
