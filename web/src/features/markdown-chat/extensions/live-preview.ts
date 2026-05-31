import { EditorState, StateField, RangeSetBuilder } from '@codemirror/state'
import { Decoration, DecorationSet, EditorView } from '@codemirror/view'
import { syntaxTree } from '@codemirror/language'

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

interface PendingDeco {
  from: number
  to: number
  deco: Decoration
}

function buildLivePreviewDecorations(state: EditorState): DecorationSet {
  const pending: PendingDeco[] = []
  const activeLine = cursorLine(state)
  const tree = syntaxTree(state)

  tree.cursor().iterate((node) => {
    switch (node.name) {
      case 'ATXHeading1':
      case 'ATXHeading2':
      case 'ATXHeading3': {
        if (state.doc.lineAt(node.from).number === activeLine) return false

        const level = node.name === 'ATXHeading1' ? 1 : node.name === 'ATXHeading2' ? 2 : 3
        let headerMarkTo = node.from

        // Walk children to find and hide the HeaderMark + trailing space (e.g., "## ")
        let child = node.node.firstChild
        while (child) {
          if (child.name === 'HeaderMark') {
            // HeaderMark = just "#", "##", "###" — does NOT include the trailing space.
            // Extend by 1 if the next character is a space so "## Heading" hides "## ".
            const trailingSpace = state.doc.sliceString(child.to, child.to + 1) === ' ' ? 1 : 0
            const replaceEnd = child.to + trailingSpace
            pending.push({ from: child.from, to: replaceEnd, deco: Decoration.replace({}) })
            headerMarkTo = replaceEnd
          }
          child = child.nextSibling
        }

        // Mark only the text portion (after the hidden header mark)
        if (headerMarkTo < node.to) {
          pending.push({
            from: headerMarkTo,
            to: node.to,
            deco: Decoration.mark({ class: `cm-live-heading-${level}` }),
          })
        }
        return false
      }

      case 'StrongEmphasis': {
        if (state.doc.lineAt(node.from).number === activeLine) return false

        let contentFrom = node.from
        let contentTo = node.to

        let child = node.node.firstChild
        while (child) {
          if (child.name === 'EmphasisMark') {
            pending.push({ from: child.from, to: child.to, deco: Decoration.replace({}) })
            if (child.from === node.from) contentFrom = child.to
            else contentTo = child.from
          }
          child = child.nextSibling
        }

        if (contentFrom < contentTo) {
          pending.push({ from: contentFrom, to: contentTo, deco: Decoration.mark({ class: 'cm-live-bold' }) })
        }
        return false
      }

      case 'Emphasis': {
        if (state.doc.lineAt(node.from).number === activeLine) return false

        let contentFrom = node.from
        let contentTo = node.to

        let child = node.node.firstChild
        while (child) {
          if (child.name === 'EmphasisMark') {
            pending.push({ from: child.from, to: child.to, deco: Decoration.replace({}) })
            if (child.from === node.from) contentFrom = child.to
            else contentTo = child.from
          }
          child = child.nextSibling
        }

        if (contentFrom < contentTo) {
          pending.push({ from: contentFrom, to: contentTo, deco: Decoration.mark({ class: 'cm-live-italic' }) })
        }
        return false
      }

      case 'InlineCode': {
        if (state.doc.lineAt(node.from).number !== activeLine) {
          pending.push({ from: node.from, to: node.to, deco: Decoration.mark({ class: 'cm-live-inline-code' }) })
        }
        return false
      }
    }
  })

  // Sort by from (RangeSetBuilder requires strictly ascending from values)
  pending.sort((a, b) => a.from !== b.from ? a.from - b.from : a.to - b.to)

  const builder = new RangeSetBuilder<Decoration>()
  for (const { from, to, deco } of pending) {
    builder.add(from, to, deco)
  }
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
  '.cm-live-heading-1': { fontSize: '1.5em', fontWeight: '700', lineHeight: '1.3' },
  '.cm-live-heading-2': { fontSize: '1.25em', fontWeight: '700', lineHeight: '1.3' },
  '.cm-live-heading-3': { fontSize: '1.1em', fontWeight: '600', lineHeight: '1.3' },
  '.cm-live-bold': { fontWeight: '700' },
  '.cm-live-italic': { fontStyle: 'italic' },
  '.cm-live-inline-code': {
    fontFamily: 'var(--font-editor)',
    backgroundColor: 'oklch(0 0 0 / 6%)',
    borderRadius: '3px',
    padding: '0 3px',
  },
})

export function livePreview() {
  return [livePreviewField, livePreviewTheme]
}
