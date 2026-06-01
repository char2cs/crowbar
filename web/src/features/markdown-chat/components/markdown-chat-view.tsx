import { useEffect, useRef, useCallback, useState } from 'react'
import { useStore } from 'zustand'
import { nanoid } from 'nanoid'
import type { EditorView } from '@codemirror/view'
import { getOrCreateConversationStore } from '../stores/conversation-store'
import { getMockMarkdownTurns, simulateMarkdownStream } from '@/lib/mock/markdown-chat'
import { MarkdownHistory } from './markdown/markdown-history'
import { MarkdownChatInput } from './markdown-chat-input'
import { MarkdownChatToolbar } from './markdown-chat-toolbar'
import type { SlashCommand } from './slash-command-palette'
import type { MarkdownTurn, WidgetData } from '../types'

interface MarkdownChatViewProps {
  workspaceId: string
  stepId: string
}

const STEP_GREETINGS: Record<string, string> = {
  brainstorm: "I'm ready to brainstorm. What do you want to build?",
  spec: "Let's refine the spec. What would you like to discuss?",
  build: 'Ready to implement. What should we tackle first?',
  ai_review: "I've reviewed the diff. Here's what I found.",
  human_review: 'Waiting for your review comments.',
}

const MOCK_RESPONSE =
  'Great point. Let me think through this carefully.\n\n' +
  'There are several considerations here:\n\n' +
  '1. **Performance** — the current approach has O(n²) complexity\n' +
  '2. **Correctness** — edge cases around empty inputs\n' +
  '3. **Maintainability** — the code is hard to follow\n\n' +
  'My recommendation is to refactor the core loop first.'

// Left/right edge of the centered text column — shared by the history grid, the
// input's line padding, and the vertical rails so everything lines up.
const COLUMN_EDGE = 'max(48px, calc((100% - 680px) / 2))'

export function MarkdownChatView({ workspaceId, stepId }: MarkdownChatViewProps) {
  const store = getOrCreateConversationStore(workspaceId)
  const turns = useStore(store, (s) => s.turns)
  const [inputEditorView, setInputEditorView] = useState<EditorView | null>(null)
  const [isStreaming, setIsStreaming] = useState(false)
  const cancelStreamRef = useRef<(() => void) | null>(null)
  const draftWidgetsRef = useRef<WidgetData[]>([])

  // getTurns is used by widgetExt in the input editor to look up widget payloads.
  // Widgets inserted in the input zone before submitting live in draftWidgetsRef
  // (not yet in any store turn); we expose them via a synthetic '__input_draft__'
  // turn so the input's FencedWidget can find them. Callers needing well-formed
  // turns must filter this one (it has no content/timestamp/authorName).
  const getTurns = useCallback((): MarkdownTurn[] => {
    const storeTurns = store.getState().turns
    const draft = draftWidgetsRef.current
    if (draft.length === 0) return storeTurns
    return [...storeTurns, {
      id: '__input_draft__',
      role: 'user' as const,
      content: '',
      timestamp: '',
      authorName: '',
      widgets: draft,
    }]
  }, [store])

  // Seed turns on mount
  useEffect(() => {
    const state = store.getState()
    if (state.turns.length > 0) return

    const mockTurns = getMockMarkdownTurns(workspaceId, stepId)
    if (mockTurns.length > 0) {
      mockTurns.forEach((t) => state.appendTurn(t))
    } else {
      const greeting = STEP_GREETINGS[stepId]
      if (greeting) {
        state.appendTurn({
          id: nanoid(),
          role: 'agent',
          content: greeting,
          timestamp: new Date().toISOString(),
          authorName: 'Claude',
          model: 'Opus 4.8',
          widgets: [],
        })
      }
    }

    return () => { cancelStreamRef.current?.() }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [workspaceId, stepId])

  // Store-driven: the React history (MarkdownHistory) renders reactively from the
  // store, so submitting only updates the store — no imperative view calls.
  const handleSubmit = useCallback(
    (content: string) => {
      cancelStreamRef.current?.()

      const state = store.getState()
      const userId = nanoid()
      const agentId = nanoid()

      // Collect and clear draft widgets — they belong to this user turn
      const inputWidgets = [...draftWidgetsRef.current]
      draftWidgetsRef.current = []

      state.appendTurn({
        id: userId,
        role: 'user',
        content,
        timestamp: new Date().toISOString(),
        authorName: 'You',
        widgets: inputWidgets,
      })
      state.appendTurn({
        id: agentId,
        role: 'agent',
        content: '',
        timestamp: new Date().toISOString(),
        authorName: 'Claude',
        model: 'Opus 4.8',
        widgets: [],
        streaming: true,
      })

      setIsStreaming(true)
      cancelStreamRef.current = simulateMarkdownStream(
        MOCK_RESPONSE,
        (chunk) => state.updateStreamingTurn(agentId, chunk),
        () => {
          state.finalizeStreamingTurn(agentId)
          cancelStreamRef.current = null
          setIsStreaming(false)
        },
      )
    },
    [store],
  )

  const handleWidgetChange = useCallback(
    (widgetId: string, payload: unknown) => {
      // Check draft widgets first (input-zone widgets not yet submitted)
      const draftIdx = draftWidgetsRef.current.findIndex((w) => w.id === widgetId)
      if (draftIdx !== -1) {
        draftWidgetsRef.current = draftWidgetsRef.current.map((w) =>
          w.id === widgetId ? { ...w, payload } : w,
        )
        return
      }
      // Fall through to persisted store turns
      const { turns: currentTurns } = store.getState()
      const turn = currentTurns.find((t) => t.widgets.some((w) => w.id === widgetId))
      if (turn) store.getState().updateWidgetPayload(turn.id, widgetId, payload)
    },
    [store],
  )

  const handleInsertWidget = useCallback(
    (widgetType: string, widgetId: string) => {
      draftWidgetsRef.current = [
        ...draftWidgetsRef.current,
        { id: widgetId, type: widgetType, payload: null },
      ]
    },
    [],
  )

  const handleSlashCommand = useCallback(
    (cmd: SlashCommand) => {
      handleSubmit(cmd.id)
    },
    [handleSubmit],
  )

  const handleInputReady = useCallback((view: EditorView) => {
    setInputEditorView(view)
  }, [])

  const handleSendClick = useCallback(() => {
    const view = inputEditorView
    if (!view) return
    const content = view.state.doc.toString().trim()
    if (content) {
      handleSubmit(content)
      view.dispatch({ changes: { from: 0, to: view.state.doc.length, insert: '' } })
    }
  }, [inputEditorView, handleSubmit])

  const handleStop = useCallback(() => {
    cancelStreamRef.current?.()
    cancelStreamRef.current = null
    setIsStreaming(false)
  }, [])

  // No empty-state placeholder: when there are no turns, the editable tail simply
  // fills the whole canvas (like opening a blank document).
  return (
    <div className="@container relative flex h-full w-full flex-col overflow-hidden">
      {/* Continuous vertical rails at the text column edges */}
      <div
        aria-hidden
        className="pointer-events-none absolute inset-y-0 z-10 w-px"
        style={{ left: COLUMN_EDGE, background: 'color-mix(in srgb, var(--foreground) 12%, transparent)' }}
      />
      <div
        aria-hidden
        className="pointer-events-none absolute inset-y-0 z-10 w-px"
        style={{ right: COLUMN_EDGE, background: 'color-mix(in srgb, var(--foreground) 12%, transparent)' }}
      />

      {/* Rendered turns — scroll internally; don't grow, so the composer fills slack */}
      <div
        className="min-h-0 shrink grow-0 overflow-y-auto"
        style={{
          // both-edges keeps the centered column aligned with the rails/input
          // whether or not a scrollbar is present.
          scrollbarGutter: 'stable both-edges',
          scrollbarWidth: 'thin',
          scrollbarColor: 'var(--app-scrollbar-thumb) var(--app-scrollbar-track)',
        }}
      >
        <MarkdownHistory turns={turns} onWidgetChange={handleWidgetChange} />
      </div>

      {/* Editable tail — the live end of the canvas. Grows to fill the viewport
          when the conversation is short; stays docked at the bottom otherwise. */}
      <div
        className="flex shrink-0 grow flex-col border-t border-primary/10"
        style={{ background: 'color-mix(in srgb, var(--primary) 10%, transparent)' }}
      >
        <div className="min-h-[88px] flex-1">
          <MarkdownChatInput
            getTurns={getTurns}
            onSubmit={handleSubmit}
            onWidgetChange={handleWidgetChange}
            onEditorReady={handleInputReady}
            onSlashCommand={handleSlashCommand}
          />
        </div>
        <MarkdownChatToolbar
          editorView={inputEditorView}
          onInsertWidget={handleInsertWidget}
          onSubmit={handleSendClick}
          isStreaming={isStreaming}
          onStop={handleStop}
        />
      </div>
    </div>
  )
}
