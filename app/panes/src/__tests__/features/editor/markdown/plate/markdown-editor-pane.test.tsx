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
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { resetWindowPaneStoreForTests } from '@/__tests__/__fixtures__/window-pane-store'
import type { EditorContent } from '@/features/panes/types/pane-content'
import { toast } from '@/features/window/stores/toast-store'
import { recordWorkspaceScope } from '@/lib/workspace-scope'
import {
  loadProjects,
  pairRemoteHost,
  projectOn,
  resetHostWorld,
} from '@/__tests__/__fixtures__/remote-host'
import { resetEntities, seedRepos } from '@/__tests__/__fixtures__/sidebar-entities'
import { useWorkspaceScopeStore } from '@/lib/workspace-scope-store'

const { BUFFER_CONTENT, THROW_SENTINEL } = vi.hoisted(() => {
  const THROW_SENTINEL = '__THROW_ON_DESERIALIZE__'
  return {
    THROW_SENTINEL,
    BUFFER_CONTENT: {
      b1: '# Hello\n\nWorld **bold**.\n',
      'b-table': '| a | b |\n| --- | --- |\n| 1 | 2 |\n',
      b2: '# Second\n\nAnother **document**.\n',
      'b-throw': THROW_SENTINEL,
      'b-empty': '',
      'b-blank': '\n',
      'b-frontmatter':
        '---\ntitle: Plate Live Check\ntags: [verification, markdown]\n---\n\n# Hello\n\nWorld.\n',
      'b-frontmatter-table': '---\ntitle: Table Doc\n---\n| a | b |\n| --- | --- |\n| 1 | 2 |\n',
      'b-html':
        '<div align="center">\n  <h1>Crowbar</h1>\n  <a href="https://example.com/docs">Docs</a>\n</div>\n\n# Heading\n',
      'b-remote-html': '<div>\n  <a href="http://localhost:3000/">Preview</a>\n</div>\n',
      'b-wrapped':
        '# Notes\n\nA _wrapped_ paragraph\nover two lines.\n\n|a|b|\n|-|-|\n|1|2|\n\nLast one.\n',
    } as Record<string, string>,
  }
})

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

const openExternalUrl = vi.fn()
vi.mock('@/lib/external-open', () => ({
  openExternalUrl: (url: string) => openExternalUrl(url),
}))

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
    expect(await screen.findByText('Hello')).toBeInTheDocument()
    expect(screen.getByText('bold')).toBeInTheDocument()
  })

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

  it('does not write on unmount when the editor is pristine, even for a GFM table', async () => {
    const { unmount } = render(<MarkdownEditorPane paneId="p1" bufferId="b-table" />)
    await screen.findByText('a')

    unmount()

    expect(documentText(docOf('b-table'))).toBe(BUFFER_CONTENT['b-table'])
  })

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

  it('does not write on mount+unmount of a pristine frontmatter+table document', async () => {
    const { unmount } = render(<MarkdownEditorPane paneId="p1" bufferId="b-frontmatter-table" />)
    await screen.findByText('a')

    unmount()

    expect(documentText(docOf('b-frontmatter-table'))).toBe(BUFFER_CONTENT['b-frontmatter-table'])
  })

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

      capturedEditor!.children = [
        ...capturedEditor!.children,
        { type: 'p', children: [{ text: 'typed into b2' }] },
      ] as Value

      const b2 = documentText(docOf('b2'))
      expect(b2).toContain('typed into b2')
      expect(b2).toContain('Second')
      expect(b2).not.toContain('Hello')
      expect(documentText(docOf('b1'))).toBe(BUFFER_CONTENT.b1)
    })
  })

  describe('an empty document', () => {
    it('does not write on mount+unmount of an empty file', async () => {
      const { unmount } = render(<MarkdownEditorPane paneId="p1" bufferId="b-empty" />)
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

    it('measures its baseline from the editor children Plate actually synthesized', async () => {
      render(<MarkdownEditorPane paneId="p1" bufferId="b-empty" />)
      await screen.findByText('Write…')

      expect(capturedEditor!.children.length).toBeGreaterThan(0)
      expect(documentText(docOf('b-empty'))).toBe('')
    })
  })

  describe('when the document text changes underneath the editor', () => {
    const reloaded = '# Reloaded\n\nFrom disk.\n'

    it('adopts external text while the editor is pristine', async () => {
      render(<MarkdownEditorPane paneId="p1" bufferId="b1" />)
      await screen.findByText('Hello')

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

      expect(screen.getByText(/title: Checked Out/)).toBeInTheDocument()
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

describe('MarkdownEditorPane raw-HTML anchors', () => {
  it('sends an external raw-HTML link to the OS browser and cancels the webview navigation', async () => {
    render(<MarkdownEditorPane paneId="p1" bufferId="b-html" />)

    const anchor = await screen.findByText('Docs')
    expect(anchor.closest('.markdown-html-block')).not.toBeNull()

    const event = new MouseEvent('click', { bubbles: true, cancelable: true })
    const notCancelled = anchor.dispatchEvent(event)

    expect(notCancelled).toBe(false)
    expect(event.defaultPrevented).toBe(true)
    expect(openExternalUrl).toHaveBeenCalledWith('https://example.com/docs')
  })

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
      useWorkspaceScopeStore.setState(useWorkspaceScopeStore.getInitialState())
    }
  })

  it('leaves clicks elsewhere in the block alone', async () => {
    render(<MarkdownEditorPane paneId="p1" bufferId="b-html" />)

    const heading = await screen.findByText('Crowbar')
    const event = new MouseEvent('click', { bubbles: true, cancelable: true })
    const notCancelled = heading.dispatchEvent(event)

    expect(notCancelled).toBe(true)
    expect(openExternalUrl).not.toHaveBeenCalled()
  })
})
