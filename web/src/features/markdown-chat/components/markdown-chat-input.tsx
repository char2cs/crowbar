import { useEffect, useRef, useState, useCallback } from 'react'
import { EditorState, Prec } from '@codemirror/state'
import { EditorView, keymap } from '@codemirror/view'
import { markdown } from '@codemirror/lang-markdown'
import { history, defaultKeymap, historyKeymap } from '@codemirror/commands'
import type { MarkdownTurn } from '../types'
import { livePreview } from '../extensions/live-preview'
import { codeBlockExt } from '../extensions/code-block'
import { codeLanguages } from '../extensions/code-languages'
import { widgetExt } from '../extensions/widget-ext'
import { slashCommandExt, type SlashCommandState } from '../extensions/slash-command-ext'
import { SlashCommandPalette, type SlashCommand } from './slash-command-palette'
import { getMockSlashCommands } from '@/lib/mock/markdown-chat'
import './excalidraw-widget'
import './mermaid-widget'

interface MarkdownChatInputProps {
  getTurns: () => MarkdownTurn[]
  onSubmit: (content: string) => void
  onWidgetChange: (widgetId: string, payload: unknown) => void
  onEditorReady: (view: EditorView) => void
  onSlashCommand?: (cmd: SlashCommand) => void
}

export function MarkdownChatInput({
  getTurns,
  onSubmit,
  onWidgetChange,
  onEditorReady,
  onSlashCommand,
}: MarkdownChatInputProps) {
  const containerRef = useRef<HTMLDivElement>(null)
  const onSubmitRef = useRef(onSubmit)
  const onSlashCommandRef = useRef(onSlashCommand)
  const viewRef = useRef<EditorView | null>(null)
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
        view.dispatch({ changes: { from: line.from + slashIdx, to: from, insert: cmd.id } })
      }
    }
    setSlashState({ open: false, query: '', anchorRect: null })
    onSlashCommandRef.current?.(cmd)
  }, [])

  useEffect(() => {
    if (!containerRef.current) return

    const submitKeymap = Prec.highest(
      keymap.of([
        {
          key: 'Mod-Enter',
          run(view) {
            const content = view.state.doc.toString().trim()
            if (content) {
              onSubmitRef.current(content)
              view.dispatch({
                changes: { from: 0, to: view.state.doc.length, insert: '' },
              })
            }
            return true
          },
        },
      ]),
    )

    const state = EditorState.create({
      doc: '',
      extensions: [
        markdown({ codeLanguages }),
        history(),
        keymap.of([...defaultKeymap, ...historyKeymap]),
        submitKeymap,
        livePreview(),
        codeBlockExt(),
        widgetExt(getTurns, onWidgetChange),
        slashCommandExt(setSlashState),
        EditorView.lineWrapping,
        EditorView.theme({
          // Text metrics should stay close to the history renderer (markdown/markdown-history.tsx).
          '&': { fontSize: '15px', height: 'auto', maxHeight: '240px' },
          '&.cm-focused': { outline: 'none' },
          '.cm-cursor, .cm-dropCursor': { borderLeftColor: 'var(--foreground)' },
          '.cm-scroller': {
            overflow: 'auto',
            fontFamily: 'var(--font-sans, system-ui)',
            scrollbarGutter: 'stable',
            scrollbarWidth: 'thin',
            scrollbarColor: 'var(--app-scrollbar-thumb) var(--app-scrollbar-track)',
          },
          // Same column model as history: vertical padding on content, the
          // centered column padding on each line.
          '.cm-content': {
            padding: '8px 0',
            minHeight: '52px',
            minWidth: '100%',
            caretColor: 'var(--foreground)',
          },
          '.cm-line': {
            padding: '0 max(48px, calc((100% - 680px) / 2 + 48px))',
            lineHeight: '1.75',
          },
        }),
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
  }, [])

  return (
    <div className="relative">
      <div ref={containerRef} className="w-full" />
      {slashState.open && slashState.anchorRect && (
        <SlashCommandPalette
          commands={getMockSlashCommands()}
          query={slashState.query}
          anchorRect={slashState.anchorRect}
          onSelect={handleSlashCommand}
          onClose={() => setSlashState({ open: false, query: '', anchorRect: null })}
        />
      )}
    </div>
  )
}
