import { useEffect, useRef, useState, useCallback } from 'react'
import { EditorState, Prec } from '@codemirror/state'
import { EditorView, keymap } from '@codemirror/view'
import { defaultKeymap, history, historyKeymap } from '@codemirror/commands'
import { markdown } from '@codemirror/lang-markdown'
import type { MarkdownTurn } from '../types'
import { turnsToDocument, getInputPos } from '../extensions/turn-boundaries'
import { livePreview } from '../extensions/live-preview'
import { streamingExt } from '../extensions/streaming-ext'
import { todoStickyExt } from '../extensions/todo-sticky'
import { widgetExt } from '../extensions/widget-ext'
import { slashCommandExt, type SlashCommandState } from '../extensions/slash-command-ext'
import { SlashCommandPalette, type SlashCommand } from './slash-command-palette'
import { turnBoundaries } from '../extensions/turn-boundaries'
// Import widgets to trigger self-registration in WIDGET_REGISTRY
import './excalidraw-widget'
import './mermaid-widget'

interface MarkdownChatEditorProps {
  turns: MarkdownTurn[]
  streamingTurnId: string | null
  getTurns: () => MarkdownTurn[]
  onSubmit: (content: string) => void
  onWidgetChange: (widgetId: string, payload: unknown) => void
  onSlashCommand: (command: SlashCommand) => void
  onEditorReady: (view: EditorView) => void
}

export function MarkdownChatEditor({
  turns,
  streamingTurnId,
  getTurns,
  onSubmit,
  onWidgetChange,
  onSlashCommand,
  onEditorReady,
}: MarkdownChatEditorProps) {
  const containerRef = useRef<HTMLDivElement>(null)
  const viewRef = useRef<EditorView | null>(null)
  const onSubmitRef = useRef(onSubmit)
  const onSlashCommandRef = useRef(onSlashCommand)
  const [slashState, setSlashState] = useState<SlashCommandState>({
    open: false,
    query: '',
    anchorRect: null,
  })

  useEffect(() => { onSubmitRef.current = onSubmit }, [onSubmit])
  useEffect(() => { onSlashCommandRef.current = onSlashCommand }, [onSlashCommand])

  const handleSlashCommand = useCallback((cmd: SlashCommand) => {
    const view = viewRef.current
    if (view) {
      const { from } = view.state.selection.main
      const line = view.state.doc.lineAt(from)
      const textBefore = line.text.slice(0, from - line.from)
      const slashIdx = textBefore.lastIndexOf('/')
      if (slashIdx !== -1) {
        view.dispatch({
          changes: { from: line.from + slashIdx, to: from, insert: cmd.id },
        })
      }
    }
    setSlashState({ open: false, query: '', anchorRect: null })
    onSlashCommandRef.current(cmd)
  }, [])

  useEffect(() => {
    if (!containerRef.current) return

    const docContent = turnsToDocument(turns)

    // Use Prec.highest so this keymap beats markdown() and defaultKeymap
    const submitKeymap = Prec.highest(keymap.of([
      {
        key: 'Mod-Enter',
        run(view) {
          const doc = view.state.doc
          const inputPos = getInputPos(doc)
          const userInput = doc.sliceString(inputPos, doc.length).trim()
          if (userInput) onSubmitRef.current(userInput)
          return true
        },
      },
    ]))

    const state = EditorState.create({
      doc: docContent,
      extensions: [
        markdown(),
        history(),
        keymap.of([...defaultKeymap, ...historyKeymap]),
        submitKeymap,
        turnBoundaries(streamingTurnId),
        livePreview(),
        streamingExt(),
        todoStickyExt(),
        widgetExt(getTurns, onWidgetChange),
        slashCommandExt(setSlashState),
        EditorView.theme({
          '&': { height: '100%', width: '100%', fontSize: '14px' },
          '.cm-scroller': {
            overflow: 'auto',
            fontFamily: 'var(--font-sans, system-ui)',
          },
          '.cm-content': { padding: '16px', minHeight: '100%' },
          '.cm-line': { lineHeight: '1.7' },
          '.cm-editor': { height: '100%', width: '100%' },
          '.cm-widget-container': { margin: '8px 0' },
          // Input area gets a subtle top border to separate it from read-only content
          '.cm-line:last-child': { borderTop: '1px dashed oklch(0 0 0 / 10%)' },
        }),
        EditorView.lineWrapping,
      ],
    })

    const view = new EditorView({ state, parent: containerRef.current })
    viewRef.current = view
    onEditorReady(view)

    return () => {
      view.destroy()
      viewRef.current = null
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []) // Mount once — turns update via INPUT_MARKER, streaming happens imperatively

  return (
    <div className="relative flex min-h-0 flex-1 flex-col overflow-hidden">
      <div ref={containerRef} className="min-h-0 w-full flex-1" />
      {slashState.open && slashState.anchorRect && (
        <SlashCommandPalette
          query={slashState.query}
          anchorRect={slashState.anchorRect}
          onSelect={handleSlashCommand}
          onClose={() => setSlashState({ open: false, query: '', anchorRect: null })}
        />
      )}
    </div>
  )
}
