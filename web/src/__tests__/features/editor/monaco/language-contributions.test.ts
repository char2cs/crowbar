import { describe, it, expect, vi } from 'vitest'
import { editor, languages, Uri } from 'monaco-editor/esm/vs/editor/editor.api.js'
import { langForUri } from '@/features/editor/lib/monaco-adapters'
import { documentUri } from '@/features/editor/documents/document-ref'
import { workspaceRef } from '@/lib/api/documents'
import {
  MONACO_LANGUAGE_BY_LANGUAGE_ID,
  toMonacoLanguageId,
} from '@/features/editor/monaco/language'
import { __resolverLanguageIdsForTests } from '@/features/editor/utils/language-id'

describe('language contributions', () => {
  it('colours a model opened before its language was ever asked for', async () => {
    const uri = documentUri(workspaceRef('ws-1', '/repo/script.lua'))
    const model = editor.createModel('local x = 1 -- c', langForUri(uri), Uri.parse(uri))
    // Every span is the default `mtk1` until the language's grammar tokenizes the line.
    await vi.waitFor(() => expect(editor.colorizeModelLine(model, 1)).toMatch(/mtk(?!1\b)\d+/))
    model.dispose()
  })
})

// A monaco id the resolver hands a model that no contribution registers renders as plaintext
// forever, so adding a value to `monaco/language.ts` fails here until a contribution covers it.
describe('registrations cover every monaco id the app resolver can produce', () => {
  const registered = new Set(languages.getLanguages().map((l) => l.id))

  it('every resolver-producible id is registered', () => {
    const uncovered = [...new Set(Object.values(MONACO_LANGUAGE_BY_LANGUAGE_ID))].filter(
      (id) => !registered.has(id),
    )
    expect(uncovered).toEqual([])
  })

  // KEY-side guard: the check above only covers monaco ids the map already
  // names. But `toMonacoLanguageId` falls back to `?? languageId`, so an id
  // the resolver produces that is NOT a key in the map slips through as a
  // verbatim, unregistered monaco id — silent plaintext (this is exactly how
  // `.jsx`→'javascriptreact' and `.erb`→'embedded_template' regressed). Drive
  // every language id `getLanguageIdFromPath` can emit through `toMonacoLanguageId`
  // and assert the result is registerable.
  it('every language id the resolver produces maps to a registerable monaco id (no unregistered ?? fallback)', () => {
    const uncovered = [...new Set(__resolverLanguageIdsForTests())]
      .map((languageId) => ({ languageId, monacoId: toMonacoLanguageId(languageId) }))
      .filter(({ monacoId }) => !registered.has(monacoId))
    expect(uncovered).toEqual([])
  })

  // The two ids that once fell through to plaintext.
  it('.jsx and .erb resolve to loadable monaco grammars, not plaintext', () => {
    expect(toMonacoLanguageId('javascriptreact')).toBe('javascript')
    expect(toMonacoLanguageId('embedded_template')).toBe('html')
  })
})
