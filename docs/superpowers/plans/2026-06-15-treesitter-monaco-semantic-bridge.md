# Tree-sitter → Monaco Semantic Highlighting Bridge — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Feed the repo's existing Tree-sitter tokenizer into the Monaco editor via one generic Document Range Semantic Tokens provider, so function/type/property/variable identifiers get colored from the CSS-first `--syntax-*` palette.

**Architecture:** A single `registerDocumentRangeSemanticTokensProvider('*', …)` reuses `tokenizerWorkerClient`, encodes its `HighlightToken[]` into Monaco's semantic-tokens `Uint32Array`, and layers additively over Monarch. Colors resolve through theme rules keyed by the legend's type names. On by default behind an editor setting.

**Tech Stack:** TypeScript, React, Vite, Vitest, monaco-editor, web-tree-sitter (existing worker), Zustand stores.

**Reference spec:** `docs/superpowers/specs/2026-06-15-treesitter-monaco-semantic-bridge-design.md`

---

## File Structure

| File | Responsibility |
| --- | --- |
| `web/src/features/editor/monaco/semantic-tokens-encode.ts` | **New, pure.** Legend, capture→type-index mapping, `encodeTokens` (delta + multi-line split). |
| `web/src/features/editor/monaco/semantic-tokens-provider.ts` | **New.** The range provider, language gating + unsupported cache, idempotent `'*'` registration. |
| `web/src/features/editor/monaco/define-theme.ts` | Add a theme rule per legend type, sourced from `readSyntaxPalette()`. |
| `web/src/features/editor/monaco/language-contributions.ts` | Call the registration once (module side-effect, already imported at startup). |
| `web/src/features/settings/types/settings.ts` · `config/default-settings.ts` · `components/tabs/editor-settings.tsx` · `config/search-index.ts` · `stores/settings-store.ts` | Add the `semanticHighlighting` boolean setting end-to-end. |
| `web/src/features/editor/hooks/use-pane-editor-satellites.ts` · `components/monaco-diff-editor.tsx` | Set `'semanticHighlighting.enabled'` from the setting. |
| `web/src/__tests__/features/editor/monaco/semantic-tokens-encode.test.ts` · `semantic-tokens-provider.test.ts` · `define-theme.test.ts` | Tests. |

**Conventions:** tests under `web/src/__tests__/` mirroring `src/`, `@/` imports. Run commands from `web/`. Single-file test: `npx vitest run <path>`. Typecheck: `npx tsc --noEmit`. Working tree has unrelated pre-existing modified files + parallel-session commits — always `git add` exact paths, never `git add -A`.

---

## Task 1: Legend + capture→type mapping (pure)

**Files:**
- Create: `web/src/features/editor/monaco/semantic-tokens-encode.ts`
- Test: `web/src/__tests__/features/editor/monaco/semantic-tokens-encode.test.ts`

- [ ] **Step 1: Write the failing test**

Create `web/src/__tests__/features/editor/monaco/semantic-tokens-encode.test.ts`:

```ts
import { describe, expect, it } from 'vitest'
import {
  SEMANTIC_TOKEN_TYPES,
  SEMANTIC_TOKEN_LEGEND,
  captureToTypeIndex,
} from '@/features/editor/monaco/semantic-tokens-encode'

describe('semantic token legend', () => {
  it('legend types match the exported list with no modifiers', () => {
    expect(SEMANTIC_TOKEN_LEGEND.tokenTypes).toEqual([...SEMANTIC_TOKEN_TYPES])
    expect(SEMANTIC_TOKEN_LEGEND.tokenModifiers).toEqual([])
  })

  it('maps tree-sitter captures to the right legend index', () => {
    const idx = (t: string) => SEMANTIC_TOKEN_TYPES[captureToTypeIndex(t)]
    expect(idx('function.call')).toBe('function')
    expect(idx('function.method')).toBe('function')
    expect(idx('type.builtin')).toBe('type')
    expect(idx('variable.parameter')).toBe('variable')
    expect(idx('variable.member')).toBe('property')
    expect(idx('constant.numeric')).toBe('number')
    expect(idx('punctuation.bracket')).toBe('punctuation')
    expect(idx('keyword.return')).toBe('keyword')
  })

  it('returns -1 for ignored / text captures', () => {
    expect(captureToTypeIndex('none')).toBe(-1)
    expect(captureToTypeIndex('spell')).toBe(-1)
    expect(captureToTypeIndex('_private')).toBe(-1)
    expect(captureToTypeIndex('totally-unknown-capture')).toBe(-1) // -> token-text -> skipped
  })
})
```

- [ ] **Step 2: Run it to verify it fails**

Run: `cd web && npx vitest run src/__tests__/features/editor/monaco/semantic-tokens-encode.test.ts`
Expected: FAIL — cannot resolve `@/features/editor/monaco/semantic-tokens-encode`.

- [ ] **Step 3: Implement the legend + mapping**

Create `web/src/features/editor/monaco/semantic-tokens-encode.ts`:

```ts
/**
 * Pure encoding of the repo's Tree-sitter highlight tokens into Monaco's
 * semantic-tokens wire format. No DOM, no Monaco — unit-testable.
 */
import { isIgnoredCapture, mapCaptureToClass } from '@/features/editor/lib/wasm-parser/capture-map'
import type { HighlightToken } from '@/features/editor/lib/wasm-parser/types'

/** Legend token types — the categories CAPTURE_TO_CLASS collapses to, minus `text`. */
export const SEMANTIC_TOKEN_TYPES = [
  'keyword',
  'function',
  'variable',
  'property',
  'constant',
  'number',
  'string',
  'comment',
  'type',
  'attribute',
  'tag',
  'operator',
  'punctuation',
] as const
export type SemanticTokenType = (typeof SEMANTIC_TOKEN_TYPES)[number]

export const SEMANTIC_TOKEN_LEGEND = {
  tokenTypes: SEMANTIC_TOKEN_TYPES as unknown as string[],
  tokenModifiers: [] as string[],
}

const TYPE_INDEX: Record<string, number> = Object.fromEntries(
  SEMANTIC_TOKEN_TYPES.map((t, i) => [t, i]),
)

/** tree-sitter capture name -> legend index, or -1 to skip (ignored or `text`). */
export function captureToTypeIndex(capture: string): number {
  if (isIgnoredCapture(capture)) return -1
  const category = mapCaptureToClass(capture).replace(/^token-/, '')
  const idx = TYPE_INDEX[category]
  return idx === undefined ? -1 : idx
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd web && npx vitest run src/__tests__/features/editor/monaco/semantic-tokens-encode.test.ts`
Expected: PASS (3 tests).

- [ ] **Step 5: Commit**

```bash
git add src/features/editor/monaco/semantic-tokens-encode.ts src/__tests__/features/editor/monaco/semantic-tokens-encode.test.ts
git commit -m "feat(editor): semantic-token legend + capture mapping"
```

---

## Task 2: `encodeTokens` — delta encoding + multi-line split (pure)

**Files:**
- Modify: `web/src/features/editor/monaco/semantic-tokens-encode.ts`
- Test: `web/src/__tests__/features/editor/monaco/semantic-tokens-encode.test.ts`

- [ ] **Step 1: Add failing tests**

Append to `web/src/__tests__/features/editor/monaco/semantic-tokens-encode.test.ts`:

```ts
import { encodeTokens } from '@/features/editor/monaco/semantic-tokens-encode'
import type { HighlightToken } from '@/features/editor/lib/wasm-parser/types'

function tok(
  type: string,
  sr: number,
  sc: number,
  er: number,
  ec: number,
): HighlightToken {
  return {
    type,
    startIndex: 0,
    endIndex: 0,
    startPosition: { row: sr, column: sc },
    endPosition: { row: er, column: ec },
  }
}

describe('encodeTokens', () => {
  const fIdx = SEMANTIC_TOKEN_TYPES.indexOf('function')
  const cIdx = SEMANTIC_TOKEN_TYPES.indexOf('comment')

  it('delta-encodes single-line tokens (5 ints each, relative)', () => {
    const data = encodeTokens(
      [tok('function.call', 0, 4, 0, 12), tok('function.call', 0, 20, 0, 23)],
      () => 100,
    )
    expect(Array.from(data)).toEqual([
      0, 4, 8, fIdx, 0, // first: line0 char4 len8
      0, 16, 3, fIdx, 0, // second: same line, deltaChar 20-4=16, len3
    ])
  })

  it('uses absolute char on a new line', () => {
    const data = encodeTokens(
      [tok('function.call', 0, 4, 0, 8), tok('function.call', 2, 2, 2, 5)],
      () => 100,
    )
    expect(Array.from(data)).toEqual([
      0, 4, 4, fIdx, 0,
      2, 2, 3, fIdx, 0, // deltaLine 2 -> deltaChar is absolute (2)
    ])
  })

  it('splits a multi-line token into per-line entries using line lengths', () => {
    // block comment from line 0 col 3 to line 2 col 4; line lengths: 0->10, 1->6, 2->20
    const lens = [10, 6, 20]
    const data = encodeTokens([tok('comment.block', 0, 3, 2, 4)], (row) => lens[row])
    expect(Array.from(data)).toEqual([
      0, 3, 7, cIdx, 0, // line0: col3..10 => len7
      1, 0, 6, cIdx, 0, // line1: full 6
      1, 0, 4, cIdx, 0, // line2: col0..4 => len4 (deltaLine 1)
    ])
  })

  it('skips ignored/text captures and zero-length segments', () => {
    const data = encodeTokens(
      [tok('none', 0, 0, 0, 5), tok('function.call', 1, 3, 1, 3)],
      () => 100,
    )
    expect(data.length).toBe(0)
  })
})
```

- [ ] **Step 2: Run to verify failure**

Run: `cd web && npx vitest run src/__tests__/features/editor/monaco/semantic-tokens-encode.test.ts`
Expected: FAIL — `encodeTokens` not exported.

- [ ] **Step 3: Implement `encodeTokens`**

Append to `web/src/features/editor/monaco/semantic-tokens-encode.ts`:

```ts
/**
 * Encode highlight tokens to Monaco's semantic-tokens Uint32Array:
 * 5 ints/token [deltaLine, deltaStartChar, length, typeIndex, modifierSet=0],
 * each relative to the previously emitted token. Monaco tokens are per-line, so a
 * token spanning rows is split into one entry per covered line.
 * `getLineLength(row)` = length (excl. EOL) of the 0-based row.
 */
export function encodeTokens(
  tokens: HighlightToken[],
  getLineLength: (row: number) => number,
): Uint32Array {
  const data: number[] = []
  let prevLine = 0
  let prevChar = 0

  const push = (line: number, char: number, length: number, typeIndex: number) => {
    if (length <= 0) return
    const deltaLine = line - prevLine
    const deltaChar = deltaLine === 0 ? char - prevChar : char
    data.push(deltaLine, deltaChar, length, typeIndex, 0)
    prevLine = line
    prevChar = char
  }

  for (const t of tokens) {
    const typeIndex = captureToTypeIndex(t.type)
    if (typeIndex < 0) continue

    const { row: startRow, column: startCol } = t.startPosition
    const { row: endRow, column: endCol } = t.endPosition

    if (startRow === endRow) {
      push(startRow, startCol, endCol - startCol, typeIndex)
      continue
    }

    push(startRow, startCol, getLineLength(startRow) - startCol, typeIndex)
    for (let row = startRow + 1; row < endRow; row++) {
      push(row, 0, getLineLength(row), typeIndex)
    }
    push(endRow, 0, endCol, typeIndex)
  }

  return new Uint32Array(data)
}
```

- [ ] **Step 4: Run to verify pass**

Run: `cd web && npx vitest run src/__tests__/features/editor/monaco/semantic-tokens-encode.test.ts`
Expected: PASS (all 7 tests).

- [ ] **Step 5: Typecheck + commit**

```bash
cd web && npx tsc --noEmit 2>&1 | grep semantic-tokens-encode || echo "clean"
git add src/features/editor/monaco/semantic-tokens-encode.ts src/__tests__/features/editor/monaco/semantic-tokens-encode.test.ts
git commit -m "feat(editor): encode tree-sitter tokens to Monaco semantic format"
```

---

## Task 3: The provider + registration

**Files:**
- Create: `web/src/features/editor/monaco/semantic-tokens-provider.ts`
- Test: `web/src/__tests__/features/editor/monaco/semantic-tokens-provider.test.ts`

- [ ] **Step 1: Write the failing test**

Create `web/src/__tests__/features/editor/monaco/semantic-tokens-provider.test.ts`:

```ts
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

vi.mock('@/features/editor/lib/wasm-parser/tokenizer-worker-client', () => ({
  tokenizerWorkerClient: { tokenize: vi.fn() },
}))
vi.mock('@/features/editor/utils/language-id', () => ({
  getLanguageIdFromPath: vi.fn(),
}))

import { tokenizerWorkerClient } from '@/features/editor/lib/wasm-parser/tokenizer-worker-client'
import { getLanguageIdFromPath } from '@/features/editor/utils/language-id'
import { treeSitterSemanticTokensProvider } from '@/features/editor/monaco/semantic-tokens-provider'

const cancel = { isCancellationRequested: false } as any
function model(path: string, value = 'x') {
  return {
    uri: { path, toString: () => `file://${path}` },
    getValue: () => value,
    getLineLength: () => 100,
  } as any
}
const range = { startLineNumber: 1, endLineNumber: 10 } as any

describe('treeSitterSemanticTokensProvider', () => {
  beforeEach(() => vi.clearAllMocks())
  afterEach(() => vi.restoreAllMocks())

  it('returns empty for unknown languages (no tokenize call)', async () => {
    ;(getLanguageIdFromPath as any).mockReturnValue(null)
    const r = await treeSitterSemanticTokensProvider.provideDocumentRangeSemanticTokens(
      model('/x.unknown'),
      range,
      cancel,
    )
    expect(r.data.length).toBe(0)
    expect(tokenizerWorkerClient.tokenize).not.toHaveBeenCalled()
  })

  it('encodes worker tokens for a known language', async () => {
    ;(getLanguageIdFromPath as any).mockReturnValue('go')
    ;(tokenizerWorkerClient.tokenize as any).mockResolvedValue({
      tokens: [
        {
          type: 'function.call',
          startIndex: 0,
          endIndex: 0,
          startPosition: { row: 0, column: 0 },
          endPosition: { row: 0, column: 5 },
        },
      ],
      normalizedText: 'x',
    })
    const r = await treeSitterSemanticTokensProvider.provideDocumentRangeSemanticTokens(
      model('/main.go'),
      range,
      cancel,
    )
    expect(r.data.length).toBe(5)
    expect(tokenizerWorkerClient.tokenize).toHaveBeenCalledOnce()
  })

  it('returns empty and caches the language on worker failure', async () => {
    ;(getLanguageIdFromPath as any).mockReturnValue('zonk')
    ;(tokenizerWorkerClient.tokenize as any).mockRejectedValue(new Error('no wasm'))
    const r1 = await treeSitterSemanticTokensProvider.provideDocumentRangeSemanticTokens(
      model('/a.zonk'),
      range,
      cancel,
    )
    expect(r1.data.length).toBe(0)
    // second call must NOT retry the worker (cached as unsupported)
    const r2 = await treeSitterSemanticTokensProvider.provideDocumentRangeSemanticTokens(
      model('/b.zonk'),
      range,
      cancel,
    )
    expect(r2.data.length).toBe(0)
    expect(tokenizerWorkerClient.tokenize).toHaveBeenCalledOnce()
  })
})
```

- [ ] **Step 2: Run to verify failure**

Run: `cd web && npx vitest run src/__tests__/features/editor/monaco/semantic-tokens-provider.test.ts`
Expected: FAIL — `treeSitterSemanticTokensProvider` not exported.

- [ ] **Step 3: Implement the provider**

Create `web/src/features/editor/monaco/semantic-tokens-provider.ts`:

```ts
/**
 * One generic Document Range Semantic Tokens provider that feeds the editor from
 * the existing Tree-sitter worker. Registered for all languages ('*'); gates
 * internally on whether a grammar exists, and caches languages with no grammar
 * so we don't refetch a missing parser on every viewport request.
 */
import { languages } from 'monaco-editor'
import { getLanguageAssetConfig } from '@/features/editor/lib/wasm-parser/extension-assets'
import { tokenizerWorkerClient } from '@/features/editor/lib/wasm-parser/tokenizer-worker-client'
import { getLanguageIdFromPath } from '@/features/editor/utils/language-id'
import { SEMANTIC_TOKEN_LEGEND, encodeTokens } from './semantic-tokens-encode'

const EMPTY: languages.SemanticTokens = { data: new Uint32Array(0) }
const unsupportedLanguages = new Set<string>()

export const treeSitterSemanticTokensProvider: languages.DocumentRangeSemanticTokensProvider = {
  getLegend: () => SEMANTIC_TOKEN_LEGEND,

  async provideDocumentRangeSemanticTokens(model, range, token) {
    const languageId = getLanguageIdFromPath(model.uri.path)
    if (!languageId || unsupportedLanguages.has(languageId)) return EMPTY

    try {
      const assets = getLanguageAssetConfig(languageId)
      const result = await tokenizerWorkerClient.tokenize({
        bufferId: model.uri.toString(),
        content: model.getValue(),
        languageId,
        wasmPath: assets.wasmPath,
        highlightQueryUrl: assets.highlightQueryUrl,
        mode: 'range',
        viewportRange: {
          startLine: range.startLineNumber - 1,
          endLine: range.endLineNumber - 1,
        },
      })
      if (token.isCancellationRequested) return EMPTY
      return { data: encodeTokens(result.tokens, (row) => model.getLineLength(row + 1)) }
    } catch {
      unsupportedLanguages.add(languageId)
      return EMPTY
    }
  },

  releaseDocumentRangeSemanticTokens() {
    // no-op: range provider holds no per-result state
  },
}

let registered = false
export function registerTreeSitterSemanticTokens(): void {
  if (registered) return
  registered = true
  languages.registerDocumentRangeSemanticTokensProvider('*', treeSitterSemanticTokensProvider)
}
```

- [ ] **Step 4: Run to verify pass**

Run: `cd web && npx vitest run src/__tests__/features/editor/monaco/semantic-tokens-provider.test.ts`
Expected: PASS (3 tests).

- [ ] **Step 5: Register at startup**

In `web/src/features/editor/monaco/language-contributions.ts`, add the import at the top (after the existing `import { languages } from 'monaco-editor'` line) and a registration call at the very end of the file:

```ts
import { registerTreeSitterSemanticTokens } from './semantic-tokens-provider'
```
…and at end of file:
```ts
registerTreeSitterSemanticTokens()
```

- [ ] **Step 6: Typecheck + commit**

```bash
cd web && npx tsc --noEmit 2>&1 | grep -E "semantic-tokens|language-contributions" || echo "clean"
git add src/features/editor/monaco/semantic-tokens-provider.ts src/features/editor/monaco/language-contributions.ts src/__tests__/features/editor/monaco/semantic-tokens-provider.test.ts
git commit -m "feat(editor): tree-sitter semantic tokens provider + registration"
```

---

## Task 4: Theme rules for semantic token types

Monaco colors a semantic token by matching its legend type name against the theme's token rules. Bind each legend type to the CSS-first palette.

**Files:**
- Modify: `web/src/features/editor/monaco/define-theme.ts`
- Test: `web/src/__tests__/features/editor/monaco/define-theme.test.ts`

- [ ] **Step 1: Add a failing test**

Append a test inside the existing `describe('buildMonacoThemeData', …)` block in `web/src/__tests__/features/editor/monaco/define-theme.test.ts`:

```ts
  it('emits a rule for each semantic legend type from the syntax palette', () => {
    const syntax = {
      keyword: '#d97757',
      function: '#6fb0e0',
      type: '#c4a6dd',
      property: '#cfc9bd',
      operator: '#999999',
      punctuation: '#999999',
      attribute: '#d6a95c',
    }
    const { rules } = buildMonacoThemeData({ isDark: true, syntax, ui: UI })
    const fg = (t: string) => rules.find((r) => r.token === t)?.foreground
    expect(fg('function')).toBe('6fb0e0')
    expect(fg('type')).toBe('c4a6dd')
    expect(fg('property')).toBe('cfc9bd') // not present in the Monarch TOKEN_MAP
    expect(fg('punctuation')).toBe('999999') // not present in the Monarch TOKEN_MAP
    expect(fg('attribute')).toBe('d6a95c')
  })
```

- [ ] **Step 2: Run to verify failure**

Run: `cd web && npx vitest run src/__tests__/features/editor/monaco/define-theme.test.ts`
Expected: FAIL — no rule with `token: 'property'` / `'punctuation'`.

- [ ] **Step 3: Add semantic rules in `buildMonacoThemeData`**

In `web/src/features/editor/monaco/define-theme.ts`, inside `buildMonacoThemeData`, right after the existing `const rules: Monaco.editor.ITokenThemeRule[] = TOKEN_MAP.flatMap(...)` block, append:

```ts
  // Semantic-token coloring: Monaco resolves a semantic token's color by matching
  // its legend type name against theme rules. Bind each legend type to the same
  // CSS-first palette (decoupled from the Monarch TOKEN_MAP scope names).
  const SEMANTIC_RULE_KEYS: SyntaxTokenKey[] = [
    'keyword',
    'function',
    'variable',
    'property',
    'constant',
    'number',
    'string',
    'comment',
    'type',
    'attribute',
    'tag',
    'operator',
    'punctuation',
  ]
  for (const key of SEMANTIC_RULE_KEYS) {
    const color = syntax[key]
    if (color) rules.push({ token: key, foreground: stripHash(color) })
  }
```

(`SEMANTIC_RULE_KEYS` mirrors `SEMANTIC_TOKEN_TYPES` in `semantic-tokens-encode.ts`; both are the 13 legend categories.)

- [ ] **Step 4: Run to verify pass**

Run: `cd web && npx vitest run src/__tests__/features/editor/monaco/define-theme.test.ts`
Expected: PASS (all tests, including the new one and the prior bracket/selection cases).

- [ ] **Step 5: Typecheck + commit**

```bash
cd web && npx tsc --noEmit 2>&1 | grep define-theme || echo "clean"
git add src/features/editor/monaco/define-theme.ts src/__tests__/features/editor/monaco/define-theme.test.ts
git commit -m "feat(editor): map semantic token types to the syntax palette"
```

---

## Task 5: Add the `semanticHighlighting` setting (default on)

Mirror the existing `renderIndentGuides` boolean across the persisted settings AND the derived editor-settings store.

**Files:**
- Modify: `web/src/features/settings/types/settings.ts`
- Modify: `web/src/features/settings/config/default-settings.ts`
- Modify: `web/src/features/settings/components/tabs/editor-settings.tsx`
- Modify: `web/src/features/settings/config/search-index.ts`
- Modify: `web/src/features/settings/stores/settings-store.ts`

- [ ] **Step 1: Persisted Settings type + default**

In `web/src/features/settings/types/settings.ts`, in the `Settings` interface next to `renderIndentGuides: boolean` (≈line 28), add:
```ts
  semanticHighlighting: boolean
```
In `web/src/features/settings/config/default-settings.ts`, next to `renderIndentGuides: true,` (≈line 30), add:
```ts
  semanticHighlighting: true,
```

- [ ] **Step 2: Editor-settings store (state, action, initial, sync)**

In `web/src/features/settings/stores/settings-store.ts`:
- In the editor-settings state type, next to `renderIndentGuides: boolean`, add `semanticHighlighting: boolean`.
- In the actions type, next to `setRenderIndentGuides`, add `setSemanticHighlighting: (value: boolean) => void`.
- In the store's initial state, next to `renderIndentGuides: true,`, add `semanticHighlighting: true,`.
- In the actions implementation, next to the `setRenderIndentGuides` setter, add:
  ```ts
  setSemanticHighlighting: (value) => set({ semanticHighlighting: value }),
  ```
- In `syncEditorSettings`, where it destructures settings (includes `renderIndentGuides`), add `semanticHighlighting`, and next to `actions.setRenderIndentGuides(renderIndentGuides)` add:
  ```ts
  actions.setSemanticHighlighting(semanticHighlighting)
  ```

(Match the exact local naming/`set` style already used in this file — follow the `renderIndentGuides` lines verbatim, swapping the name.)

- [ ] **Step 3: Settings UI row**

In `web/src/features/settings/components/tabs/editor-settings.tsx`, immediately after the `renderIndentGuides` `<SettingRow>…</SettingRow>` block (≈lines 153-166), add:
```tsx
<SettingRow
  label="Semantic Highlighting"
  description="Color functions, types, and properties using the Tree-sitter tokenizer"
  onReset={() =>
    updateSetting('semanticHighlighting', getDefaultSetting('semanticHighlighting'))
  }
  canReset={settings.semanticHighlighting !== getDefaultSetting('semanticHighlighting')}
>
  <Switch
    checked={settings.semanticHighlighting}
    onChange={(checked) => updateSetting('semanticHighlighting', checked)}
    size="sm"
  />
</SettingRow>
```

- [ ] **Step 4: Search index entry**

In `web/src/features/settings/config/search-index.ts`, after the `editor-render-indent-guides` entry (≈lines 120-127), add:
```ts
{
  id: 'editor-semantic-highlighting',
  tab: 'editor',
  section: 'Display',
  label: 'Semantic Highlighting',
  description: 'Color functions, types, and properties using the Tree-sitter tokenizer',
  keywords: ['semantic', 'highlighting', 'syntax', 'tree-sitter', 'colors', 'tokens', 'display'],
},
```

- [ ] **Step 5: Typecheck**

Run: `cd web && npx tsc --noEmit; echo "exit: $?"`
Expected: exit 0. (TS will flag every place that constructs a `Settings`/editor-settings object missing the new field — if any beyond the above appear, add `semanticHighlighting` there too, matching `renderIndentGuides`.)

- [ ] **Step 6: Commit**

```bash
git add src/features/settings/types/settings.ts src/features/settings/config/default-settings.ts src/features/settings/components/tabs/editor-settings.tsx src/features/settings/config/search-index.ts src/features/settings/stores/settings-store.ts
git commit -m "feat(settings): add semanticHighlighting editor setting (default on)"
```

---

## Task 6: Wire the editor option from the setting

Turn Monaco's semantic highlighting on (the typed option key is `'semanticHighlighting.enabled'`) from the setting, in the two editor surfaces that build options.

**Files:**
- Modify: `web/src/features/editor/hooks/use-pane-editor-satellites.ts`
- Modify: `web/src/features/editor/components/monaco-diff-editor.tsx`

- [ ] **Step 1: Satellites (live `updateOptions`)**

In `web/src/features/editor/hooks/use-pane-editor-satellites.ts`:
- Next to `const renderIndentGuides = useEditorSettingsStore.use.renderIndentGuides()` (≈line 173), add:
  ```ts
  const semanticHighlighting = useEditorSettingsStore.use.semanticHighlighting()
  ```
- In the `settingsKey` `JSON.stringify([...])` checksum array (≈line 581), add `semanticHighlighting,`.
- In the `editor.updateOptions({...})` object (≈lines 595-620), add:
  ```ts
  'semanticHighlighting.enabled': semanticHighlighting,
  ```
- In that effect's dependency array (≈line 630), add `semanticHighlighting,`.

- [ ] **Step 2: Diff/standalone editor (create + update)**

In `web/src/features/editor/components/monaco-diff-editor.tsx`:
- Next to `const renderIndentGuides = useEditorSettingsStore.use.renderIndentGuides()` (≈line 154), add:
  ```ts
  const semanticHighlighting = useEditorSettingsStore.use.semanticHighlighting()
  ```
- Add `semanticHighlighting` to BOTH the initial `latestEditorSettingsRef` object (≈line 206) and the per-render `latestEditorSettingsRef.current = {…}` update (≈line 225), next to `renderIndentGuides`.
- In the `monacoEditor.create(container, {…})` options (≈lines 285-330), next to the `guides: { … }` block, add:
  ```ts
  'semanticHighlighting.enabled': s.semanticHighlighting,
  ```
- If this component has a live `updateOptions` effect that mirrors settings (search the file for `updateOptions(`), add `'semanticHighlighting.enabled': semanticHighlighting` there too and include `semanticHighlighting` in its dependency array. If there is no such effect (the create-time ref pattern only), no further change is needed.

- [ ] **Step 3: Typecheck**

Run: `cd web && npx tsc --noEmit; echo "exit: $?"`
Expected: exit 0. If TS rejects the `'semanticHighlighting.enabled'` key, confirm the exact key against `node_modules/monaco-editor/esm/vs/editor/common/config/editorOptions.d.ts` (search for `semanticHighlighting`) and use the form it declares; do not guess — match the declared option key.

- [ ] **Step 4: Full suite + commit**

Run: `cd web && npx vitest run 2>&1 | grep -E "Tests +[0-9]+ (passed|failed)" | tail -1`
Expected: all pass.

```bash
git add src/features/editor/hooks/use-pane-editor-satellites.ts src/features/editor/components/monaco-diff-editor.tsx
git commit -m "feat(editor): enable Monaco semantic highlighting from the setting"
```

---

## Task 7: Live verification in the running app

Automated tests can't see the editor render. Verify against the running Tauri app (a `driver_session` is already connected via the Tauri MCP; reconnect with `driver_session status` if needed).

**Files:** none (verification only)

- [ ] **Step 1: Confirm semantic tokens now render**

Open a Go file in the app. Via the Tauri MCP `webview_execute_js`, sample editor token spans (the earlier diagnostic used `.view-line span` + `getComputedStyle().color`). Confirm function/type/property identifiers (`Resolve`, `Context`, `NewTraversal`) are now colored (not `#262626`) and match the `--syntax-*` palette (function ≈ `#6fb0e0` dark / `#2f6fae` light, type ≈ `#c4a6dd` / `#8257a8`). Confirm `[class*="token-"]` is still 0 (we use Monaco semantic tokens, not the `.token-*` DOM path) but the colors are now varied rather than mostly `#262626`.

- [ ] **Step 2: TypeScript interaction (the known edge case)**

Open a `.ts` and a `.tsx` file. Confirm functions/types get semantic colors there too (i.e. our `'*'` provider wins; no built-in TS document-semantic-tokens provider is overriding it). If TS is NOT colored, record it — the fallback is to also register the provider explicitly for `['typescript','typescriptreact']` so it outscores any built-in, but only do that if observed.

- [ ] **Step 3: Toggle the setting**

Settings → Editor → Semantic Highlighting → off. Confirm the editor reverts to the flat Monarch coloring (functions/types back to foreground). Turn it back on → rich colors return.

- [ ] **Step 4: Sanity on a large file**

Open a large source file; scroll. Confirm no perceptible lag and that colors fill in per viewport (range provider) without freezing the UI.

- [ ] **Step 5: Final full checks**

Run: `cd web && npx tsc --noEmit; echo "tsc: $?"` and `npx vitest run 2>&1 | grep -E "Tests +[0-9]+ (passed|failed)" | tail -1`
Expected: tsc exit 0, all tests pass.

---

## Self-Review Notes

- **Spec coverage:** encoder+legend (T1-2), provider+registration+gating+cache (T3), theme mapping (T4), setting end-to-end (T5), editor-option wiring incl. kill switch (T6), live verification incl. the TS-selector edge case (T7). The spec's `getLanguageAssetConfig`-returns-null gating was corrected to a `getLanguageIdFromPath`-null check + unsupported-language cache (T3), since that function never returns null.
- **Type consistency:** `SEMANTIC_TOKEN_TYPES` (T1) ⟷ `SEMANTIC_RULE_KEYS` (T4) are the same 13 categories; `encodeTokens(tokens, getLineLength)` signature is identical across T2/T3; the provider object name `treeSitterSemanticTokensProvider` and `registerTreeSitterSemanticTokens` match between T3 source, T3 test, and the T3 Step-5 registration.
- **Monaco option key:** `'semanticHighlighting.enabled'` is the Monaco-typed sub-option key; T6 Step 3 verifies it against the bundled `editorOptions.d.ts` rather than guessing.
