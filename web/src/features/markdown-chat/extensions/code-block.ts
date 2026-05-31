import { syntaxTree } from '@codemirror/language'
import {
  Decoration,
  type DecorationSet,
  EditorView,
  ViewPlugin,
  type ViewUpdate,
} from '@codemirror/view'
import { type EditorState, RangeSetBuilder } from '@codemirror/state'
import { WIDGET_ID_RE } from '../types'

interface LineDeco {
  pos: number
  cls: string
}

// Style every fenced code block (```lang … ```) as a monospace panel.
// Widget blocks (excalidraw/mermaid, identified by a `widget-id:` info string)
// are skipped — those are rendered as inline widgets by widget-ext. We don't do
// per-language token highlighting here: most languages (e.g. Go) have no parser
// installed, so a consistent code-panel treatment is what actually applies
// across all of them.
function buildCodeBlockDecorations(state: EditorState, view: EditorView): DecorationSet {
  const lines: LineDeco[] = []

  for (const { from, to } of view.visibleRanges) {
    syntaxTree(state).iterate({
      from,
      to,
      enter: (node) => {
        if (node.name !== 'FencedCode') return
        const startLine = state.doc.lineAt(node.from)
        // Skip widget fences — handled by widget-ext.
        if (WIDGET_ID_RE.test(startLine.text)) return
        const endLine = state.doc.lineAt(node.to)

        for (let ln = startLine.number; ln <= endLine.number; ln++) {
          let cls = 'cm-code-block'
          if (ln === startLine.number) cls += ' cm-code-block-first'
          if (ln === endLine.number) cls += ' cm-code-block-last'
          lines.push({ pos: state.doc.line(ln).from, cls })
        }
      },
    })
  }

  // RangeSetBuilder requires strictly ascending positions.
  lines.sort((a, b) => a.pos - b.pos)
  const builder = new RangeSetBuilder<Decoration>()
  for (const { pos, cls } of lines) {
    builder.add(pos, pos, Decoration.line({ class: cls }))
  }
  return builder.finish()
}

const codeBlockTheme = EditorView.theme({
  '.cm-code-block': {
    fontFamily: 'var(--font-editor, monospace)',
    fontSize: '0.9em',
    background: 'var(--code-highlight)',
  },
  '.cm-code-block-first': {
    paddingTop: '8px',
    borderTopLeftRadius: '8px',
    borderTopRightRadius: '8px',
  },
  '.cm-code-block-last': {
    paddingBottom: '8px',
    borderBottomLeftRadius: '8px',
    borderBottomRightRadius: '8px',
  },
})

const codeBlockPlugin = ViewPlugin.fromClass(
  class {
    decorations: DecorationSet
    constructor(view: EditorView) {
      this.decorations = buildCodeBlockDecorations(view.state, view)
    }
    update(update: ViewUpdate) {
      if (update.docChanged || update.viewportChanged) {
        this.decorations = buildCodeBlockDecorations(update.state, update.view)
      }
    }
  },
  { decorations: (v) => v.decorations },
)

export function codeBlockExt() {
  return [codeBlockPlugin, codeBlockTheme]
}
