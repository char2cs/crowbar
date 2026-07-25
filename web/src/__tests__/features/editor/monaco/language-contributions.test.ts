import { describe, it, expect } from 'vitest'
import { languages } from 'monaco-editor/esm/vs/editor/editor.api.js'
import {
  loadLanguageForPath,
  __loadedLanguagesForTests,
  __loaderLanguageIdsForTests,
} from '@/features/editor/monaco/language-contributions'
import {
  MONACO_LANGUAGE_BY_LANGUAGE_ID,
  toMonacoLanguageId,
} from '@/features/editor/monaco/language'
import { __resolverLanguageIdsForTests } from '@/features/editor/utils/language-id'

describe('on-demand language contributions', () => {
  // Documents the "brief plaintext flash" the task brief calls out: a model
  // can be assigned a language id (e.g. 'kotlin', via `langForUri`) BEFORE
  // that language's grammar has finished loading — `languages.register()`
  // hasn't run yet, so monaco renders it unhighlighted until the contribution
  // resolves, at which point it retokenizes existing models of that language
  // id in place. This is real monaco (no mocks), so `languages.getLanguages()`
  // reflects actual registration state before/after the await boundary.
  it('a language is unregistered before its load resolves and registered after', async () => {
    expect(languages.getLanguages().some((l) => l.id === 'kotlin')).toBe(false)
    const pending = loadLanguageForPath('main.kt')
    // Still unregistered synchronously right after kicking off the load —
    // this is the window where a model created with languageId 'kotlin'
    // would render as plaintext (the "brief flash").
    expect(languages.getLanguages().some((l) => l.id === 'kotlin')).toBe(false)
    await pending
    expect(languages.getLanguages().some((l) => l.id === 'kotlin')).toBe(true)
  })

  it('loads a grammar once per language and dedupes', async () => {
    await loadLanguageForPath('main.go')
    await loadLanguageForPath('other.go')
    expect(__loadedLanguagesForTests()).toContain('go')
    expect(__loadedLanguagesForTests().filter((l) => l === 'go')).toHaveLength(1)
  })

  it('unknown extensions resolve without throwing', async () => {
    await expect(loadLanguageForPath('file.xyzunknown')).resolves.toBeUndefined()
  })

  it('loads a distinct language for a different extension (no cross-language bleed)', async () => {
    await loadLanguageForPath('main.rs')
    expect(__loadedLanguagesForTests()).toContain('rust')
  })

  it('concurrent requests for the same never-before-loaded language dedupe in-flight', async () => {
    await Promise.all([loadLanguageForPath('a.swift'), loadLanguageForPath('b.swift')])
    expect(__loadedLanguagesForTests().filter((l) => l === 'swift')).toHaveLength(1)
  })

  it('a dot in a directory segment is not mistaken for an extension', async () => {
    // Naive lastIndexOf('.') over the whole path would extract ".2/readme".
    await expect(loadLanguageForPath('/repo/v1.2/README')).resolves.toBeUndefined()
    await expect(loadLanguageForPath('C:\\repo.v2\\README')).resolves.toBeUndefined()
    // And an extensionless basename resolves as a no-op.
    await expect(loadLanguageForPath('/repo/Makefile')).resolves.toBeUndefined()
    // The basename's own extension still wins when a directory also has a dot.
    await loadLanguageForPath('/repo/v1.2/query.sql')
    expect(__loadedLanguagesForTests()).toContain('sql')
  })
})

// The loader map and the app's own path→monaco-id resolver
// (`MONACO_LANGUAGE_BY_LANGUAGE_ID`, fed through `toMonacoLanguageId` at every
// model-assignment site) are parallel sources of truth. If the resolver can
// hand a model a monaco id that no loader (and no custom Monarch registration)
// ever registers, that language silently renders as plaintext forever. This
// test forces the two files to move together: adding a value to
// `monaco/language.ts` fails here until `language-contributions.ts` grows the
// loader (or the id is consciously added to an exclusion below).
describe('loader map covers every monaco id the app resolver can produce', () => {
  // Registered synchronously by language-contributions.ts's custom Monarch
  // section (ensureLanguage + setMonarchTokensProvider) — no loader needed.
  const CUSTOM_MONARCH_IDS = new Set([
    'diff',
    'gitignore',
    'gitattributes',
    'toml',
    'zig',
    'elm',
    'elisp',
    'lockfile',
    'ocaml',
  ])
  // Deliberately unloadable ids, each with a reason:
  // - plaintext: monaco built-in, needs no grammar by definition.
  const EXCLUDED_IDS = new Set(['plaintext'])

  it('every resolver-producible id has a loader, a Monarch registration, or a documented exclusion', () => {
    const loaderIds = new Set(__loaderLanguageIdsForTests())
    const uncovered = [...new Set(Object.values(MONACO_LANGUAGE_BY_LANGUAGE_ID))].filter(
      (id) => !loaderIds.has(id) && !CUSTOM_MONARCH_IDS.has(id) && !EXCLUDED_IDS.has(id),
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
    const loaderIds = new Set(__loaderLanguageIdsForTests())
    const registerable = (id: string) =>
      loaderIds.has(id) || CUSTOM_MONARCH_IDS.has(id) || EXCLUDED_IDS.has(id)
    const uncovered = [...new Set(__resolverLanguageIdsForTests())]
      .map((languageId) => ({ languageId, monacoId: toMonacoLanguageId(languageId) }))
      .filter(({ monacoId }) => !registerable(monacoId))
    expect(uncovered).toEqual([])
  })

  // Explicit regression pins for the two ids that were rendering as plaintext.
  it('.jsx and .erb resolve to loadable monaco grammars, not plaintext', () => {
    expect(toMonacoLanguageId('javascriptreact')).toBe('javascript')
    expect(toMonacoLanguageId('embedded_template')).toBe('html')
  })
})
