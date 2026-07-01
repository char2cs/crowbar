# Crowbar Theme System Redesign — Design

**Date:** 2026-06-14
**Status:** Approved (brainstorm) → ready for implementation plan
**Scope:** Big-bang replacement of the current fragmented color system. No backward-compat / migration code (pre-production, no users).

## Problem

Color in Crowbar today has three independent owners with no shared source of truth, and two of them disagree:

- **App UI** (shadcn tokens) — OKLCH in `web/src/styles/theme.css`.
- **Editor syntax** — defined *twice*: OKLCH `--syntax-*` in `web/src/styles/editor-theme.css` (drives only Tree-sitter CSS classes) **and** hex maps in `web/src/features/settings/lib/appearance-bootstrap.ts` (`CROWBAR_*_SYNTAX`, which is what Monaco actually renders). The two are unsynced.
- **Terminal** (xterm) — hex in `appearance-bootstrap.ts` (`CROWBAR_*_COLORS` `terminal-*`) plus a `getComputedStyle` fallback chain in `web/src/features/terminal/hooks/use-terminal-theme.ts`.

Consequences:
1. **Unreadable syntax.** Monaco renders the hex palette where `keyword`, `boolean`, `constant`, `attribute` all collapse to one terracotta `#d97757`, and `function`/`type`/`tag` to near-identical olives. The palette is the warm Anthropic brand earth-tones used *directly* as code colors — beautiful for chrome, too hue-poor for reading code.
2. **Editing a color often does nothing**, because the stylesheet `--syntax-*` vars are ignored by Monaco (it reads the TS hex copy).
3. **Drift everywhere** — UI, syntax, and terminal colors must be hand-synced across CSS + TS, OKLCH + hex.

## Decisions (locked during brainstorm)

| Decision | Choice |
| --- | --- |
| Single source of truth | **CSS-first** — CSS custom properties are canonical; Monaco/xterm derive from them at runtime. |
| Theme breadth | Make **Crowbar Light + Dark** genuinely good now; keep a clean seam so **installable third-party themes** are a future drop-in (do not build the installer). |
| Syntax model | **Coordinated extension** — bind UI-meaningful roles to shadcn tokens; dedicated hue-separated `--syntax-*` for the rest. |
| Syntax palette direction | **Palette A — "Refined Anthropic"**: keep warm brand identity, terracotta reserved for keywords, distinct cool hues for function/type/constant. |
| Terminal colors | **Fully bound** to the unified token layer — no standalone terminal palette. |

## Architecture

### 1. One canonical token layer — `web/src/styles/theme.css`

All color tokens live here and nowhere else.

- **shadcn semantic tokens** — unchanged (`--background`, `--foreground`, `--primary`, `--muted-foreground`, `--destructive`, `--success`, `--warning`, `--info`, `--accent`, `--border`, …).
- **Bound syntax roles** — CSS aliases so they auto-track the UI theme:
  - `--syntax-comment: var(--muted-foreground)`
  - `--syntax-error: var(--destructive)`
  - `--syntax-variable: var(--foreground)`
  - `--syntax-punctuation: var(--muted-foreground)`
  - `--syntax-operator: var(--muted-foreground)`
- **Dedicated syntax hues** — authored OKLCH, Palette A, with a light value (in the theme selector) and a dark value (under `.dark`), tuned for ≥4.5:1 contrast on `--background`:
  - `--syntax-keyword` (terracotta), `--syntax-string` (sage), `--syntax-number` / `--syntax-constant` (gold / teal), `--syntax-function` (blue), `--syntax-type` (violet), `--syntax-property`, `--syntax-regex`, `--syntax-tag`, `--syntax-attribute`, `--syntax-boolean`, `--syntax-null`, `--syntax-markdown-*`.
- **Terminal ANSI-16** — all CSS aliases onto the unified layer (no independent values):
  - `--terminal-red: var(--destructive)`, `--terminal-green: var(--success)`, `--terminal-yellow: var(--warning)`, `--terminal-blue: var(--info)`
  - `--terminal-black:` dark neutral, `--terminal-white: var(--muted-foreground)`, `--terminal-bright-white: var(--foreground)`
  - **No shadcn magenta/cyan exists**, so: `--terminal-magenta: var(--syntax-type)`, `--terminal-cyan: var(--syntax-constant)` (now part of the same coordinated system).
  - `--terminal-bright-*` alias to the lighter sibling (e.g. `bright-black:` `--muted-foreground`; bright hues reuse the same semantic/syntax token unless a distinct lighter token is warranted).

**Themes are selectors.** Default is `[data-theme="crowbar"]`; `.dark` toggles dark within any theme. A future installed theme is another `[data-theme="…"]` block (+ its `.dark`).

**Deleted big-bang:**
- The rival palette in `web/src/styles/editor-theme.css` (keep only the non-color bits: font import, the `data-pane-resizing` GPU-promote rule).
- `CROWBAR_DARK_COLORS` / `CROWBAR_LIGHT_COLORS` / `CROWBAR_DARK_SYNTAX` / `CROWBAR_LIGHT_SYNTAX` hex maps in `appearance-bootstrap.ts`.
- The `cssVariables` / `syntaxTokens` cache plumbing in `appearance-bootstrap.ts`.

### 2. Runtime color resolver (new) — `web/src/features/editor/theme/resolve-css-color.ts`

The linchpin of CSS-first: Monaco and xterm cannot read CSS vars, so we resolve them to hex at runtime.

- `resolveCssVar(name: string): string | null` — read the value off a probe element via `getComputedStyle`, then normalize through a **canvas `fillStyle`** round-trip to a guaranteed `#rrggbb` (robust OKLCH→hex; verified-capable in WKWebView / recent Safari). Returns `null` if the var is unset/unparseable.
- `readSyntaxPalette(): Record<SyntaxToken, string>` and `readTerminalPalette(): Record<AnsiKey, string>` — iterate the **known typed key lists**, resolving each.
- Results cached; cache invalidated on theme/mode change.

### 3. Consumers — all read the same vars

- **shadcn components & Tailwind** — via the CSS cascade, unchanged.
- **Tree-sitter token CSS** (`web/src/features/editor/styles/token-theme.css`) — `.token-keyword { color: var(--syntax-keyword) }`, etc. Already CSS; just point at the unified vars. Free sync.
- **Monaco** (`web/src/features/editor/monaco/define-theme.ts`, rewritten) — build `ITokenThemeRule[]` from `readSyntaxPalette()`; `base` is `vs` / `vs-dark` by mode; editor bg/fg/selection/line-highlight from resolved shadcn tokens. **No hardcoded hex fallbacks** — a missing token is skipped (Monaco uses its base-theme color) and dev-warned.
- **xterm** (`web/src/features/terminal/hooks/use-terminal-theme.ts`, simplified) — build the `ITheme` from `readTerminalPalette()` + resolved `background`/`foreground`/`cursor`/`selection`. Delete the syntax-fallback chain and the hardcoded `DEFAULT_THEME`.

### 4. Applying a theme

- **Switching a built-in** = set `data-theme` + toggle `.dark`. CSS updates all UI and Tree-sitter instantly — **no JS var-setting, no FOUC** (values ship in bundled CSS).
- **One effect** `useEditorAppearanceSync` watches `data-theme` / `.dark` and, on change, invalidates the resolver cache and rebuilds + applies the Monaco theme (`monaco.editor.defineTheme` + `setTheme`) and the xterm theme (`term.options.theme = …`).
- `appearance-bootstrap.ts` shrinks to: early-set `data-theme` + `.dark` from localStorage (FOUC guard) and fonts. The color cache maps are removed.

### 5. Typed registry — `web/src/extensions/themes`

- `ThemeMeta { id: string; label: string; type: 'light' | 'dark'; dataTheme: string }`.
- A typed list of built-ins — **only Crowbar Light + Dark registered now**. No per-theme color objects (colors live in CSS).
- **Extension seam (documented, not built):** a future installable theme provides (a) a CSS block defining `[data-theme="<id>"]` (+ `.dark`) for the full token set, and (b) a `ThemeMeta` entry. The runtime needs no other changes — the resolver and consumers are theme-agnostic.

## Data flow

```
theme.css vars (canonical)
   ├─(CSS cascade)──────────────► shadcn components, Tailwind, Tree-sitter token classes
   └─(resolver: getComputedStyle ─► canvas normalize ─► #hex)──► Monaco theme, xterm theme  (rebuilt on theme/mode change)
```

## Error handling

- Resolver returns `null` for a missing/unparseable var → token skipped (Monaco/xterm fall back to base-theme color) + dev-only `console.warn`.
- Probe-element and canvas failures are caught; the editor/terminal never crash over a color.
- `getComputedStyle` is read once per rebuild against a single probe, not per-token DOM thrash.

## Testing (mirrored in `web/src/__tests__/`, `@/` imports)

- **Resolver:** `oklch(...)`, `rgb(...)`, `#hex`, and `var()`-alias inputs all normalize to `#rrggbb`; unset var → `null`.
- **Palette readers:** `readSyntaxPalette` / `readTerminalPalette` return every expected key for both light and dark.
- **Contrast guard:** parse `--syntax-*` and `--background` from `theme.css` per mode; assert each syntax foreground ≥ 4.5:1 against the background. Locks in readability and catches regressions.
- **Registry integrity:** every `ThemeMeta.dataTheme` has a matching `[data-theme]` block in CSS; every registered theme defines the full required token set.
- **Monaco builder:** given a known palette, produces the expected `ITokenThemeRule[]` and skips (does not crash on) a missing token.

## Out of scope (deferred)

- The actual theme **install / marketplace** mechanism (seam only).
- Settings-UI changes beyond wiring the existing theme switch to the registry.
- Per-language syntax overrides.

## Files touched (summary)

| File | Change |
| --- | --- |
| `web/src/styles/theme.css` | Add `--syntax-*` (bound + dedicated) and `--terminal-*` aliases; light + `.dark`. Canonical layer. |
| `web/src/styles/editor-theme.css` | Remove the rival color palette; keep font import + `data-pane-resizing` rule. |
| `web/src/features/editor/styles/token-theme.css` | Point `.token-*` classes at unified `--syntax-*`. |
| `web/src/features/editor/theme/resolve-css-color.ts` | **New** resolver + typed palette readers. |
| `web/src/features/editor/monaco/define-theme.ts` | Rewrite to build from resolver; drop hardcoded hex. |
| `web/src/features/terminal/hooks/use-terminal-theme.ts` | Simplify to read `--terminal-*` via resolver; drop fallbacks/DEFAULT_THEME. |
| `web/src/features/settings/lib/appearance-bootstrap.ts` | Strip color/syntax hex maps + cache plumbing; keep data-theme/.dark + fonts. |
| `web/src/extensions/themes/*` | Typed `ThemeMeta` registry; Crowbar Light/Dark only; document extension seam. |
| `web/src/features/.../useEditorAppearanceSync` | **New/relocated** effect: rebuild Monaco + xterm on theme/mode change. |
| `web/src/__tests__/...` | Resolver, palette, contrast-guard, registry, Monaco-builder tests. |
