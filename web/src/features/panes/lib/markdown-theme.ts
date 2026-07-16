// react-doctor-disable-next-line prefer-dynamic-import -- only imported by comment-composer.tsx, itself already behind the `GitDiffEditorStackLazy` React.lazy() boundary (review-diff-tab.tsx). Verified via `bunx vite build`: 0 "codemirror" occurrences in the entry chunk.
import { EditorView } from '@codemirror/view'

// CodeMirror theme for the transparent inline markdown editor (review comment
// composer). Kept out of the markdown component file so that file stays
// Fast-Refresh-safe. Only imported by lazy-loaded review surfaces, so it never
// pulls @codemirror into the entry chunk.
export const transparentMarkdownTheme = EditorView.theme({
  '&': { backgroundColor: 'transparent !important', color: 'var(--foreground)' },
  '&.cm-focused': { outline: 'none !important', backgroundColor: 'transparent !important' },
  '.cm-content': { caretColor: 'var(--foreground)', padding: '0' },
  '.cm-cursor': { borderLeftColor: 'var(--foreground)' },
  '.cm-placeholder': { color: 'var(--muted-foreground)', opacity: '0.4' },
  '.cm-line': { padding: '0' },
  '.cm-scroller': { fontFamily: 'inherit', backgroundColor: 'transparent !important' },
  '.cm-gutters': { backgroundColor: 'transparent !important', border: 'none' },
  '.cm-activeLine': { backgroundColor: 'transparent !important' },
  '.cm-activeLineGutter': { backgroundColor: 'transparent !important' },
  '.cm-selectionBackground': {
    backgroundColor: 'color-mix(in srgb, var(--primary) 20%, transparent) !important',
  },
  '&.cm-focused .cm-selectionBackground': {
    backgroundColor: 'color-mix(in srgb, var(--primary) 30%, transparent) !important',
  },
})
