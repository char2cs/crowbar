# Configurable markdown font size

## Problem

The Plate markdown editor renders at a fixed size. `markdown-editor.css` sets
`font-size: 1rem` on the editable, and every heading, code block and caption
below it is expressed in `em`. There is no way for a user to change it.

## Scope

**In:** the markdown document editor (`.crowbar-markdown-editor`, the Rich view
of a `.md` buffer).

**Out, deliberately:**

- The **review comment composer**. It is Plate too, but it shares
  `MARKDOWN_PROSE_CLASS` with the posted-comment renderer at `text-sm`, and that
  equality is the feature — the composer is a preview of what gets posted. A
  document-reading size would make the composer the largest text in the review
  pane. It already scales with UI Font Size.
- The **Source view** of a markdown file. That is Monaco, and it correctly
  follows the existing code `fontSize`.
- The **Markdown Preview** pane (`markdown-preview.tsx`, react-markdown, not
  Plate). It follows the code `fontSize` today; changing that is a separate call.

## Design

One setting, one CSS variable, one declaration changed.

### Setting

`markdownFontSize: number` — pixels, default **16**, range **12–24**, step 1.

16 is exactly what `1rem` resolves to today (nothing overrides `html`'s
font-size), so the default is byte-for-byte the current rendering.

Kept separate from the existing `fontSize`, which is the code/Monaco size — the
two surfaces have genuinely different reading needs, and the Source view of the
same file already follows `fontSize`.

### Application

`--md-base-font-size` on `document.documentElement`, consumed by

```css
.crowbar-markdown-editor [data-slate-editor] {
  font-size: var(--md-base-font-size, 1rem);
}
```

Everything else in the stylesheet is `em`-relative, so it rescales for free.

`--md-measure` (the reading column) becomes `calc(45 * var(--md-base-font-size,
1rem))` so the column tracks the type and holds its documented 70–80 character
measure instead of cramping as the text grows. It cannot be `45em`: the token is
also consumed by the frontmatter banner, which has a different font-size, and
`em` inside a custom property resolves at the element that *uses* it — the two
would stop aligning. At the default the expression is identical to `45rem`.

### Wiring

Follows the `uiFontSize` path exactly:

| File | Change |
| --- | --- |
| `lib/markdown-font-size.ts` (new) | min/max/step/default + `normalizeMarkdownFontSize` |
| `config/typography-defaults.ts` | `DEFAULT_MARKDOWN_FONT_SIZE = 16` |
| `types/settings.ts` | `markdownFontSize: number` |
| `config/default-settings.ts` | default + snapshot normalization |
| `lib/settings-normalization.ts` | normalize on load and on update |
| `components/font-style-injector.tsx` | set the var live (no reopen) |
| `lib/appearance-bootstrap.ts` | cache + apply at boot |
| `lib/settings-effects.ts` | re-cache when the key changes |
| `components/tabs/editor-settings.tsx` | `Markdown` section + `NumberInput` row |
| `config/search-index.ts` | settings-search entry |

Settings hydrate **asynchronously** (`await loadSettingsFromStore()`), so
without the bootstrap cache a restored markdown tab would paint at 16px and jump
to the configured size. The cache is what prevents that.

`cacheFontsForBootstrap` takes three positional args today; a fourth makes the
call site unreadable, so it moves to an options object. Its only caller is
`cacheFontSettings` in `settings-effects.ts`.

No migration for existing bootstrap caches: `parseBootstrapCache` normalizes a
missing `markdownFontSize` to the default, which is the value those users are
already seeing.

## Testing

- Unit: `normalizeMarkdownFontSize` clamps, snaps, and rejects non-finite input.
- Unit: `normalizeSettings` / `normalizeSettingValue` carry the new key.
- Live: change the setting in a running app, confirm the document rescales
  without reopening the file, and that the reading column stays centred.
