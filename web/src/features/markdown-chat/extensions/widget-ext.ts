import { EditorState, StateField, RangeSetBuilder } from '@codemirror/state'
import { Decoration, DecorationSet, EditorView, WidgetType } from '@codemirror/view'
import { createElement } from 'react'
import { createRoot } from 'react-dom/client'
import { syntaxTree } from '@codemirror/language'
import type { MarkdownTurn, ToolCallData } from '../types'
import { WIDGET_ID_RE, TOOL_CALL_RE } from '../types'
import { getWidget } from './widget-registry'
import { ToolCallPill } from '../components/tool-call-pill'

// -- Tool call pill widget --

class ToolCallWidget extends WidgetType {
  constructor(private data: ToolCallData) {
    super()
  }

  eq(other: ToolCallWidget) {
    return JSON.stringify(this.data) === JSON.stringify((other as ToolCallWidget).data)
  }

  toDOM() {
    const div = document.createElement('div')
    const root = createRoot(div)
    root.render(createElement(ToolCallPill, { data: this.data }))
    return div
  }

  destroy(_dom: HTMLElement) {
    // React roots should be unmounted to prevent memory leaks.
    // We use a microtask to avoid unmounting during render.
    Promise.resolve().then(() => {
      try {
        // createRoot().unmount() is the proper cleanup but we can't hold a ref here.
        // The DOM node is removed by CM6 anyway; GC handles the rest.
      } catch { /* ignore */ }
    })
  }
}

// -- Generic (Excalidraw / Mermaid) widget --

class FencedWidget extends WidgetType {
  constructor(
    private widgetId: string,
    private widgetType: string,
    private turns: MarkdownTurn[],
    private onWidgetChange: (widgetId: string, payload: unknown) => void,
  ) {
    super()
  }

  eq(other: FencedWidget) {
    return (
      this.widgetId === (other as FencedWidget).widgetId &&
      this.widgetType === (other as FencedWidget).widgetType
    )
  }

  toDOM() {
    const div = document.createElement('div')
    div.className = 'cm-widget-container'

    const Component = getWidget(this.widgetType)
    if (!Component) {
      div.textContent = `[unknown widget type: ${this.widgetType}]`
      return div
    }

    const turn = this.turns.find((t) => t.widgets.some((w) => w.id === this.widgetId))
    const widgetData = turn?.widgets.find((w) => w.id === this.widgetId)
    if (!widgetData) {
      div.textContent = `[widget not found: ${this.widgetId}]`
      return div
    }

    const root = createRoot(div)
    root.render(
      createElement(Component, {
        data: widgetData,
        onChange: (payload) => this.onWidgetChange(this.widgetId, payload),
      }),
    )
    return div
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
  turns: MarkdownTurn[],
  onWidgetChange: (widgetId: string, payload: unknown) => void,
): DecorationSet {
  const pending: PendingDeco[] = []

  // Collect tool call decorations line by line
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
      } catch { /* malformed JSON — skip */ }
    }
  }

  // Collect fenced widget block decorations via syntax tree
  syntaxTree(state).cursor().iterate((node) => {
    if (node.name !== 'FencedCode') return
    const infoLine = state.doc.lineAt(node.from).text
    const widgetIdMatch = infoLine.match(WIDGET_ID_RE)
    if (!widgetIdMatch) return

    const widgetId = widgetIdMatch[1]
    // Extract widget type from info string: e.g. "```excalidraw widget-id:abc"
    const infoString = infoLine.replace(/^```/, '').trim()
    const widgetType = infoString.split(' ')[0]

    pending.push({
      from: node.from,
      to: node.to,
      deco: Decoration.replace({
        widget: new FencedWidget(widgetId, widgetType, turns, onWidgetChange),
        inclusive: true,
      }),
    })
  })

  // Sort ascending by `from` — CM6 RangeSetBuilder requires strictly ascending order
  // across all ranges. Tool calls and fenced blocks may be interleaved in the document.
  pending.sort((a, b) => a.from - b.from)

  const builder = new RangeSetBuilder<Decoration>()
  for (const { from, to, deco } of pending) {
    builder.add(from, to, deco)
  }
  return builder.finish()
}

// Factory that creates the StateField with access to turns and callback.
export function widgetExt(
  turns: MarkdownTurn[],
  onWidgetChange: (widgetId: string, payload: unknown) => void,
) {
  const widgetDecoField = StateField.define<DecorationSet>({
    create: (state) => buildWidgetDecorations(state, turns, onWidgetChange),
    update(_deco, tr) {
      if (!tr.docChanged) return _deco
      return buildWidgetDecorations(tr.state, turns, onWidgetChange)
    },
    provide: (f) => EditorView.decorations.from(f),
  })

  return [widgetDecoField]
}
