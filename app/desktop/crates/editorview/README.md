# editorview

## Owns

Drawing one editor pane: the gpui-kit `Editor` over the text of the document the pane shows, under
develop's toolbar (the path breadcrumb, the caret's line and column, the language).

## Public interface

- `EditorView`: `new` from an `Opening` (text, path, the line to reveal, `Access`), `text`,
  `caret`; it is `Focusable`.
- `Opening`, `Access` (`Editable`, `ReadOnly`), `Caret`.
- `init`: registers the gpui-kit editor's theme global and key bindings.
- `Settings` (font size, line height multiplier, tab size, word wrap, line numbers, indent
  guides, `WhitespaceMode`): installed as a global with `install`; every open `EditorView`
  follows it live. The font family is the theme's code font.

## Depends on

- `editor`: the language of a path.
- `gpui`, `ui` (palette, icons, text sizes).
- `gpui-kit` (feature `component`): the `Editor` element and its `EditorState` engine, from the
  vendored `gpui-component` and `gpui-base` (`app/desktop/vendor`).

## Benchmarks

`cargo bench -p editorview --bench open_large`: opening 10K, 50K and 200K-line files and typing
at their end, with the Linux text system shaping the lines.
