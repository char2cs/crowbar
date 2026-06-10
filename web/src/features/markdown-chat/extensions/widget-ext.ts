import { EditorState, StateField, RangeSetBuilder } from '@codemirror/state'
import { Decoration, DecorationSet, EditorView, WidgetType } from '@codemirror/view'
import { createElement } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { syntaxTree } from '@codemirror/language'
import type { MarkdownTurn, ToolCallData } from '../types'
import { WIDGET_ID_RE, TOOL_CALL_RE } from '../types'
import { ToolCallPill } from '../components/tool-call-pill'
import { MarkdownBlock } from '../components/markdown/markdown-block'

// Both CM widgets below mount a React root; each unmounts it in destroy() to
// avoid leaking roots as CodeMirror recycles decorations.

// -- Tool call pill widget --

class ToolCallWidget extends WidgetType {
  private root: Root | null = null

  constructor(private data: ToolCallData) {
    super()
  }

  eq(other: ToolCallWidget) {
    return JSON.stringify(this.data) === JSON.stringify(other.data)
  }

  toDOM() {
    const div = document.createElement('div')
    this.root = createRoot(div)
    this.root.render(createElement(ToolCallPill, { data: this.data }))
    return div
  }

  destroy() {
    const root = this.root
    this.root = null
    if (root) Promise.resolve().then(() => root.unmount())
  }
}

// -- Fenced widget (referenced blocks, e.g. excalidraw) --
//
// Renders the SAME block component the history uses (via MarkdownBlock), in its
// editable variant. Only fenced blocks carrying a `widget-id` are widgetized;
// plain code/mermaid stay as editable text in the input.

class FencedWidget extends WidgetType {
  private root: Root | null = null

  constructor(
    private widgetId: string,
    private info: string,
    private getTurns: () => MarkdownTurn[],
    private onWidgetChange: (widgetId: string, payload: unknown) => void,
  ) {
    super()
  }

  eq(other: FencedWidget) {
    return this.widgetId === other.widgetId && this.info === other.info
  }

  toDOM() {
    const div = document.createElement('div')
    div.className = 'cm-widget-container'
    const widgets = this.getTurns().flatMap((t) => t.widgets)
    this.root = createRoot(div)
    this.root.render(
      createElement(MarkdownBlock, {
        info: this.info,
        source: '',
        widgets,
        editable: true,
        onChange: this.onWidgetChange,
      }),
    )
    return div
  }

  destroy() {
    const root = this.root
    this.root = null
    if (root) Promise.resolve().then(() => root.unmount())
  }
}

// Pending decoration entry — collected before sorting to satisfy CM6's ascending-from requirement.
interface PendingDeco {
  from: number
  to: number
  deco: Decoration
}

function buildWidgetDecorations(
  state: EditorState,
  getTurns: () => MarkdownTurn[],
  onWidgetChange: (widgetId: string, payload: unknown) => void,
): DecorationSet {
  const pending: PendingDeco[] = []

  // Tool call comment lines → pill
  for (let i = 1; i <= state.doc.lines; i++) {
    const line = state.doc.line(i)
    const toolMatch = line.text.match(TOOL_CALL_RE)
    if (toolMatch) {
      try {
        const data: ToolCallData = JSON.parse(toolMatch[1])
        pending.push({
          from: line.from,
          to: line.to,
          deco: Decoration.replace({ widget: new ToolCallWidget(data) }),
        })
      } catch {
        /* malformed JSON — skip */
      }
    }
  }

  // Fenced blocks carrying a widget-id → editable block widget
  syntaxTree(state)
    .cursor()
    .iterate((node) => {
      if (node.name !== 'FencedCode') return
      const infoLine = state.doc.lineAt(node.from).text
      const widgetIdMatch = infoLine.match(WIDGET_ID_RE)
      if (!widgetIdMatch) return

      const widgetId = widgetIdMatch[1]
      const info = infoLine.replace(/^```/, '').trim()

      pending.push({
        from: node.from,
        to: node.to,
        deco: Decoration.replace({
          widget: new FencedWidget(widgetId, info, getTurns, onWidgetChange),
          inclusive: true,
        }),
      })
    })

  // CM6 RangeSetBuilder requires strictly ascending `from`.
  pending.sort((a, b) => a.from - b.from)

  const builder = new RangeSetBuilder<Decoration>()
  for (const { from, to, deco } of pending) {
    builder.add(from, to, deco)
  }
  return builder.finish()
}

// Factory that creates the StateField with access to turns and callback.
export function widgetExt(
  getTurns: () => MarkdownTurn[],
  onWidgetChange: (widgetId: string, payload: unknown) => void,
) {
  const widgetDecoField = StateField.define<DecorationSet>({
    create: (state) => buildWidgetDecorations(state, getTurns, onWidgetChange),
    update(deco, tr) {
      if (!tr.docChanged) return deco
      return buildWidgetDecorations(tr.state, getTurns, onWidgetChange)
    },
    provide: (f) => EditorView.decorations.from(f),
  })

  return [widgetDecoField]
}
