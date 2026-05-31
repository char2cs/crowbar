import { useEffect, useRef } from 'react'
import { EditorState } from '@codemirror/state'
import { EditorView, keymap } from '@codemirror/view'
import { markdown } from '@codemirror/lang-markdown'
import { history, defaultKeymap, historyKeymap } from '@codemirror/commands'
import type { MarkdownTurn } from '../types'
import { turnsToDocument, turnBoundaries } from '../extensions/turn-boundaries'
import { livePreview } from '../extensions/live-preview'
import { streamingExt } from '../extensions/streaming-ext'
import { todoStickyExt } from '../extensions/todo-sticky'
import { widgetExt } from '../extensions/widget-ext'
import './excalidraw-widget'
import './mermaid-widget'

interface MarkdownChatHistoryProps {
  turns: MarkdownTurn[]
  getTurns: () => MarkdownTurn[]
  onWidgetChange: (widgetId: string, payload: unknown) => void
  onReady: (view: EditorView) => void
}

export function MarkdownChatHistory({
  turns,
  getTurns,
  onWidgetChange,
  onReady,
}: MarkdownChatHistoryProps) {
  const containerRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!containerRef.current) return

    const state = EditorState.create({
      doc: turnsToDocument(turns),
      extensions: [
        markdown(),
        history(),
        keymap.of([...defaultKeymap, ...historyKeymap]),
        EditorView.editable.of(false),
        EditorView.lineWrapping,
        turnBoundaries(),
        livePreview(),
        streamingExt(),
        todoStickyExt(),
        widgetExt(getTurns, onWidgetChange),
      ],
    })

    const view = new EditorView({ state, parent: containerRef.current })
    onReady(view)

    return () => view.destroy()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  return (
    <div
      ref={containerRef}
      className="h-full w-full"
    />
  )
}
