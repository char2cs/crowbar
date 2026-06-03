import { EditorView } from '@codemirror/view'

export interface SlashCommandState {
  open: boolean
  query: string
  anchorRect: DOMRect | null
}

export type SlashCommandListener = (state: SlashCommandState) => void

function getAnchorRect(view: EditorView): DOMRect {
  const pos = view.state.selection.main.head
  const coords = view.coordsAtPos(pos)
  if (coords) {
    return new DOMRect(coords.left, coords.top, 0, coords.bottom - coords.top)
  }
  return new DOMRect(0, 0, 0, 0)
}

export function slashCommandExt(onStateChange: SlashCommandListener) {
  let paletteOpen = false

  function close(view: EditorView) {
    paletteOpen = false
    onStateChange({ open: false, query: '', anchorRect: null })
    view.focus()
  }

  return [
    EditorView.domEventHandlers({
      keydown(e, view) {
        if (e.key === 'Escape' && paletteOpen) {
          close(view)
          e.preventDefault()
          return true
        }
        return false
      },
    }),
    EditorView.updateListener.of((update) => {
      if (!update.docChanged && !update.selectionSet) return

      const { from } = update.state.selection.main
      const line = update.state.doc.lineAt(from)
      const col = from - line.from
      const textBeforeCursor = line.text.slice(0, col)
      const slashIdx = textBeforeCursor.lastIndexOf('/')

      if (slashIdx === -1) {
        if (paletteOpen) close(update.view)
        return
      }

      // Only trigger if / is at start of line or after whitespace
      const charBefore = slashIdx > 0 ? textBeforeCursor[slashIdx - 1] : undefined
      if (charBefore !== undefined && charBefore !== ' ' && charBefore !== '\t') {
        if (paletteOpen) close(update.view)
        return
      }

      paletteOpen = true
      const currentQuery = textBeforeCursor.slice(slashIdx + 1)
      onStateChange({
        open: true,
        query: currentQuery,
        anchorRect: getAnchorRect(update.view),
      })
    }),
  ]
}
