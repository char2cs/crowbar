import { beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'

const { readDocument, writeDocument } = vi.hoisted(() => ({
  readDocument: vi.fn(),
  writeDocument: vi.fn(),
}))
vi.mock('@/lib/api/documents', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/api/documents')>()),
  readDocument,
  writeDocument,
}))

import { MONACO_LOAD_MS } from '@/__tests__/__fixtures__/documents'
import { workspaceRef, type DocumentRef } from '@/lib/api/documents'
import { documentText } from '@/features/editor/documents/document-text'
import { findDocumentBuffer, openDocument } from '@/features/editor/documents/open-document'
import { loadEditorHost } from '@/features/editor/host/editor-host'
import { resetWindowPaneStoreForTests } from '@/features/panes/stores/window-pane-store'
import {
  applyTextEditsToContent,
  applyWorkspaceEdit,
  collectWorkspaceTextEdits,
  filePathFromUri,
  isWorkspaceEdit,
  offsetFromPosition,
} from '@/features/editor/lsp/workspace-edit'

describe('workspace edit utilities', () => {
  it('decodes file URIs into filesystem paths', () => {
    expect(filePathFromUri('file:///tmp/hello%20world.ts')).toBe('/tmp/hello world.ts')
  })

  it('converts LSP positions into string offsets', () => {
    expect(offsetFromPosition('one\ntwo\nthree', { line: 1, character: 2 })).toBe(6)
  })

  it('clamps LSP positions without rebuilding line arrays', () => {
    expect(offsetFromPosition('one\ntwo', { line: 10, character: 5 })).toBe(7)
    expect(offsetFromPosition('one\ntwo', { line: 1, character: 50 })).toBe(7)
    expect(offsetFromPosition('one\ntwo', { line: -1, character: -3 })).toBe(0)
  })

  it('applies text edits from bottom to top', () => {
    expect(
      applyTextEditsToContent('const one = 1;\nconst two = 2;', [
        {
          range: {
            start: { line: 0, character: 6 },
            end: { line: 0, character: 9 },
          },
          newText: 'first',
        },
        {
          range: {
            start: { line: 1, character: 6 },
            end: { line: 1, character: 9 },
          },
          newText: 'second',
        },
      ]),
    ).toBe('const first = 1;\nconst second = 2;')
  })

  it('applies same-line text edits using original document offsets', () => {
    expect(
      applyTextEditsToContent('0123456789', [
        {
          range: {
            start: { line: 0, character: 2 },
            end: { line: 0, character: 4 },
          },
          newText: 'AA',
        },
        {
          range: {
            start: { line: 0, character: 6 },
            end: { line: 0, character: 8 },
          },
          newText: 'BB',
        },
      ]),
    ).toBe('01AA45BB89')
  })

  it('collects edits from changes and documentChanges', () => {
    const edit = {
      changes: {
        'file:///tmp/a.ts': [
          {
            range: {
              start: { line: 0, character: 0 },
              end: { line: 0, character: 0 },
            },
            newText: 'a',
          },
        ],
      },
      documentChanges: [
        {
          textDocument: { uri: 'file:///tmp/a.ts' },
          edits: [
            {
              range: {
                start: { line: 1, character: 0 },
                end: { line: 1, character: 0 },
              },
              newText: 'b',
            },
          ],
        },
      ],
    }

    expect(isWorkspaceEdit(edit)).toBe(true)
    expect(collectWorkspaceTextEdits(edit).get('/tmp/a.ts')).toHaveLength(2)
  })
})

describe('applyWorkspaceEdit', () => {
  // The documents live in Monaco models: the first load is the slow part, not the tests.
  beforeAll(() => loadEditorHost(), MONACO_LOAD_MS)

  const PATH = '/repo/src/a.ts'
  const rename = {
    changes: {
      [`file://${PATH}`]: [
        {
          range: { start: { line: 0, character: 6 }, end: { line: 0, character: 9 } },
          newText: 'uno',
        },
      ],
    },
  }

  async function open(workspaceId: string, text: string): Promise<DocumentRef> {
    const ref = workspaceRef(workspaceId, PATH)
    readDocument.mockResolvedValueOnce({ ref, text })
    await openDocument(ref)
    return ref
  }

  beforeEach(() => {
    resetWindowPaneStoreForTests()
    readDocument.mockReset()
    writeDocument.mockReset().mockResolvedValue(undefined)
  })

  it('edits an open document in place and leaves it dirty', async () => {
    const ref = await open('ws-1', 'const one = 1')

    await applyWorkspaceEdit(rename, 'ws-1')

    expect(documentText(ref)).toBe('const uno = 1')
    expect(findDocumentBuffer(ref)?.isDirty).toBe(true)
    expect(writeDocument).not.toHaveBeenCalled()
  })

  it('writes a closed file in its own workspace', async () => {
    readDocument.mockResolvedValueOnce({ ref: workspaceRef('ws-1', PATH), text: 'const one = 1' })

    await expect(applyWorkspaceEdit(rename, 'ws-1')).resolves.toEqual({ editedFiles: 1 })

    expect(readDocument).toHaveBeenCalledWith(workspaceRef('ws-1', PATH))
    expect(writeDocument).toHaveBeenCalledWith(workspaceRef('ws-1', PATH), 'const uno = 1')
  })

  it("leaves another workspace's buffer for the same path untouched", async () => {
    const sibling = await open('ws-2', 'const one = 1')
    readDocument.mockResolvedValueOnce({ ref: workspaceRef('ws-1', PATH), text: 'const one = 1' })

    await applyWorkspaceEdit(rename, 'ws-1')

    expect(documentText(sibling)).toBe('const one = 1')
    expect(findDocumentBuffer(sibling)?.isDirty).toBe(false)
    expect(writeDocument).toHaveBeenCalledWith(workspaceRef('ws-1', PATH), 'const uno = 1')
  })
})
