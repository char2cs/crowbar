import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { act, render, screen } from '@testing-library/react'
import type { Value } from 'platejs'
import { MarkdownEditorPane } from '@/features/editor/markdown/plate/markdown-editor-pane'
import { markdownToPlateValue } from '@/features/editor/markdown/plate/markdown-serialization'
import { useMarkdownViewStore } from '@/features/editor/markdown/plate/markdown-view-store'

// Buffer fixtures keyed by bufferId, so each test can pick the shape it needs
// (normal prose, a GFM table, or a sentinel that forces a deserialize throw)
// without any test-order-dependent mutable shared state. Declared via
// `vi.hoisted` so it's safe to reference from the hoisted `vi.mock` factory
// below.
const { BUFFER_CONTENT, THROW_SENTINEL } = vi.hoisted(() => {
  const THROW_SENTINEL = '__THROW_ON_DESERIALIZE__'
  return {
    THROW_SENTINEL,
    BUFFER_CONTENT: {
      b1: '# Hello\n\nWorld **bold**.\n',
      // Regression fixture for Finding 1: `plateValueToMarkdown(markdownToPlateValue(md))`
      // is NOT byte-stable for GFM tables (`| --- |` round-trips to `| - |`),
      // so this is the case that would falsely dirty a pristine file under
      // the old "write unconditionally" behavior.
      'b-table': '| a | b |\n| --- | --- |\n| 1 | 2 |\n',
      // C1 fixture: a SECOND, clearly distinguishable document. Switching
      // between two `.md` tabs is a prop update (pane-container renders the
      // active buffer without a key), so this is the content that must appear
      // — and `b1`'s text must never be written to `b2`.
      b2: '# Second\n\nAnother **document**.\n',
      'b-throw': THROW_SENTINEL,
      // C2 fixtures: an empty file and a whitespace-only file. Plate's `init`
      // replaces empty children with a synthesized paragraph, so the editor's
      // children at mount are NOT `initialValue` — a baseline taken from
      // `initialValue` is `''` and the first flush writes an invisible
      // zero-width space over a file the user never touched.
      'b-empty': '',
      'b-blank': '\n',
      // Task 8 (data safety): a real-world frontmatter block ahead of prose.
      // Feeding this whole string into Plate would rewrite the `---` block
      // into a thematic break + setext heading — the exact corruption the
      // frontmatter split/join seam exists to prevent.
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
    } as Record<string, string>,
  }
})

// A minimal reactive stand-in for the buffer store: `content` can be replaced
// underneath a MOUNTED editor (a second pane flushing the same file, or
// external-buffer-sync applying a git checkout), and subscribers re-render —
// exactly the seam I1/I2 are about. The real store is a zustand selector, so a
// content swap likewise produces a new buffer object and re-renders.
const bufferStore = vi.hoisted(() => {
  const listeners = new Set<() => void>()
  let contents: Record<string, string> = {}
  let version = 0
  return {
    reset(initial: Record<string, string>) {
      contents = { ...initial }
      version++
    },
    get: (id: string) => contents[id] ?? '',
    setContent(id: string, content: string) {
      contents[id] = content
      version++
      listeners.forEach((l) => l())
    },
    subscribe(l: () => void) {
      listeners.add(l)
      return () => listeners.delete(l)
    },
    getVersion: () => version,
  }
})

// Buffer + write-seam doubles, mocked at the REAL module specifiers so the
// component under test resolves them exactly as it would in the app.
const handleContentChange = vi.fn()
vi.mock('@/features/workspace/stores/hooks/use-buffer-store', async () => {
  const { useSyncExternalStore } = await import('react')
  return {
    useBufferById: (bufferId: string) => {
      // Subscribe so a `setContent` re-renders the pane, like the real store.
      useSyncExternalStore(bufferStore.subscribe, bufferStore.getVersion)
      const content = bufferStore.get(bufferId)
      return {
        id: bufferId,
        type: 'editor',
        path: `/repo/${bufferId}.md`,
        name: `${bufferId}.md`,
        content,
        savedContent: content,
        isDirty: false,
      }
    },
  }
})
vi.mock('@/features/editor/stores/editor-app-store', () => ({
  useEditorAppStore: { use: { actions: () => ({ handleContentChange }) } },
}))
vi.mock('@/features/window/stores/toast-store', () => ({
  toast: { error: vi.fn() },
}))

// The single approved shell-opener seam. Mocked here so the raw-HTML anchor
// test can assert the click reached the OS browser without launching one.
const openExternalUrl = vi.fn()
vi.mock('@/lib/external-open', () => ({
  openExternalUrl: (url: string) => openExternalUrl(url),
}))

// Deserialization normally lives entirely inside `markdownToPlateValue`. Wrap
// it (pass-through for everything except the sentinel) so the ErrorBoundary
// test can force the exact throw Finding 2 is about, without touching real
// parser internals.
vi.mock('@/features/editor/markdown/plate/markdown-serialization', async (importOriginal) => {
  const actual =
    await importOriginal<typeof import('@/features/editor/markdown/plate/markdown-serialization')>()
  return {
    ...actual,
    markdownToPlateValue: (md: string) => {
      if (md === THROW_SENTINEL) throw new Error('boom: forced deserialize failure')
      return actual.markdownToPlateValue(md)
    },
  }
})

// Capture the live editor instance `usePlateEditor` builds inside the
// component so tests can apply a REAL content change directly to
// `editor.children` (the same field `flush()` reads). Slate typing can't be
// simulated in jsdom (`setSelectionRange` on a contenteditable throws "Not
// implemented"), so this is the reliable way to exercise the "an actual edit
// happened" path end-to-end through the real component.
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

beforeEach(() => {
  handleContentChange.mockClear()
  openExternalUrl.mockClear()
  capturedEditor = null
  bufferStore.reset(BUFFER_CONTENT)
})

afterEach(() => {
  useMarkdownViewStore.setState({ views: {} })
})

describe('MarkdownEditorPane', () => {
  it('renders the markdown content as rich text', async () => {
    render(<MarkdownEditorPane paneId="p1" bufferId="b1" />)
    // The heading text and the bold run are present in the rendered rich output.
    expect(await screen.findByText('Hello')).toBeInTheDocument()
    expect(screen.getByText('bold')).toBeInTheDocument()
  })

  it('does not write on flush-editor-content when the editor is pristine', async () => {
    render(<MarkdownEditorPane paneId="p1" bufferId="b1" />)
    await screen.findByText('Hello')

    window.dispatchEvent(new Event('flush-editor-content'))

    // Selection/cursor-only `onChange` noise and a same-content re-serialize
    // must never write — a pristine flush is a true no-op.
    expect(handleContentChange).not.toHaveBeenCalled()
  })

  it('writes the changed content on flush-editor-content after a real edit', async () => {
    render(<MarkdownEditorPane paneId="p1" bufferId="b1" />)
    await screen.findByText('Hello')
    expect(capturedEditor).not.toBeNull()

    capturedEditor!.children = markdownToPlateValue('# Hello\n\nWorld **changed**.\n')
    window.dispatchEvent(new Event('flush-editor-content'))

    expect(handleContentChange).toHaveBeenCalledTimes(1)
    const [md, , , , options] = handleContentChange.mock.calls[0] as [
      string,
      string | undefined,
      unknown,
      unknown,
      { targetBufferId?: string; skipUndoGrouping?: boolean } | undefined,
    ]
    expect(md).toContain('changed')
    expect(options?.targetBufferId).toBe('b1')
    expect(options?.skipUndoGrouping).toBe(true)
  })

  it('flushes changed content to the correct buffer on unmount', async () => {
    const { unmount } = render(<MarkdownEditorPane paneId="p1" bufferId="b1" />)
    await screen.findByText('Hello')
    expect(capturedEditor).not.toBeNull()

    capturedEditor!.children = markdownToPlateValue('# Hello\n\nWorld **changed**.\n')
    unmount()

    expect(handleContentChange).toHaveBeenCalledTimes(1)
    const [md, , , , options] = handleContentChange.mock.calls[0] as [
      string,
      string | undefined,
      unknown,
      unknown,
      { targetBufferId?: string } | undefined,
    ]
    expect(md).toContain('changed')
    expect(options?.targetBufferId).toBe('b1')
  })

  // Regression test for Finding 1 (CRITICAL): opening a pristine file and
  // unmounting/flushing without any edit must NOT write, even when the
  // deserialize -> serialize round trip is not byte-stable for the file's
  // content (GFM tables). Against the old "flush writes unconditionally"
  // behavior, this fails because the re-canonicalized table markdown
  // (`| --- |` -> `| - |`) differs from the original bytes and old code wrote
  // it anyway. With autoSave on, that meant opening a table-containing file
  // silently rewrote it on disk.
  it('does not write on unmount when the editor is pristine, even for a GFM table', async () => {
    const { unmount } = render(<MarkdownEditorPane paneId="p1" bufferId="b-table" />)
    await screen.findByText('a')

    unmount()

    expect(handleContentChange).not.toHaveBeenCalled()
  })

  // Task 8 (data safety, CRITICAL): frontmatter must never be fed through
  // Plate's markdown deserialize/serialize round trip — doing so rewrites the
  // `---` block into a thematic break + setext heading and destroys it. The
  // flushed content's leading frontmatter block must be byte-identical to
  // the buffer's original bytes, even after a real edit to the body.
  it('preserves the frontmatter block byte-identically after a real body edit', async () => {
    render(<MarkdownEditorPane paneId="p1" bufferId="b-frontmatter" />)
    await screen.findByText('Hello')
    expect(capturedEditor).not.toBeNull()

    capturedEditor!.children = markdownToPlateValue('# Hello\n\nWorld **changed**.\n')
    window.dispatchEvent(new Event('flush-editor-content'))

    expect(handleContentChange).toHaveBeenCalledTimes(1)
    const [md] = handleContentChange.mock.calls[0] as [string]
    expect(
      md.startsWith('---\ntitle: Plate Live Check\ntags: [verification, markdown]\n---\n'),
    ).toBe(true)
    expect(md).toContain('changed')
  })

  // Companion to the GFM-table pristine-flush regression above, but with
  // frontmatter present too: opening and closing a frontmatter+table
  // document without editing it must perform NO write at all.
  it('does not write on mount+unmount of a pristine frontmatter+table document', async () => {
    const { unmount } = render(<MarkdownEditorPane paneId="p1" bufferId="b-frontmatter-table" />)
    await screen.findByText('a')

    unmount()

    expect(handleContentChange).not.toHaveBeenCalled()
  })

  // C1 (CRITICAL): `pane-container` renders the active buffer with no `key`, so
  // switching between two `.md` tabs is a PROP UPDATE, not a remount. The
  // document is derived once (useMemo with `[]` deps, memoized editor) while
  // `bufferId` — and therefore the flush's `targetBufferId` — does change. The
  // result was file A's whole document being written into file B, destroying B.
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
      window.dispatchEvent(new Event('flush-editor-content'))

      const writesToB2 = handleContentChange.mock.calls.filter(
        ([, , , , options]) =>
          (options as { targetBufferId?: string } | undefined)?.targetBufferId === 'b2',
      ) as [string][]
      expect(writesToB2).toHaveLength(1)
      expect(writesToB2[0][0]).toContain('typed into b2')
      expect(writesToB2[0][0]).toContain('Second')
      // The smoking gun: b1's document reaching b1's neighbour on disk.
      expect(writesToB2[0][0]).not.toContain('Hello')
    })
  })

  // C2 (CRITICAL): Plate's `init` replaces empty children with a synthesized
  // paragraph, so `editor.children !== initialValue` at mount. A baseline taken
  // from `initialValue` is therefore `''`, and the first flush writes the
  // synthesized paragraph — an invisible U+200B plus a newline — over a file the
  // user only opened.
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

      expect(handleContentChange).not.toHaveBeenCalled()
    })

    it('does not write on mount+unmount of a whitespace-only file', async () => {
      const { unmount } = render(<MarkdownEditorPane paneId="p1" bufferId="b-blank" />)
      await screen.findByText('Write…')

      unmount()

      expect(handleContentChange).not.toHaveBeenCalled()
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
      window.dispatchEvent(new Event('flush-editor-content'))
      expect(handleContentChange).not.toHaveBeenCalled()
    })
  })

  // I1 (two panes on the same file) + I2 (external reload after a git
  // checkout): the buffer's `content` is replaced underneath a MOUNTED editor.
  // Plate held a parsed copy and never resynced, so the stale document was
  // flushed back over the new content on the user's next keystroke.
  describe('when the buffer content changes underneath the editor', () => {
    it('adopts the new content while the editor is pristine', async () => {
      render(<MarkdownEditorPane paneId="p1" bufferId="b1" />)
      await screen.findByText('Hello')

      act(() => bufferStore.setContent('b1', '# Reloaded\n\nFrom disk.\n'))

      expect(await screen.findByText('Reloaded')).toBeInTheDocument()
      expect(screen.queryByText('Hello')).toBeNull()
    })

    it('adopting new content is not an edit — it must not write back', async () => {
      render(<MarkdownEditorPane paneId="p1" bufferId="b1" />)
      await screen.findByText('Hello')

      act(() => bufferStore.setContent('b1', '# Reloaded\n\nFrom disk.\n'))
      await screen.findByText('Reloaded')
      window.dispatchEvent(new Event('flush-editor-content'))

      expect(handleContentChange).not.toHaveBeenCalled()
    })

    it('adopts a new frontmatter block along with the body', async () => {
      render(<MarkdownEditorPane paneId="p1" bufferId="b1" />)
      await screen.findByText('Hello')

      act(() =>
        bufferStore.setContent('b1', '---\ntitle: Checked Out\n---\n\n# Reloaded\n\nFrom disk.\n'),
      )
      await screen.findByText('Reloaded')

      // The banner reflects the NEW frontmatter…
      expect(screen.getByText(/title: Checked Out/)).toBeInTheDocument()
      // …and a later edit re-attaches it byte-identically.
      capturedEditor!.children = markdownToPlateValue('# Reloaded\n\nEdited after reload.\n')
      window.dispatchEvent(new Event('flush-editor-content'))
      const [md] = handleContentChange.mock.calls[0] as [string]
      expect(md.startsWith('---\ntitle: Checked Out\n---\n')).toBe(true)
      expect(md).toContain('Edited after reload')
    })

    it('keeps in-progress local edits when the editor is dirty (no clobber)', async () => {
      render(<MarkdownEditorPane paneId="p1" bufferId="b1" />)
      await screen.findByText('Hello')

      capturedEditor!.children = markdownToPlateValue('# Hello\n\nWorld **edited here**.\n')
      act(() => bufferStore.setContent('b1', '# Reloaded\n\nFrom disk.\n'))
      window.dispatchEvent(new Event('flush-editor-content'))

      expect(handleContentChange).toHaveBeenCalledTimes(1)
      const [md] = handleContentChange.mock.calls[0] as [string]
      expect(md).toContain('edited here')
      expect(md).not.toContain('Reloaded')
    })

    it('ignores its own write echoing back through the buffer', async () => {
      render(<MarkdownEditorPane paneId="p1" bufferId="b1" />)
      await screen.findByText('Hello')

      capturedEditor!.children = markdownToPlateValue('# Hello\n\nWorld **changed**.\n')
      window.dispatchEvent(new Event('flush-editor-content'))
      expect(handleContentChange).toHaveBeenCalledTimes(1)
      const [written] = handleContentChange.mock.calls[0] as [string]

      // The store applies our write; the same bytes come back as `content`.
      // Re-parsing them would reset the caret mid-typing, so the document must
      // be left exactly as-is.
      const childrenBefore = capturedEditor!.children
      act(() => bufferStore.setContent('b1', written))

      expect(capturedEditor!.children).toBe(childrenBefore)
      expect(handleContentChange).toHaveBeenCalledTimes(1)
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

// A raw HTML block is the one place anchors appear inside markup the editor
// holds as an opaque string and cannot annotate, so its links are delegated
// from the block container rather than wired per-anchor. The container is NOT
// itself a control: the anchors inside it are, and they are natively focusable,
// so the delegation is a native listener on the container instead of a React
// `onClick` prop on a static div.
//
// What must hold, whichever way it is wired: a click inside the Tauri WKWebView
// NEVER navigates the app view (there is no browser chrome to catch it — the
// whole app would be replaced by the target page), and an external href reaches
// the OS default browser instead.
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
