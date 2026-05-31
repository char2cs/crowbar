import { EditorState, StateField, RangeSetBuilder } from '@codemirror/state'
import { Decoration, DecorationSet, EditorView } from '@codemirror/view'
import { syntaxTree } from '@codemirror/language'

// Exposed for testing — checks if a decoration of a given class exists at position.
export function hasLivePreviewDecoration(
  state: EditorState,
  pos: number,
  cls: string,
): boolean {
  const deco = state.field(livePreviewField, false)
  if (!deco) return false
  let found = false
  deco.between(pos, pos + 1, (_from, _to, d) => {
    if ((d.spec as { class?: string }).class === cls) found = true
  })
  return found
}

function cursorLine(state: EditorState): number {
  return state.doc.lineAt(state.selection.main.head).number
}

function buildLivePreviewDecorations(state: EditorState): DecorationSet {
  const builder = new RangeSetBuilder<Decoration>()
  const activeLine = cursorLine(state)
  const tree = syntaxTree(state)

  tree.cursor().iterate((node) => {
    const lineNum = state.doc.lineAt(node.from).number
    if (lineNum === activeLine) return // show raw syntax on cursor line

    switch (node.name) {
      case 'ATXHeading1':
        builder.add(node.from, node.to, Decoration.mark({ class: 'cm-live-heading-1' }))
        break
      case 'ATXHeading2':
        builder.add(node.from, node.to, Decoration.mark({ class: 'cm-live-heading-2' }))
        break
      case 'ATXHeading3':
        builder.add(node.from, node.to, Decoration.mark({ class: 'cm-live-heading-3' }))
        break
      case 'StrongEmphasis':
        builder.add(node.from, node.to, Decoration.mark({ class: 'cm-live-bold' }))
        break
      case 'Emphasis':
        builder.add(node.from, node.to, Decoration.mark({ class: 'cm-live-italic' }))
        break
      case 'InlineCode':
        builder.add(node.from, node.to, Decoration.mark({ class: 'cm-live-inline-code' }))
        break
    }
  })

  return builder.finish()
}

const livePreviewField = StateField.define<DecorationSet>({
  create: (state) => buildLivePreviewDecorations(state),
  update(deco, tr) {
    if (!tr.docChanged && !tr.selection) return deco
    return buildLivePreviewDecorations(tr.state)
  },
  provide: (f) => EditorView.decorations.from(f),
})

const livePreviewTheme = EditorView.theme({
  '.cm-live-heading-1': {
    fontSize: '1.5em',
    fontWeight: '700',
  },
  '.cm-live-heading-2': {
    fontSize: '1.25em',
    fontWeight: '700',
  },
  '.cm-live-heading-3': {
    fontSize: '1.1em',
    fontWeight: '700',
  },
  '.cm-live-bold': { fontWeight: '700' },
  '.cm-live-italic': { fontStyle: 'italic' },
  '.cm-live-inline-code': {
    fontFamily: 'var(--font-editor)',
    backgroundColor: 'hsl(var(--color-code) / 0.15)',
    borderRadius: '3px',
    padding: '0 3px',
  },
})

export function livePreview() {
  return [livePreviewField, livePreviewTheme]
}
