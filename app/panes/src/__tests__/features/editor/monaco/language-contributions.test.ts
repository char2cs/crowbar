import { describe, it, expect, vi } from 'vitest'
import { editor, languages, Uri } from 'monaco-editor/esm/vs/editor/editor.api.js'
import { langForUri } from '@/features/editor/lib/monaco-adapters'
import { documentUri } from '@/features/editor/documents/document-ref'
import { workspaceRef } from '@/lib/api/documents'
import { toMonacoLanguageId } from '@/features/editor/monaco/language'
import { getAllLanguages } from '@/features/editor/utils/language-id'

describe('language contributions', () => {
  it('colours a model opened before its language was ever asked for', async () => {
    const uri = documentUri(workspaceRef('ws-1', '/repo/script.lua'))
    const model = editor.createModel('local x = 1 -- c', langForUri(uri), Uri.parse(uri))
    await vi.waitFor(() => expect(editor.colorizeModelLine(model, 1)).toMatch(/mtk(?!1\b)\d+/))
    model.dispose()
  })
})

describe('registrations cover every monaco id the app resolver can produce', () => {
  const registered = new Set(languages.getLanguages().map((l) => l.id))

  it('every language the picker offers, but Plain Text, colours through a registered grammar', () => {
    const plain = getAllLanguages()
      .filter(({ id }) => id !== 'text')
      .map(({ id }) => ({ id, monacoId: toMonacoLanguageId(id) }))
      .filter(({ monacoId }) => monacoId === 'plaintext' || !registered.has(monacoId))
    expect(plain).toEqual([])
  })

  it('.jsx and .erb resolve to loadable monaco grammars, not plaintext', () => {
    expect(toMonacoLanguageId('javascriptreact')).toBe('javascript')
    expect(toMonacoLanguageId('embedded_template')).toBe('html')
  })
})
