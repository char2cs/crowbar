import { useEffect, useRef } from 'react'
import { EditorState } from '@codemirror/state'
import { EditorView, keymap } from '@codemirror/view'
import { markdown } from '@codemirror/lang-markdown'
import { history, defaultKeymap, historyKeymap } from '@codemirror/commands'
import type { MarkdownTurn } from '../types'
import { turnsToDocument, turnBoundaries } from '../extensions/turn-boundaries'
import { mountTurnMeta, turnMetaTheme } from '../extensions/turn-meta'
import { livePreview } from '../extensions/live-preview'
import { codeBlockExt } from '../extensions/code-block'
import { codeLanguages } from '../extensions/code-languages'
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
  const overlayRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!containerRef.current || !overlayRef.current) return

    // Set after the view exists; the updateListener calls through this ref so the
    // sticky metadata layer recomputes on doc/geometry changes (e.g. streaming).
    const refreshMetaRef = { current: () => {} }

    const state = EditorState.create({
      doc: turnsToDocument(turns),
      extensions: [
        markdown({ codeLanguages }),
        history(),
        keymap.of([...defaultKeymap, ...historyKeymap]),
        EditorView.editable.of(false),
        EditorView.lineWrapping,
        turnBoundaries(),
        turnMetaTheme,
        livePreview(),
        codeBlockExt(),
        streamingExt(),
        todoStickyExt(),
        widgetExt(getTurns, onWidgetChange),
        EditorView.updateListener.of((u) => {
          if (u.docChanged || u.geometryChanged || u.viewportChanged) {
            refreshMetaRef.current()
          }
        }),
      ],
    })

    const view = new EditorView({ state, parent: containerRef.current })
    onReady(view)

    const meta = mountTurnMeta(view, overlayRef.current, getTurns)
    refreshMetaRef.current = meta.refresh

    return () => {
      meta.destroy()
      view.destroy()
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  return (
    <div ref={containerRef} className="relative h-full w-full">
      <div ref={overlayRef} aria-hidden="true" />
    </div>
  )
}
