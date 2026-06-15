# Tree-sitter → Monaco Semantic Highlighting Bridge — Design

**Date:** 2026-06-15
**Status:** Approved (brainstorm) → ready for implementation plan
**Related:** `docs/superpowers/specs/2026-06-14-theme-system-redesign-design.md` (the CSS-first palette this feeds)

## Problem

Crowbar's editor is Monaco, which tokenizes with its built-in Monarch grammars. These are **lexical only**: for most languages every non-keyword identifier — including type names and function names — is tagged as a generic `identifier` and rendered in the plain foreground color. Verified live against the running app on a Go file: keywords (`#b26045`), strings (`#578141`), and operators (muted) are correctly colored by the new palette, but `ResolverFunc`, `Context`, `Namespace`, `Resolve`, `New`, `NewTraversal` all render as plain `#262626`. DOM evidence: **0 Tree-sitter token spans, 159 Monaco `mtk` spans** in the editor. The result reads as flat, near-monochrome code.

The repo already has a Tree-sitter tokenizer (`features/editor/lib/wasm-parser/`, driven by `useTokenizer`) that produces rich, semantically-distinct tokens (function vs. type vs. property vs. variable). But it is wired to *non-Monaco surfaces* (per its own header comment) and never feeds the Monaco editor. No theme change can fix the flatness, because Monaco isn't emitting the tokens to color.

## Goal

Make the Monaco editor render the same rich, semantically-distinct coloring the Tree-sitter tokenizer already computes, using the existing CSS-first `--syntax-*` palette — so the editor and the non-Monaco surfaces finally agree, for every language that has a Tree-sitter grammar.

## Decisions (locked during brainstorm)

| Decision | Choice |
| --- | --- |
| Integration mechanism | A **Document *Range* Semantic Tokens provider**, layered **additively over** Monaco's Monarch base. |
| Registration | **One** generic provider registered against the **`'*'`** language selector. No per-language code or enumeration. |
| Language gating | Inside the provider: if the model's language has a configured Tree-sitter grammar asset, tokenize; otherwise return empty → Monaco keeps its Monarch base. |
| Token source | Reuse the **existing** `tokenizerWorkerClient` (same worker, same caching) the non-Monaco path uses — no second parser. |
| Color source | The existing `--syntax-*` palette via `readSyntaxPalette()`; semantic token types resolve to those colors through the Monaco theme. |
| Rollout | **On by default**, with an editor setting to disable (kill switch). |

## Architecture

### 1. Pure encoder + legend (new, `features/editor/monaco/semantic-tokens-encode.ts`)

The DOM-free, unit-testable core.

- **Legend:** the distinct categories from the existing `CAPTURE_TO_CLASS` map, with the `token-` prefix stripped — `keyword, function, variable, property, constant, number, string, comment, type, attribute, tag, operator, punctuation, text`. Exported as `SEMANTIC_TOKEN_LEGEND = { tokenTypes: [...], tokenModifiers: [] }` plus a `categoryIndex: Record<category, number>`.
- **Capture → category:** reuse `mapCaptureToClass(capture)` (already collapses ~100 captures → ~16 classes), then strip `token-` to get the legend category. One small adapter, no new mapping table.
- **`encodeTokens(highlightTokens: HighlightToken[]): Uint32Array`** — convert the worker's tokens to Monaco's encoded form: 5 ints per token `[deltaLine, deltaStartChar, length, tokenTypeIndex, tokenModifierSet=0]`, each relative to the previous emitted token.
  - **Multi-line splitting (the critical detail):** Monaco semantic tokens are per-line — a token's `length` may not cross a line boundary. A `HighlightToken` whose `startPosition.row !== endPosition.row` (block comments, multi-line strings) is split into one emitted token per line it covers: first line from `startColumn` to end-of-line, full middle lines, last line from column 0 to `endColumn`. End-of-line length for split rows comes from `model.getLineLength(row)`.
  - Assumes the worker's tokens are ordered and non-overlapping (they already feed the line renderer); `dedupeHighlightTokens` covers exact-duplicate ranges. Tokens whose category is `text` (the no-op fallback) are skipped so they inherit the Monarch/foreground base rather than forcing a color.

### 2. The provider (new, `features/editor/monaco/semantic-tokens-provider.ts`)

- Implements `languages.DocumentRangeSemanticTokensProvider`:
  - `getLegend()` → `SEMANTIC_TOKEN_LEGEND`.
  - `provideDocumentRangeSemanticTokens(model, range, token)`:
    1. Derive the Tree-sitter `languageId` from the model's path (`model.uri`) via the **same** `getLanguageIdFromPath` the worker path uses — keeping it consistent with `getLanguageAssetConfig`'s keys (Monaco's `model.getLanguageId()` ids can differ from the tokenizer's).
    2. `getLanguageAssetConfig(languageId)`; if there's no grammar asset → return an empty `SemanticTokens` (`{ data: new Uint32Array(0) }`). Monarch base remains.
    3. Call `tokenizerWorkerClient.tokenize({ bufferId: model.uri.toString(), content: model.getValue(), languageId, wasmPath, highlightQueryUrl, mode: 'range', viewportRange: { startLine: range.startLineNumber-1, endLine: range.endLineNumber-1 } })` (0-based to match the existing hook).
    4. `encodeTokens(result.tokens)` (passing `model` for line lengths) → `{ data, resultId: undefined }`.
  - `releaseDocumentRangeSemanticTokens()` → no-op.
- **Registration:** `languages.registerDocumentRangeSemanticTokensProvider('*', provider)`, called **once** at editor module init (alongside `language-contributions.ts`). Global registration covers every editor instance (surface, diff, satellites) automatically. Guard against double-registration (idempotent init).

### 3. Theme: semantic types → palette colors (`monaco/define-theme.ts`)

Monaco resolves a semantic token's color by matching its legend *type name* against the theme's token `rules` (treated like a TextMate scope). `buildMonacoThemeData` already emits rules for several of these, but not all legend categories line up (e.g. `property`, `punctuation`, `attribute`, `text`). Add an explicit rule for **each** legend category, sourced from `readSyntaxPalette()` (so semantic coloring is decoupled from the Monarch `TOKEN_MAP` scope quirks and always uses the CSS-first palette):

```
property → --syntax-property, punctuation/operator → --syntax-operator (muted),
attribute → --syntax-attribute, number → --syntax-number, tag → --syntax-tag,
function/type/variable/constant/string/comment/keyword → their --syntax-* hue.
```

`text` gets no rule (skipped in the encoder anyway). Existing rules stay for the Monarch base layer.

### 4. Enabling semantic highlighting + the kill switch

- Monaco only applies semantic tokens when the editor option `'semanticHighlighting.enabled'` is on (default is `'configuredByTheme'`, which is unreliable for standalone themes) — so set it **explicitly to `true`** in the construction options of all three editor surfaces (`editor-surface.tsx`, `monaco-diff-editor.tsx`, `use-pane-editor-satellites.ts`), gated by the setting below.
- **Setting:** add a boolean editor setting `semanticHighlighting` (default `true`) to the settings schema, the Appearance/Editor settings tab, and the search index — mirroring the existing `renderIndentGuides`/`highlightOccurrences` pattern. When off, the editor option is set to `false` (Monaco drops the semantic layer; Monarch base remains). Provider registration can stay (it's cheap and inert when the option is off), keeping the toggle a pure per-editor option flip.

## Data flow

```
Monaco viewport range
  → provider.provideDocumentRangeSemanticTokens(model, range)
    → languageId = getLanguageIdFromPath(model.uri);  asset = getLanguageAssetConfig(languageId)
    → (no asset) → empty tokens → Monarch base only
    → tokenizerWorkerClient.tokenize({mode:'range', viewportRange, content, …})   [existing worker + cache]
      → HighlightToken[] (row/col positions, tree-sitter capture names)
        → encodeTokens(): capture→category→legend index, multi-line split, delta-encode → Uint32Array
          → Monaco renders semantic layer OVER Monarch, colored by theme rules ← readSyntaxPalette()
```

## Perf

- **Range provider only** — Monaco requests semantic tokens for the visible viewport (+ small buffer) and re-requests on scroll/edit with its own debounce. This matches the worker's existing range-tokenization strategy; no full-document parse on the UI thread.
- Reuses the worker's per-`bufferId` caching, so the semantic provider and the non-Monaco `useTokenizer` share parse results for the same buffer.
- Additive over Monarch means no loss of the fast sync base; the semantic layer can arrive a frame later without flashing uncolored keywords.

## Error handling

- No grammar asset, worker rejection, or cancellation (`token.isCancellationRequested`) → return empty `SemanticTokens`. Monaco falls back to the Monarch base (today's look). The editor never crashes or blocks over highlighting.
- Encoder is total: unknown captures map to `text` (skipped); malformed positions are clamped to the model's line bounds.

## Known edge cases (record, verify during implementation)

1. **Selector scoring vs. built-ins.** Monaco scores a specific language selector higher than `'*'`. If a built-in (e.g. the TS worker) ever registers its own *document semantic tokens* provider for `typescript`, Monaco would prefer it over ours for that language. In practice the monaco-typescript worker does not provide document semantic tokens, so `'*'` wins everywhere — verify on a `.ts`/`.tsx` file during implementation.
2. **Language-id mismatch.** Monaco's `model.getLanguageId()` may differ from the tokenizer's asset keys; deriving from the path via `getLanguageIdFromPath` avoids this. Verify a few extensions (`.tsx`, `.go`, `.rs`).

## Testing

- **Encoder (unit, pure):** delta encoding for sequential tokens; a multi-line token splits into correct per-line segments with right lengths; `text`/ignored captures are skipped; capture→legend-index mapping for representative captures (`function.call`, `type.builtin`, `variable.parameter`→property/variable, `punctuation.bracket`).
- **Legend/theme coverage:** every legend category has a corresponding rule in `buildMonacoThemeData` output (no semantic type renders uncolored).
- **Provider (unit, mocked worker):** no-grammar language → empty tokens; a stubbed worker result → expected encoded `Uint32Array`; cancellation → empty.
- **Manual (live app):** open Go/TS/Rust files, confirm functions/types/properties pick up `--syntax-*` colors and match the non-Monaco surface; toggle the setting off → reverts to Monarch; large file stays responsive.

## Files touched (summary)

| File | Change |
| --- | --- |
| `features/editor/monaco/semantic-tokens-encode.ts` | **New.** Legend, capture→category adapter, pure `encodeTokens` (+ multi-line split). |
| `features/editor/monaco/semantic-tokens-provider.ts` | **New.** Range provider + idempotent `'*'` registration. |
| `features/editor/monaco/define-theme.ts` | Add a theme rule per legend category, sourced from `readSyntaxPalette()`. |
| `features/editor/components/editor-surface.tsx` | Set `'semanticHighlighting.enabled'` from the setting; ensure provider init runs. |
| `features/editor/components/monaco-diff-editor.tsx` | Same option wiring. |
| `features/editor/hooks/use-pane-editor-satellites.ts` | Same option wiring. |
| editor settings (schema + Appearance/Editor tab + search index) | Add `semanticHighlighting` boolean (default on). |
| `__tests__/features/editor/monaco/*` | Encoder, legend coverage, provider tests. |

## Out of scope (deferred)

- Replacing Monaco's Monarch base or removing the non-Monaco tokenizer surfaces.
- Semantic token **modifiers** (readonly/declaration/etc.) — legend ships with an empty modifier set; can be added later.
- Incremental semantic-token *deltas* (`DocumentSemanticTokensProvider` edits API) — the range provider re-requests per viewport, which is sufficient.
- LSP/gopls semantic tokens — this uses the in-repo Tree-sitter tokenizer, not a language server.
