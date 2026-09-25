import { describe, expect, it } from 'vitest'
import {
  completionToMonaco,
  documentSymbolsToMonaco,
  hoverToMonaco,
  markerToLspDiagnostic,
  signatureHelpToMonaco,
  toLspPosition,
  toMonacoRange,
  workspaceEditByPath,
} from '@/features/editor/lsp/lsp-to-monaco'

const DEFAULT_RANGE = { startLineNumber: 1, startColumn: 1, endLineNumber: 1, endColumn: 4 }

describe('lsp-to-monaco', () => {
  it('shifts positions between 0-based LSP and 1-based Monaco', () => {
    expect(
      toMonacoRange({ start: { line: 0, character: 4 }, end: { line: 2, character: 0 } }),
    ).toEqual({ startLineNumber: 1, startColumn: 5, endLineNumber: 3, endColumn: 1 })
    expect(toLspPosition({ lineNumber: 10, column: 1 })).toEqual({ line: 9, character: 0 })
  })

  it('maps completion kinds, snippets and text edits', () => {
    const { suggestions, incomplete } = completionToMonaco(
      {
        isIncomplete: true,
        items: [
          { label: 'Println', kind: 3, insertText: 'Println(${1})', insertTextFormat: 2 },
          {
            label: 'fmt',
            kind: 9,
            textEdit: {
              range: { start: { line: 0, character: 0 }, end: { line: 0, character: 2 } },
              newText: 'fmt',
            },
          },
          { label: 'plain' },
        ],
      },
      DEFAULT_RANGE,
    )
    expect(incomplete).toBe(true)
    expect(suggestions[0]).toMatchObject({ kind: 1, insertTextRules: 4, range: DEFAULT_RANGE })
    expect(suggestions[1]).toMatchObject({
      kind: 8,
      insertText: 'fmt',
      range: { startLineNumber: 1, startColumn: 1, endLineNumber: 1, endColumn: 3 },
    })
    expect(suggestions[2]).toMatchObject({ kind: 18, insertText: 'plain' })
    expect(completionToMonaco(null, DEFAULT_RANGE).suggestions).toEqual([])
  })

  it('turns hover contents into markdown and drops empty hovers', () => {
    expect(hoverToMonaco({ contents: { kind: 'markdown', value: '**x**' } })?.contents).toEqual([
      { value: '**x**' },
    ])
    expect(hoverToMonaco({ contents: { language: 'go', value: 'func f()' } })?.contents).toEqual([
      { value: '```go\nfunc f()\n```' },
    ])
    expect(hoverToMonaco({ contents: '' })).toBeNull()
    expect(hoverToMonaco(null)).toBeNull()
  })

  it('keeps signature help active indexes', () => {
    const result = signatureHelpToMonaco({
      signatures: [{ label: 'f(a, b)', parameters: [{ label: [2, 3] }, { label: 'b' }] }],
      activeSignature: 0,
      activeParameter: 1,
    })
    expect(result?.value.activeParameter).toBe(1)
    expect(result?.value.signatures[0]?.parameters[0]?.label).toEqual([2, 3])
    expect(signatureHelpToMonaco({ signatures: [] })).toBeNull()
  })

  it('converts hierarchical and flat document symbols', () => {
    const range = { start: { line: 0, character: 0 }, end: { line: 5, character: 1 } }
    const [nested] = documentSymbolsToMonaco([
      {
        name: 'Server',
        kind: 5,
        range,
        selectionRange: range,
        children: [{ name: 'Run', kind: 6, range, selectionRange: range }],
      },
    ])
    expect(nested).toMatchObject({ name: 'Server', kind: 4 })
    expect(nested?.children?.[0]).toMatchObject({ name: 'Run', kind: 5 })

    const [flat] = documentSymbolsToMonaco([
      { name: 'main', kind: 12, location: { uri: 'file:///x.go', range }, containerName: 'pkg' },
    ])
    expect(flat).toMatchObject({ name: 'main', kind: 11, containerName: 'pkg' })
  })

  it('sends markers back as LSP diagnostics for quick fixes', () => {
    expect(
      markerToLspDiagnostic({
        severity: 8,
        message: 'unused variable',
        startLineNumber: 3,
        startColumn: 2,
        endLineNumber: 3,
        endColumn: 5,
        source: 'compiler',
        code: 'UnusedVar',
      }),
    ).toEqual({
      range: { start: { line: 2, character: 1 }, end: { line: 2, character: 4 } },
      severity: 1,
      message: 'unused variable',
      source: 'compiler',
      code: 'UnusedVar',
    })
  })

  it('groups workspace edits by path from both edit shapes', () => {
    const edit = {
      range: { start: { line: 0, character: 0 }, end: { line: 0, character: 1 } },
      newText: 'y',
    }
    const byPath = workspaceEditByPath({
      changes: { 'a.go': [edit] },
      documentChanges: [{ textDocument: { uri: 'a.go' }, edits: [edit] }, { kind: 'create' }],
    })
    expect(byPath.get('a.go')).toHaveLength(2)
    expect(workspaceEditByPath(null).size).toBe(0)
  })
})
