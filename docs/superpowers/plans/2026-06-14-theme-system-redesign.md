# Theme System Redesign Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make Crowbar's editor + terminal colors derive from one CSS-first token layer, fixing the unreadable, drifting dual-palette syntax highlighting, with a genuinely better Crowbar Light + Dark.

**Architecture:** `web/src/styles/theme.css` becomes the single source of truth: shadcn tokens + bound `--syntax-*` aliases + dedicated `--syntax-*` hues (Palette A) + `--terminal-*` aliases, in `:root` (light) and `.dark`. A new runtime resolver (`resolve-css-color.ts`) reads those vars off `document.documentElement` and converts OKLCH/rgb/hex → `#hex`. Monaco and xterm theme builders are rewritten to build from the resolver. The existing `themeRegistry.onThemeChange` subscriptions already re-invoke the builders on theme/mode change, so no new sync wiring is needed. The registry and bootstrap are stripped of their now-dead color-map machinery.

**Tech Stack:** TypeScript, React, Vite, Vitest, monaco-editor, xterm, Tailwind v4 + shadcn CSS variables (OKLCH).

**Reference spec:** `docs/superpowers/specs/2026-06-14-theme-system-redesign-design.md`

---

## File Structure

| File | Responsibility after this plan |
| --- | --- |
| `web/src/styles/theme.css` | **Canonical token layer.** Adds full `--syntax-*` (bound + dedicated, light + dark) and `--terminal-*` aliases. |
| `web/src/styles/editor-theme.css` | Keeps `--editor-font-family`, the JetBrains font import, and the `data-pane-resizing` GPU rule. **Loses** its rival `--syntax-*` palette. |
| `web/src/features/editor/styles/token-theme.css` | Unchanged behavior — already consumes `--syntax-*`. Only the leading comment is corrected. |
| `web/src/features/editor/theme/resolve-css-color.ts` | **New.** Color converter (`cssColorToHex`) + DOM resolver (`resolveCssVar`, `readSyntaxPalette`, `readTerminalPalette`). |
| `web/src/features/editor/monaco/define-theme.ts` | Rewritten: pure `buildMonacoThemeData()` + `defineMonacoTheme()` that builds from the resolver. No hardcoded hex, no registry token reads. |
| `web/src/features/terminal/hooks/use-terminal-theme.ts` | Rewritten: pure `buildTerminalTheme()` + `getTerminalTheme()` reading `--terminal-*` via resolver. No `DEFAULT_THEME` fallback chain. |
| `web/src/features/settings/lib/appearance-bootstrap.ts` | Stripped of `CROWBAR_*_COLORS`/`CROWBAR_*_SYNTAX` and the `cssVariables`/`syntaxTokens` cache. Keeps `data-theme`/`.dark` + fonts. |
| `web/src/extensions/themes/theme-registry.ts` | Stripped of `dualModeVariants`/color maps. `applyTheme` sets `data-theme`, syncs `.dark`, notifies listeners. Public API unchanged. |
| `web/src/__tests__/features/editor/theme/resolve-css-color.test.ts` | New tests for converter + readers. |
| `web/src/__tests__/features/editor/monaco/define-theme.test.ts` | New tests for `buildMonacoThemeData`. |
| `web/src/__tests__/features/terminal/use-terminal-theme.test.ts` | New tests for `buildTerminalTheme`. |
| `web/src/__tests__/styles/theme-tokens.test.ts` | New: token-presence + contrast guard parsing `theme.css`. |

**Conventions (from CLAUDE.md):** tests live under `web/src/__tests__/` mirroring `src/`, use `@/` imports. Component/file names kebab-case.

**Commands:** run from `web/`. Test a single file: `npx vitest run <path>`. Typecheck: `npx tsc --noEmit`.

---

## Task 1: Canonical token layer in `theme.css`

Move all syntax + terminal colors into `theme.css` as the single source of truth, and delete the rival palette in `editor-theme.css`.

**Files:**
- Modify: `web/src/styles/theme.css` (append to the existing `:root` block at ~line 230 and the `.dark` block at ~line 278)
- Modify: `web/src/styles/editor-theme.css` (remove the `--syntax-*` declarations)
- Modify: `web/src/features/editor/styles/token-theme.css:1-2` (fix comment)

- [ ] **Step 1: Add the syntax + terminal tokens to `theme.css` `:root`**

In `web/src/styles/theme.css`, inside the existing `:root { … }` block (just before its closing `}` at ~line 231), add:

```css
  /* ── Syntax tokens — light mode ───────────────────────────────── */
  /* Bound to shadcn semantics (auto-track the UI theme) */
  --syntax-comment: var(--muted-foreground);
  --syntax-variable: var(--foreground);
  --syntax-punctuation: var(--muted-foreground);
  --syntax-operator: var(--muted-foreground);
  --syntax-error: var(--destructive);

  /* Dedicated hues — Palette A "Refined Anthropic" (light) */
  --syntax-keyword: #be664a;
  --syntax-string: #5e8b46;
  --syntax-number: #a67a35;
  --syntax-constant: #3a8a92;
  --syntax-function: #2f6fae;
  --syntax-type: #8257a8;
  --syntax-property: #5a564d;
  --syntax-tag: #2f6fae;
  --syntax-attribute: #a67a35;
  --syntax-boolean: #3a8a92;
  --syntax-null: #8257a8;
  --syntax-regex: #3a8a92;
  --syntax-jsx: #2f6fae;
  --syntax-jsx-attribute: #a67a35;
  --syntax-markdown-heading: #2f6fae;
  --syntax-markdown-bold: #a67a35;
  --syntax-markdown-italic: #be664a;
  --syntax-markdown-strikethrough: var(--muted-foreground);
  --syntax-markdown-link: #2f6fae;
  --syntax-markdown-link-text: #5e8b46;
  --syntax-markdown-code: #5e8b46;
  --syntax-markdown-list: #be664a;
  --syntax-markdown-quote: var(--muted-foreground);

  /* ── Terminal ANSI-16 — pure aliases onto the unified layer ───── */
  /* magenta/cyan have no shadcn equivalent → reuse coordinated syntax hues */
  --terminal-black: oklch(0.27 0 0);
  --terminal-red: var(--destructive);
  --terminal-green: var(--success);
  --terminal-yellow: var(--warning);
  --terminal-blue: var(--info);
  --terminal-magenta: var(--syntax-type);
  --terminal-cyan: var(--syntax-constant);
  --terminal-white: var(--muted-foreground);
  --terminal-bright-black: var(--muted-foreground);
  --terminal-bright-red: var(--destructive);
  --terminal-bright-green: var(--success);
  --terminal-bright-yellow: var(--warning);
  --terminal-bright-blue: var(--info);
  --terminal-bright-magenta: var(--syntax-type);
  --terminal-bright-cyan: var(--syntax-constant);
  --terminal-bright-white: var(--foreground);
```

- [ ] **Step 2: Add the dark dedicated hues to `theme.css` `.dark`**

Inside the existing `.dark { … }` block (just before its closing `}` at ~line 279), add (only the dedicated hues need a dark override — the bound + terminal aliases follow their referenced tokens automatically):

```css
  /* ── Syntax dedicated hues — Palette A (dark) ─────────────────── */
  --syntax-keyword: #d97757;
  --syntax-string: #a3c585;
  --syntax-number: #d6a95c;
  --syntax-constant: #5fbcc4;
  --syntax-function: #6fb0e0;
  --syntax-type: #c4a6dd;
  --syntax-property: #cfc9bd;
  --syntax-tag: #6fb0e0;
  --syntax-attribute: #d6a95c;
  --syntax-boolean: #5fbcc4;
  --syntax-null: #c4a6dd;
  --syntax-regex: #7ec0cf;
  --syntax-jsx: #6fb0e0;
  --syntax-jsx-attribute: #d6a95c;
  --syntax-markdown-heading: #6fb0e0;
  --syntax-markdown-bold: #d6a95c;
  --syntax-markdown-italic: #d97757;
  --syntax-markdown-link: #6fb0e0;
  --syntax-markdown-link-text: #a3c585;
  --syntax-markdown-code: #a3c585;
  --syntax-markdown-list: #d97757;
```

(`--syntax-markdown-strikethrough` and `--syntax-markdown-quote` stay bound to `--muted-foreground` from `:root`, so they are intentionally not repeated here.)

- [ ] **Step 3: Remove the rival palette from `editor-theme.css`**

In `web/src/styles/editor-theme.css`, delete the two `--syntax-*` groups: lines `6-33` (the light block inside `:root`) and the entire `.dark { … }` block at lines `36-65`. Keep line 4 (`--editor-font-family`), the font `@import` (line 1), and the `html[data-pane-resizing] .monaco-editor` rule (lines 67-87). After editing, `:root` should contain only `--editor-font-family`, and there should be no `.dark` block in this file.

- [ ] **Step 4: Fix the stale comment in `token-theme.css`**

In `web/src/features/editor/styles/token-theme.css`, change line 2 from:

```css
/* Uses --syntax-* variables declared in index.css */
```
to:
```css
/* Uses --syntax-* variables declared in styles/theme.css */
```

- [ ] **Step 5: Verify the build still compiles the CSS**

Run: `cd web && npx vite build --mode development 2>&1 | tail -20`
Expected: build completes without "Unexpected" / CSS parse errors. (A full prod build is not required; this just confirms the CSS is valid.)

- [ ] **Step 6: Commit**

```bash
git add web/src/styles/theme.css web/src/styles/editor-theme.css web/src/features/editor/styles/token-theme.css
git commit -m "feat(theme): make theme.css the canonical syntax + terminal token layer"
```

---

## Task 2: Color converter (`cssColorToHex`)

Pure, DOM-free conversion of any token value (`oklch()`, `rgb()/rgba()`, `#hex`/`#rgb`) to `#rrggbb`/`#rrggbbaa`. This is what makes CSS-first feed Monaco/xterm.

**Files:**
- Create: `web/src/features/editor/theme/resolve-css-color.ts`
- Test: `web/src/__tests__/features/editor/theme/resolve-css-color.test.ts`

- [ ] **Step 1: Write the failing test**

Create `web/src/__tests__/features/editor/theme/resolve-css-color.test.ts`:

```ts
import { describe, expect, it } from 'vitest'
import { cssColorToHex } from '@/features/editor/theme/resolve-css-color'

describe('cssColorToHex', () => {
  it('passes through and expands hex', () => {
    expect(cssColorToHex('#aabbcc')).toBe('#aabbcc')
    expect(cssColorToHex('#abc')).toBe('#aabbcc')
    expect(cssColorToHex('  #ABCDEF ')).toBe('#abcdef')
  })

  it('converts rgb/rgba', () => {
    expect(cssColorToHex('rgb(255, 0, 0)')).toBe('#ff0000')
    expect(cssColorToHex('rgba(0, 0, 0, 0.5)')).toBe('#00000080')
  })

  it('converts oklch endpoints exactly', () => {
    expect(cssColorToHex('oklch(1 0 0)')).toBe('#ffffff')
    expect(cssColorToHex('oklch(0 0 0)')).toBe('#000000')
  })

  it('handles oklch alpha', () => {
    expect(cssColorToHex('oklch(1 0 0 / 50%)')).toBe('#ffffff80')
  })

  it('converts a known chromatic oklch within tolerance of sRGB red', () => {
    // oklch for #ff0000 ≈ L0.6279 C0.2577 H29.23
    const hex = cssColorToHex('oklch(0.6279 0.2577 29.23)')
    expect(hex.slice(0, 1)).toBe('#')
    const r = Number.parseInt(hex.slice(1, 3), 16)
    expect(r).toBeGreaterThan(250) // strong red channel
  })

  it('returns null for unparseable input', () => {
    expect(cssColorToHex('')).toBeNull()
    expect(cssColorToHex('not-a-color')).toBeNull()
  })
})
```

- [ ] **Step 2: Run it to verify it fails**

Run: `cd web && npx vitest run src/__tests__/features/editor/theme/resolve-css-color.test.ts`
Expected: FAIL — cannot resolve `@/features/editor/theme/resolve-css-color`.

- [ ] **Step 3: Implement the converter**

Create `web/src/features/editor/theme/resolve-css-color.ts`:

```ts
/**
 * CSS-first color resolution. The canonical color values live in theme.css as
 * CSS custom properties (OKLCH / rgb / hex). Monaco and xterm cannot read CSS
 * variables, so we resolve them off the DOM and convert to #hex here.
 */

function clamp255(n: number): number {
  return Math.max(0, Math.min(255, Math.round(n)))
}

function toHexByte(n: number): string {
  return clamp255(n).toString(16).padStart(2, '0')
}

function expandShortHex(hex: string): string {
  if (hex.length === 4) {
    const [, r, g, b] = hex
    return `#${r}${r}${g}${g}${b}${b}`
  }
  return hex
}

function gammaEncode(c: number): number {
  const v = c <= 0.0031308 ? 12.92 * c : 1.055 * c ** (1 / 2.4) - 0.055
  return v * 255
}

/** OKLCH → sRGB hex. Math per Björn Ottosson's OKLab reference. */
function oklchToHex(l: number, c: number, hDeg: number, alpha: number): string {
  const h = (hDeg * Math.PI) / 180
  const a = c * Math.cos(h)
  const b = c * Math.sin(h)

  const l_ = l + 0.3963377774 * a + 0.2158037573 * b
  const m_ = l - 0.1055613458 * a - 0.0638541728 * b
  const s_ = l - 0.0894841775 * a - 1.291485548 * b

  const lc = l_ ** 3
  const mc = m_ ** 3
  const sc = s_ ** 3

  const r = 4.0767416621 * lc - 3.3077115913 * mc + 0.2309699292 * sc
  const g = -1.2684380046 * lc + 2.6097574011 * mc - 0.3413193965 * sc
  const bl = -0.0041960863 * lc - 0.7034186147 * mc + 1.707614701 * sc

  const hex = `#${toHexByte(gammaEncode(r))}${toHexByte(gammaEncode(g))}${toHexByte(gammaEncode(bl))}`
  return alpha >= 1 ? hex : `${hex}${toHexByte(alpha * 255)}`
}

function parseAlpha(raw: string | undefined): number {
  if (!raw) return 1
  const t = raw.trim()
  if (t.endsWith('%')) return Number.parseFloat(t) / 100
  return Number.parseFloat(t)
}

/**
 * Convert a CSS color string to #rrggbb (or #rrggbbaa when alpha < 1).
 * Supports the formats used in theme.css: #hex/#rgb, rgb()/rgba(), oklch().
 * Returns null if the value is empty or unrecognized.
 */
export function cssColorToHex(value: string): string | null {
  const v = value.trim().toLowerCase()
  if (!v) return null

  if (/^#[0-9a-f]{3}$/.test(v)) return expandShortHex(v)
  if (/^#[0-9a-f]{6}([0-9a-f]{2})?$/.test(v)) return v

  const rgb = v.match(
    /^rgba?\(\s*([\d.]+)[\s,]+([\d.]+)[\s,]+([\d.]+)(?:[\s,/]+([\d.]+%?))?\s*\)$/,
  )
  if (rgb) {
    const [, r, g, b, a] = rgb
    const alpha = parseAlpha(a)
    const hex = `#${toHexByte(Number(r))}${toHexByte(Number(g))}${toHexByte(Number(b))}`
    return alpha >= 1 ? hex : `${hex}${toHexByte(alpha * 255)}`
  }

  const oklch = v.match(
    /^oklch\(\s*([\d.]+%?)\s+([\d.]+)\s+([\d.]+)(?:\s*\/\s*([\d.]+%?))?\s*\)$/,
  )
  if (oklch) {
    const [, lRaw, cRaw, hRaw, aRaw] = oklch
    const l = lRaw.endsWith('%') ? Number.parseFloat(lRaw) / 100 : Number.parseFloat(lRaw)
    return oklchToHex(l, Number(cRaw), Number(hRaw), parseAlpha(aRaw))
  }

  return null
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd web && npx vitest run src/__tests__/features/editor/theme/resolve-css-color.test.ts`
Expected: PASS (all 6 tests).

- [ ] **Step 5: Commit**

```bash
git add web/src/features/editor/theme/resolve-css-color.ts web/src/__tests__/features/editor/theme/resolve-css-color.test.ts
git commit -m "feat(theme): add cssColorToHex converter (oklch/rgb/hex -> hex)"
```

---

## Task 3: DOM resolver + typed palette readers

Read the live CSS vars off `document.documentElement` and return typed `#hex` maps for syntax and terminal.

**Files:**
- Modify: `web/src/features/editor/theme/resolve-css-color.ts`
- Test: `web/src/__tests__/features/editor/theme/resolve-css-color.test.ts`

- [ ] **Step 1: Add failing tests for the readers**

Append to `web/src/__tests__/features/editor/theme/resolve-css-color.test.ts`:

```ts
import { afterEach, beforeEach, vi } from 'vitest'
import {
  SYNTAX_TOKEN_KEYS,
  TERMINAL_ANSI_KEYS,
  readSyntaxPalette,
  readTerminalPalette,
  resolveCssVar,
} from '@/features/editor/theme/resolve-css-color'

describe('DOM resolver', () => {
  beforeEach(() => {
    // Map every CSS var this suite asks for to a known oklch value.
    vi.spyOn(window, 'getComputedStyle').mockReturnValue({
      getPropertyValue: (name: string) => (name.startsWith('--') ? 'oklch(1 0 0)' : ''),
    } as unknown as CSSStyleDeclaration)
  })
  afterEach(() => vi.restoreAllMocks())

  it('resolveCssVar converts the computed value to hex', () => {
    expect(resolveCssVar('--syntax-keyword')).toBe('#ffffff')
  })

  it('resolveCssVar returns null for an unset var', () => {
    vi.spyOn(window, 'getComputedStyle').mockReturnValue({
      getPropertyValue: () => '',
    } as unknown as CSSStyleDeclaration)
    expect(resolveCssVar('--nope')).toBeNull()
  })

  it('readSyntaxPalette returns a hex for every syntax key', () => {
    const palette = readSyntaxPalette()
    for (const key of SYNTAX_TOKEN_KEYS) {
      expect(palette[key]).toBe('#ffffff')
    }
  })

  it('readTerminalPalette returns a hex for every ANSI key', () => {
    const palette = readTerminalPalette()
    for (const key of TERMINAL_ANSI_KEYS) {
      expect(palette[key]).toBe('#ffffff')
    }
  })
})
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd web && npx vitest run src/__tests__/features/editor/theme/resolve-css-color.test.ts`
Expected: FAIL — `readSyntaxPalette` etc. not exported.

- [ ] **Step 3: Implement the resolver + readers**

Append to `web/src/features/editor/theme/resolve-css-color.ts`:

```ts
/** Syntax token keys → their CSS variable is `--syntax-<key>`. */
export const SYNTAX_TOKEN_KEYS = [
  'keyword', 'string', 'number', 'constant', 'comment', 'variable', 'property',
  'type', 'function', 'operator', 'punctuation', 'tag', 'attribute', 'boolean',
  'null', 'regex', 'jsx', 'jsx-attribute', 'error',
  'markdown-heading', 'markdown-bold', 'markdown-italic', 'markdown-strikethrough',
  'markdown-link', 'markdown-link-text', 'markdown-code', 'markdown-list', 'markdown-quote',
] as const
export type SyntaxTokenKey = (typeof SYNTAX_TOKEN_KEYS)[number]

/** ANSI keys → their CSS variable is `--terminal-<key>`. */
export const TERMINAL_ANSI_KEYS = [
  'black', 'red', 'green', 'yellow', 'blue', 'magenta', 'cyan', 'white',
  'bright-black', 'bright-red', 'bright-green', 'bright-yellow',
  'bright-blue', 'bright-magenta', 'bright-cyan', 'bright-white',
] as const
export type TerminalAnsiKey = (typeof TERMINAL_ANSI_KEYS)[number]

/** Resolve a single CSS variable on <html> to #hex, or null if unset/unparseable. */
export function resolveCssVar(name: string, el: Element = document.documentElement): string | null {
  const raw = getComputedStyle(el).getPropertyValue(name)
  return cssColorToHex(raw)
}

function readPalette<K extends string>(keys: readonly K[], prefix: string): Record<K, string> {
  const out = {} as Record<K, string>
  for (const key of keys) {
    const hex = resolveCssVar(`${prefix}${key}`)
    if (hex) out[key] = hex
  }
  return out
}

export function readSyntaxPalette(): Partial<Record<SyntaxTokenKey, string>> {
  return readPalette(SYNTAX_TOKEN_KEYS, '--syntax-')
}

export function readTerminalPalette(): Partial<Record<TerminalAnsiKey, string>> {
  return readPalette(TERMINAL_ANSI_KEYS, '--terminal-')
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd web && npx vitest run src/__tests__/features/editor/theme/resolve-css-color.test.ts`
Expected: PASS (all tests, including Task 2's).

- [ ] **Step 5: Commit**

```bash
git add web/src/features/editor/theme/resolve-css-color.ts web/src/__tests__/features/editor/theme/resolve-css-color.test.ts
git commit -m "feat(theme): add DOM resolver + typed syntax/terminal palette readers"
```

---

## Task 4: Rewrite the Monaco theme builder

Build Monaco token rules from the resolved syntax palette + resolved shadcn UI tokens. Keep `defineMonacoTheme(themeId)`'s signature so the 3 callers (`editor-surface.tsx`, `monaco-diff-editor.tsx`, `use-pane-editor-satellites.ts`) need no change.

**Files:**
- Modify: `web/src/features/editor/monaco/define-theme.ts`
- Test: `web/src/__tests__/features/editor/monaco/define-theme.test.ts`

- [ ] **Step 1: Write the failing test for the pure builder**

Create `web/src/__tests__/features/editor/monaco/define-theme.test.ts`:

```ts
import { describe, expect, it } from 'vitest'
import { buildMonacoThemeData } from '@/features/editor/monaco/define-theme'

const SYNTAX = {
  keyword: '#d97757',
  string: '#a3c585',
  function: '#6fb0e0',
  comment: '#999999',
}

const UI = {
  background: '#1f1f1f',
  foreground: '#f5f5f5',
  selection: '#33445566',
  border: '#2a2a2a',
  subtle: '#888888',
}

describe('buildMonacoThemeData', () => {
  it('uses vs-dark base in dark mode and vs in light', () => {
    expect(buildMonacoThemeData({ isDark: true, syntax: SYNTAX, ui: UI }).base).toBe('vs-dark')
    expect(buildMonacoThemeData({ isDark: false, syntax: SYNTAX, ui: UI }).base).toBe('vs')
  })

  it('maps syntax tokens to rules without the leading #', () => {
    const { rules } = buildMonacoThemeData({ isDark: true, syntax: SYNTAX, ui: UI })
    const keyword = rules.find((r) => r.token === 'keyword')
    expect(keyword?.foreground).toBe('d97757')
  })

  it('skips tokens that are missing from the palette (no crash)', () => {
    const { rules } = buildMonacoThemeData({ isDark: true, syntax: { keyword: '#d97757' }, ui: UI })
    expect(rules.some((r) => r.token === 'string')).toBe(false)
    expect(rules.some((r) => r.token === 'keyword')).toBe(true)
  })

  it('sets editor background/foreground from UI tokens', () => {
    const { colors } = buildMonacoThemeData({ isDark: true, syntax: SYNTAX, ui: UI })
    expect(colors['editor.background']).toBe('#1f1f1f')
    expect(colors['editor.foreground']).toBe('#f5f5f5')
  })
})
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd web && npx vitest run src/__tests__/features/editor/monaco/define-theme.test.ts`
Expected: FAIL — `buildMonacoThemeData` not exported.

- [ ] **Step 3: Rewrite `define-theme.ts`**

Replace the entire contents of `web/src/features/editor/monaco/define-theme.ts` with:

```ts
/**
 * Builds + registers a Monaco theme from the live CSS token layer (theme.css).
 * CSS-first: every color is resolved off <html> at call time, so the Monaco
 * theme always matches whatever .dark / [data-theme] is currently applied.
 */

import { editor as monacoEditor } from 'monaco-editor'
import type * as Monaco from 'monaco-editor'
import {
  readSyntaxPalette,
  resolveCssVar,
  type SyntaxTokenKey,
} from '@/features/editor/theme/resolve-css-color'

/** Monaco token scope → our syntax palette key. */
const TOKEN_MAP: Array<[monacoToken: string, syntaxKey: SyntaxTokenKey]> = [
  ['comment', 'comment'],
  ['keyword', 'keyword'],
  ['string', 'string'],
  ['number', 'number'],
  ['regexp', 'regex'],
  ['function', 'function'],
  ['variable', 'variable'],
  ['constant', 'constant'],
  ['type', 'type'],
  ['class', 'type'],
  ['interface', 'type'],
  ['namespace', 'type'],
  ['tag', 'tag'],
  ['attribute.name', 'attribute'],
  ['delimiter', 'punctuation'],
  ['delimiter.bracket', 'punctuation'],
  ['operator', 'operator'],
  ['keyword.operator', 'operator'],
  ['keyword.json', 'property'],
  ['string.key.json', 'property'],
]

function stripHash(value: string): string {
  return value.startsWith('#') ? value.slice(1) : value
}

export interface MonacoUiTokens {
  background: string
  foreground: string
  selection: string
  border: string
  subtle: string
}

export interface MonacoThemeInput {
  isDark: boolean
  syntax: Partial<Record<SyntaxTokenKey, string>>
  ui: MonacoUiTokens
}

export interface MonacoThemeData {
  base: 'vs' | 'vs-dark'
  inherit: true
  rules: Monaco.editor.ITokenThemeRule[]
  colors: Record<string, string>
}

/** Pure: turn resolved palettes into Monaco theme data. Unit-tested. */
export function buildMonacoThemeData(input: MonacoThemeInput): MonacoThemeData {
  const { isDark, syntax, ui } = input

  const rules: Monaco.editor.ITokenThemeRule[] = TOKEN_MAP.flatMap(([token, key]) => {
    const color = syntax[key]
    return color ? [{ token, foreground: stripHash(color) }] : []
  })

  return {
    base: isDark ? 'vs-dark' : 'vs',
    inherit: true,
    rules,
    colors: {
      'editor.background': ui.background,
      'editor.foreground': ui.foreground,
      'editorCursor.foreground': ui.foreground,
      'editor.selectionBackground': ui.selection,
      'editor.inactiveSelectionBackground': ui.border,
      'editor.lineHighlightBackground': ui.border,
      'editorLineNumber.foreground': ui.subtle,
      'editorLineNumber.activeForeground': ui.foreground,
      'editorIndentGuide.background1': ui.border,
      'editorIndentGuide.activeBackground1': ui.subtle,
      'editorWhitespace.foreground': ui.subtle,
      'editorWidget.background': ui.background,
      'editorWidget.foreground': ui.foreground,
      'editorWidget.border': ui.border,
      'editorSuggestWidget.background': ui.background,
      'editorSuggestWidget.foreground': ui.foreground,
      'editorSuggestWidget.border': ui.border,
      'editorSuggestWidget.selectedBackground': ui.border,
      'input.background': ui.background,
      'input.foreground': ui.foreground,
      'input.border': ui.border,
    },
  }
}

function readUiTokens(isDark: boolean): MonacoUiTokens {
  return {
    background: resolveCssVar('--background') ?? (isDark ? '#1f1f1f' : '#ffffff'),
    foreground: resolveCssVar('--foreground') ?? (isDark ? '#f5f5f5' : '#1f1f1f'),
    selection: resolveCssVar('--accent') ?? (isDark ? '#2a2a2a' : '#e7ebf0'),
    border: resolveCssVar('--border') ?? (isDark ? '#2a2a2a' : '#e4e7ec'),
    subtle: resolveCssVar('--muted-foreground') ?? (isDark ? '#888888' : '#787d86'),
  }
}

function toMonacoThemeName(isDark: boolean): string {
  return isDark ? 'crowbar-dark' : 'crowbar-light'
}

/**
 * Resolve the live token layer, (re)define the Monaco theme, and return its id.
 * Signature is unchanged from the previous implementation; `themeId` only seeds
 * the dark/light decision as a fallback when no .dark class is present.
 */
export function defineMonacoTheme(themeId: string): string {
  const isDark =
    typeof document !== 'undefined'
      ? document.documentElement.classList.contains('dark')
      : !themeId.includes('light')

  const data = buildMonacoThemeData({
    isDark,
    syntax: readSyntaxPalette(),
    ui: readUiTokens(isDark),
  })

  const name = toMonacoThemeName(isDark)
  monacoEditor.defineTheme(name, data)
  return name
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd web && npx vitest run src/__tests__/features/editor/monaco/define-theme.test.ts`
Expected: PASS (4 tests).

- [ ] **Step 5: Typecheck the callers still compile**

Run: `cd web && npx tsc --noEmit 2>&1 | grep -E "define-theme|editor-surface|monaco-diff-editor|use-pane-editor-satellites" || echo "no type errors in callers"`
Expected: `no type errors in callers` (callers pass a `string` and use the returned `string` — unchanged).

- [ ] **Step 6: Commit**

```bash
git add web/src/features/editor/monaco/define-theme.ts web/src/__tests__/features/editor/monaco/define-theme.test.ts
git commit -m "feat(theme): build Monaco theme from live CSS token layer"
```

---

## Task 5: Rewrite the terminal theme builder

Build the xterm `ITheme` from the resolved `--terminal-*` palette + resolved UI tokens. Keep `useTerminalTheme()` / `getTerminalTheme()`'s shape so the 3 consumers (`terminal.tsx`, `use-terminal-connection.ts`, `external-editor-terminal.tsx`) need no change.

**Files:**
- Modify: `web/src/features/terminal/hooks/use-terminal-theme.ts`
- Test: `web/src/__tests__/features/terminal/use-terminal-theme.test.ts`

- [ ] **Step 1: Write the failing test for the pure builder**

Create `web/src/__tests__/features/terminal/use-terminal-theme.test.ts`:

```ts
import { describe, expect, it } from 'vitest'
import { buildTerminalTheme } from '@/features/terminal/hooks/use-terminal-theme'

const ANSI = {
  black: '#1f1f1f', red: '#d97757', green: '#a3c585', yellow: '#d6a95c',
  blue: '#6fb0e0', magenta: '#c4a6dd', cyan: '#5fbcc4', white: '#999999',
  'bright-black': '#999999', 'bright-red': '#d97757', 'bright-green': '#a3c585',
  'bright-yellow': '#d6a95c', 'bright-blue': '#6fb0e0', 'bright-magenta': '#c4a6dd',
  'bright-cyan': '#5fbcc4', 'bright-white': '#f5f5f5',
}
const UI = { background: '#141413', foreground: '#f5f5f5', cursor: '#f5f5f5' }

describe('buildTerminalTheme', () => {
  it('maps ANSI palette keys onto xterm theme fields', () => {
    const theme = buildTerminalTheme(ANSI, UI)
    expect(theme.red).toBe('#d97757')
    expect(theme.brightWhite).toBe('#f5f5f5')
    expect(theme.background).toBe('#141413')
    expect(theme.foreground).toBe('#f5f5f5')
  })

  it('derives a translucent selection from the cursor color', () => {
    const theme = buildTerminalTheme(ANSI, UI)
    expect(theme.selectionBackground?.toLowerCase()).toContain('f5f5f5')
  })
})
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd web && npx vitest run src/__tests__/features/terminal/use-terminal-theme.test.ts`
Expected: FAIL — `buildTerminalTheme` not exported.

- [ ] **Step 3: Rewrite `use-terminal-theme.ts`**

Replace the entire contents of `web/src/features/terminal/hooks/use-terminal-theme.ts` with:

```ts
import { useCallback } from 'react'
import {
  readTerminalPalette,
  resolveCssVar,
  type TerminalAnsiKey,
} from '@/features/editor/theme/resolve-css-color'

export interface TerminalTheme {
  background: string
  foreground: string
  cursor: string
  cursorAccent: string
  selectionBackground: string
  selectionForeground: string
  black: string
  red: string
  green: string
  yellow: string
  blue: string
  magenta: string
  cyan: string
  white: string
  brightBlack: string
  brightRed: string
  brightGreen: string
  brightYellow: string
  brightBlue: string
  brightMagenta: string
  brightCyan: string
  brightWhite: string
}

export interface TerminalUiTokens {
  background: string
  foreground: string
  cursor: string
}

const ANSI_FALLBACK = '#808080'

function camel(key: TerminalAnsiKey): keyof TerminalTheme {
  // 'bright-red' -> 'brightRed', 'black' -> 'black'
  return key.replace(/-([a-z])/g, (_, c) => c.toUpperCase()) as keyof TerminalTheme
}

/** #rrggbb -> rgba() with the given alpha (for the selection wash). */
function withAlpha(hex: string, alpha: number): string {
  const h = hex.replace('#', '').slice(0, 6)
  const r = Number.parseInt(h.slice(0, 2), 16)
  const g = Number.parseInt(h.slice(2, 4), 16)
  const b = Number.parseInt(h.slice(4, 6), 16)
  return `rgba(${r}, ${g}, ${b}, ${alpha})`
}

/** Pure: turn resolved palettes into an xterm ITheme. Unit-tested. */
export function buildTerminalTheme(
  ansi: Partial<Record<TerminalAnsiKey, string>>,
  ui: TerminalUiTokens,
): TerminalTheme {
  const theme = {
    background: ui.background,
    foreground: ui.foreground,
    cursor: ui.cursor,
    cursorAccent: ui.background,
    selectionBackground: withAlpha(ui.cursor, 0.25),
    selectionForeground: ui.foreground,
  } as TerminalTheme

  const keys: TerminalAnsiKey[] = [
    'black', 'red', 'green', 'yellow', 'blue', 'magenta', 'cyan', 'white',
    'bright-black', 'bright-red', 'bright-green', 'bright-yellow',
    'bright-blue', 'bright-magenta', 'bright-cyan', 'bright-white',
  ]
  for (const key of keys) {
    theme[camel(key)] = ansi[key] ?? ANSI_FALLBACK
  }
  return theme
}

function readUiTokens(): TerminalUiTokens {
  const isDark =
    typeof document !== 'undefined' && document.documentElement.classList.contains('dark')
  return {
    background: resolveCssVar('--background') ?? (isDark ? '#141413' : '#ffffff'),
    foreground: resolveCssVar('--foreground') ?? (isDark ? '#f5f5f5' : '#141413'),
    cursor: resolveCssVar('--foreground') ?? (isDark ? '#f5f5f5' : '#141413'),
  }
}

export function useTerminalTheme() {
  const getTerminalTheme = useCallback(
    (): TerminalTheme => buildTerminalTheme(readTerminalPalette(), readUiTokens()),
    [],
  )
  return { getTerminalTheme }
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd web && npx vitest run src/__tests__/features/terminal/use-terminal-theme.test.ts`
Expected: PASS (2 tests).

- [ ] **Step 5: Typecheck the terminal consumers**

Run: `cd web && npx tsc --noEmit 2>&1 | grep -E "use-terminal-theme|terminal.tsx|use-terminal-connection|external-editor-terminal" || echo "no type errors in terminal consumers"`
Expected: `no type errors in terminal consumers` (the `TerminalTheme` shape is unchanged).

- [ ] **Step 6: Commit**

```bash
git add web/src/features/terminal/hooks/use-terminal-theme.ts web/src/__tests__/features/terminal/use-terminal-theme.test.ts
git commit -m "feat(theme): build terminal theme from --terminal-* token layer"
```

---

## Task 6: Strip dead color machinery from `appearance-bootstrap.ts`

The bootstrap no longer needs to inline syntax/UI hex or cache color maps — the values live in bundled CSS keyed on `data-theme`/`.dark`. Keep only the early `data-theme`/`.dark` application and fonts.

**Files:**
- Modify: `web/src/features/settings/lib/appearance-bootstrap.ts`
- Modify: `web/src/extensions/themes/theme-registry.ts` (imports of removed exports — handled fully in Task 7; this task only removes the color data)

- [ ] **Step 1: Remove the color data and slim the cache shape**

In `web/src/features/settings/lib/appearance-bootstrap.ts`:

1. Delete the constants `CROWBAR_DARK_COLORS` (lines ~21-61), `CROWBAR_DARK_SYNTAX` (~63-82), `CROWBAR_LIGHT_COLORS` (~84-124), `CROWBAR_LIGHT_SYNTAX` (~126-145).
2. Replace the `AppearanceBootstrapCache` interface so it no longer carries color maps:

```ts
export interface AppearanceBootstrapCache {
  version: 1
  themeId: string
  themeType: 'light' | 'dark'
  editorFontFamily: string
  uiFontFamily: string
  uiFontSize: number
}
```

3. Replace `CROWBAR_BOOTSTRAP_DEFAULTS` and `DEFAULT_APPEARANCE_BOOTSTRAP_CACHE` with:

```ts
export const CROWBAR_BOOTSTRAP_DEFAULTS = {
  dark: { id: 'crowbar', type: 'dark' as const },
  light: { id: 'crowbar', type: 'light' as const },
}

export const DEFAULT_APPEARANCE_BOOTSTRAP_CACHE: AppearanceBootstrapCache = {
  version: 1,
  themeId: 'crowbar',
  themeType: 'dark',
  editorFontFamily: DEFAULT_EDITOR_FONT,
  uiFontFamily: DEFAULT_UI_FONT,
  uiFontSize: UI_FONT_SIZE_DEFAULT,
}
```

4. In `parseBootstrapCache`, delete the `cssVariables`/`syntaxTokens` reads and drop them from the returned object.
5. In `applyBootstrapAppearance`, delete the two `for (… of Object.entries(cache.cssVariables …))` / `syntaxTokens` loops (lines ~297-302). Keep `setAttribute('data-theme', …)`, `setAttribute('data-theme-type', …)`, the `.dark` toggle, and all the font/`--app-ui-*` property sets.
6. In `cacheThemeForBootstrap`, remove the `cssVariables`/`syntaxTokens` assembly; build `next` from `{ version, themeId: theme.id, themeType, editorFontFamily, uiFontFamily, uiFontSize }`.
7. Delete `sanitizeVarMap`, `prefixRecord`, and `themeTokensToCssVars` if they are now unused. (Verify with the grep in Step 2; `sanitizeVarMap` is re-exported by `theme-registry.ts` and is removed there in Task 7, so it is safe to delete here.)

- [ ] **Step 2: Verify no remaining references to the removed symbols**

Run:
```bash
cd web && grep -rn "CROWBAR_DARK_COLORS\|CROWBAR_LIGHT_COLORS\|CROWBAR_DARK_SYNTAX\|CROWBAR_LIGHT_SYNTAX\|themeTokensToCssVars\|\.cssVariables\b\|\.syntaxTokens\b" src/features/settings/lib/appearance-bootstrap.ts
```
Expected: no output.

- [ ] **Step 3: Typecheck**

Run: `cd web && npx tsc --noEmit 2>&1 | grep "appearance-bootstrap" || echo "appearance-bootstrap clean"`
Expected: may still report errors in `theme-registry.ts` (fixed in Task 7) but **none** inside `appearance-bootstrap.ts` itself. Confirm the grep prints `appearance-bootstrap clean`.

- [ ] **Step 4: Commit**

```bash
git add web/src/features/settings/lib/appearance-bootstrap.ts
git commit -m "refactor(theme): drop color/syntax hex maps from appearance bootstrap"
```

---

## Task 7: Simplify the theme registry

The registry no longer owns color palettes — CSS does. `applyTheme` just records the active theme, sets `data-theme`, syncs `.dark`, refreshes the (color-free) bootstrap cache, and notifies listeners. Public API (`getTheme`, `getAllThemes`, `applyTheme`, `onThemeChange`, `onRegistryChange`, `isRegistryReady`, `onReady`, `registerTheme`) is unchanged so all consumers keep working.

**Files:**
- Modify: `web/src/extensions/themes/theme-registry.ts`

- [ ] **Step 1: Rewrite the registry internals**

Replace the contents of `web/src/extensions/themes/theme-registry.ts` with:

```ts
import type { ThemeDefinition } from './types'
import {
  applyBootstrapAppearance,
  readAppearanceBootstrapCache,
  writeAppearanceBootstrapCache,
  DEFAULT_APPEARANCE_BOOTSTRAP_CACHE,
} from '@/features/settings/lib/appearance-bootstrap'

export { cacheThemeForBootstrap } from '@/features/settings/lib/appearance-bootstrap'

/**
 * Built-in themes exposed in the UI. Colors are CSS-first (theme.css); a theme
 * is metadata + a `dataTheme` selector. A future installable theme adds a CSS
 * block for `[data-theme="<id>"]` (+ .dark) and one entry here — nothing else.
 */
const BUILTIN_THEMES: ThemeDefinition[] = [
  {
    id: 'crowbar',
    name: 'Crowbar',
    isDark: true, // default mode; actual type follows the current Theme Mode
    type: 'dark',
    category: 'Dark',
  },
]

export class ThemeRegistry {
  private themes: Map<string, ThemeDefinition> = new Map()
  private activeThemeId: string | null = null
  private listeners: Set<() => void> = new Set()

  constructor() {
    for (const theme of BUILTIN_THEMES) {
      this.themes.set(theme.id, theme)
    }
  }

  getTheme(id: string): ThemeDefinition | undefined {
    return this.themes.get(id)
  }

  getAllThemes(): ThemeDefinition[] {
    return Array.from(this.themes.values())
  }

  getActiveTheme(): ThemeDefinition | null {
    if (!this.activeThemeId) return null
    return this.themes.get(this.activeThemeId) ?? null
  }

  registerTheme(theme: ThemeDefinition): void {
    this.themes.set(theme.id, theme)
    this.notifyListeners()
  }

  /**
   * Apply a theme. Colors come from CSS keyed on `data-theme` + `.dark`
   * (applyThemeMode toggles `.dark` immediately before this runs). We only set
   * the selector attributes, refresh the bootstrap cache, and notify so the
   * Monaco/xterm subscribers rebuild from the now-current CSS vars.
   */
  applyTheme(themeId: string): void {
    const known = this.themes.get(themeId)
    const resolvedId = known ? themeId : 'crowbar'

    this.activeThemeId = resolvedId

    const isDark =
      typeof document !== 'undefined'
        ? document.documentElement.classList.contains('dark')
        : !themeId.includes('light')

    if (known) {
      this.themes.set(resolvedId, { ...known, isDark })
    }

    const existing = readAppearanceBootstrapCache() ?? DEFAULT_APPEARANCE_BOOTSTRAP_CACHE
    const next = {
      ...existing,
      themeId: resolvedId,
      themeType: (isDark ? 'dark' : 'light') as 'dark' | 'light',
    }
    writeAppearanceBootstrapCache(next)
    applyBootstrapAppearance(next)
    this.notifyListeners()
  }

  onThemeChange(callback: () => void): () => void {
    this.listeners.add(callback)
    return () => this.listeners.delete(callback)
  }

  onRegistryChange(callback: () => void): () => void {
    return this.onThemeChange(callback)
  }

  isRegistryReady(): boolean {
    return true
  }

  onReady(cb: () => void): void {
    cb()
  }

  private notifyListeners(): void {
    for (const listener of this.listeners) {
      listener()
    }
  }
}

export const themeRegistry = new ThemeRegistry()
```

- [ ] **Step 2: Check for orphaned imports of `sanitizeVarMap` / `cacheThemeForBootstrap`**

Run:
```bash
cd web && grep -rn "sanitizeVarMap" src --include="*.ts" --include="*.tsx"
```
Expected: no remaining references (it was only re-exported here and used internally in bootstrap). If any consumer still imports it, that is a pre-existing dependency — leave `sanitizeVarMap` exported from `appearance-bootstrap.ts` instead of deleting it in Task 6. Otherwise, no action.

- [ ] **Step 3: Full typecheck**

Run: `cd web && npx tsc --noEmit`
Expected: exit 0 (no errors). Fix any remaining references to removed registry fields (`dualModeVariants`, `cssVariables` on the Crowbar theme) by deleting them at the call site — note `define-theme.ts` no longer reads the registry, so the only consumers are `appearance-settings.tsx` / `theme-selector.tsx`, which use `getAllThemes()`/`name`/`category` (all still present).

- [ ] **Step 4: Run the full test suite**

Run: `cd web && npx vitest run`
Expected: PASS (all suites, including the three new ones).

- [ ] **Step 5: Commit**

```bash
git add web/src/extensions/themes/theme-registry.ts web/src/features/settings/lib/appearance-bootstrap.ts
git commit -m "refactor(theme): make registry CSS-first metadata-only"
```

---

## Task 8: Token-presence + contrast guard test

Lock in readability: assert `theme.css` defines every `--syntax-*` key the renderers reference, and that each dedicated dark syntax hue clears WCAG AA (≥4.5:1) against the dark `--background`.

**Files:**
- Test: `web/src/__tests__/styles/theme-tokens.test.ts`

- [ ] **Step 1: Write the test**

Create `web/src/__tests__/styles/theme-tokens.test.ts`:

```ts
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'
import { cssColorToHex, SYNTAX_TOKEN_KEYS } from '@/features/editor/theme/resolve-css-color'

const css = readFileSync(
  fileURLToPath(new URL('../../styles/theme.css', import.meta.url)),
  'utf8',
)

function relLuminance(hex: string): number {
  const h = hex.replace('#', '').slice(0, 6)
  const chan = [0, 2, 4].map((i) => {
    const c = Number.parseInt(h.slice(i, i + 2), 16) / 255
    return c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4
  })
  return 0.2126 * chan[0] + 0.7152 * chan[1] + 0.0722 * chan[2]
}

function contrast(a: string, b: string): number {
  const la = relLuminance(a)
  const lb = relLuminance(b)
  const [hi, lo] = la > lb ? [la, lb] : [lb, la]
  return (hi + 0.05) / (lo + 0.05)
}

/** Pull the dark-block value of a CSS var (last definition wins for .dark). */
function darkValue(name: string): string | null {
  const darkBlock = css.slice(css.indexOf('.dark'))
  const matches = [...darkBlock.matchAll(new RegExp(`${name}:\\s*([^;]+);`, 'g'))]
  if (matches.length === 0) {
    // bound aliases live only in :root — resolve one hop for known aliases below
    return null
  }
  return matches[matches.length - 1][1].trim()
}

describe('theme.css syntax tokens', () => {
  it('declares every syntax token the renderers reference', () => {
    for (const key of SYNTAX_TOKEN_KEYS) {
      expect(css.includes(`--syntax-${key}:`)).toBe(true)
    }
  })

  it('dark dedicated syntax hues clear WCAG AA against --background', () => {
    const bgRaw = darkValue('--background')
    expect(bgRaw).toBeTruthy()
    const bg = cssColorToHex(bgRaw as string)
    expect(bg).toBeTruthy()

    // Only the dedicated (non-aliased) hues — bound roles inherit muted-foreground.
    const dedicated = [
      'keyword', 'string', 'number', 'constant', 'function', 'type', 'property',
      'tag', 'attribute', 'boolean', 'null', 'regex', 'jsx',
    ]
    for (const key of dedicated) {
      const raw = darkValue(`--syntax-${key}`)
      expect(raw, `--syntax-${key} missing in .dark`).toBeTruthy()
      const hex = cssColorToHex(raw as string)
      expect(hex, `--syntax-${key} unparseable`).toBeTruthy()
      const ratio = contrast(hex as string, bg as string)
      expect(ratio, `--syntax-${key} contrast ${ratio.toFixed(2)} < 4.5`).toBeGreaterThanOrEqual(4.5)
    }
  })
})
```

- [ ] **Step 2: Run the test**

Run: `cd web && npx vitest run src/__tests__/styles/theme-tokens.test.ts`
Expected: PASS. If a hue fails the 4.5 ratio, lighten that dark hue in `theme.css` `.dark` (raise its value toward white) until it passes, then re-run. Record any value you change.

- [ ] **Step 3: Commit**

```bash
git add web/src/__tests__/styles/theme-tokens.test.ts web/src/styles/theme.css
git commit -m "test(theme): guard syntax token presence + dark-mode AA contrast"
```

---

## Task 9: Manual verification in the running app

Automated tests can't see the editor/terminal render. Verify the real thing.

**Files:** none (verification only)

- [ ] **Step 1: Launch the app**

Use the `run` skill (or the project's dev command, e.g. `cd desktop && npm run tauri dev`) to launch Crowbar.

- [ ] **Step 2: Editor syntax — dark**

Open a TypeScript file. Confirm: keyword (terracotta), string (sage), number (gold), function (blue), type (violet), boolean/const (teal) are each visibly distinct — no terracotta collision. Comments are muted grey.

- [ ] **Step 3: Switch to Light mode**

Settings → Appearance → Theme Mode → Light. Confirm the editor re-themes immediately (the `onThemeChange` subscription fires `defineMonacoTheme`) and syntax is readable on the light background.

- [ ] **Step 4: Terminal**

Open the integrated terminal. Run `ls --color` or `git status`. Confirm ANSI colors render and track the theme (red=destructive, green=success, etc.), and that switching light/dark recolors the terminal.

- [ ] **Step 5: Tree-sitter parity**

Confirm any Tree-sitter-rendered surfaces (`.token-*` classes) match the Monaco colors — both now read the same `--syntax-*` vars.

- [ ] **Step 6: Final commit (if any tuning was needed)**

```bash
git add -A
git commit -m "chore(theme): final palette tuning from manual verification"
```

---

## Self-Review Notes (for the implementer)

- **Spec coverage:** canonical layer (T1), resolver (T2-3), Monaco builder (T4), terminal builder (T5), bootstrap slimming (T6), registry (T7), contrast guard + registry/token integrity (T8), manual verify (T9). The "installable theme seam" is satisfied by T7's metadata-only registry + the `data-theme` CSS convention (documented in the registry comment) — no installer is built, per scope.
- **Signatures held stable on purpose:** `defineMonacoTheme(themeId: string): string` and `useTerminalTheme().getTerminalTheme(): TerminalTheme` keep their exact shapes so the 6 call sites and the existing `onThemeChange` subscriptions need no edits.
- **Bound vs dedicated:** comment/variable/punctuation/operator/error are CSS aliases (defined once in `:root`); the 23 dedicated hues are defined in both `:root` and `.dark`. The contrast test only asserts dedicated hues (aliases inherit already-validated UI tokens).
