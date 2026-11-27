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
import { resetWindowPaneStoreForTests } from '@/__tests__/__fixtures__/window-pane-store'
import { applyTextEditsToContent, applyWorkspaceEdit } from '@/features/editor/lsp/workspace-edit'

describe('workspace edit utilities', () => {
  const editAt = (
    start: { line: number; character: number },
    end: { line: number; character: number },
    newText: string,
  ) => ({ range: { start, end }, newText })

  it('converts LSP positions into string offsets', () => {
    expect(
      applyTextEditsToContent('one\ntwo\nthree', [
        editAt({ line: 1, character: 2 }, { line: 1, character: 3 }, 'X'),
      ]),
    ).toBe('one\ntwX\nthree')
  })

  it('clamps LSP positions into the document', () => {
    expect(
      applyTextEditsToContent('one\ntwo', [
        editAt({ line: 1, character: 50 }, { line: 10, character: 5 }, '!'),
      ]),
    ).toBe('one\ntwo!')
    expect(
      applyTextEditsToContent('one\ntwo', [
        editAt({ line: -1, character: -3 }, { line: -1, character: -3 }, '>'),
      ]),
    ).toBe('>one\ntwo')
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
})

describe('applyWorkspaceEdit', () => {
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

  it('decodes a file URI into the filesystem path it writes', async () => {
    const path = '/tmp/hello world.ts'
    readDocument.mockResolvedValueOnce({ ref: workspaceRef('ws-1', path), text: 'a' })

    await applyWorkspaceEdit(
      {
        changes: {
          'file:///tmp/hello%20world.ts': [
            {
              range: { start: { line: 0, character: 0 }, end: { line: 0, character: 1 } },
              newText: 'b',
            },
          ],
        },
      },
      'ws-1',
    )

    expect(writeDocument).toHaveBeenCalledWith(workspaceRef('ws-1', path), 'b')
  })

  it('merges edits from changes and documentChanges into one write per file', async () => {
    const at = (line: number, newText: string) => ({
      range: { start: { line, character: 0 }, end: { line, character: 0 } },
      newText,
    })
    readDocument.mockResolvedValueOnce({ ref: workspaceRef('ws-1', '/tmp/a.ts'), text: 'x\ny' })

    await expect(
      applyWorkspaceEdit(
        {
          changes: { 'file:///tmp/a.ts': [at(0, 'a')] },
          documentChanges: [{ textDocument: { uri: 'file:///tmp/a.ts' }, edits: [at(1, 'b')] }],
        },
        'ws-1',
      ),
    ).resolves.toEqual({ editedFiles: 1 })

    expect(writeDocument).toHaveBeenCalledWith(workspaceRef('ws-1', '/tmp/a.ts'), 'ax\nby')
  })
})
