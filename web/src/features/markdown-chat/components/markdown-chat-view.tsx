import { useEffect, useRef, useCallback, useState } from 'react'
import { useStore } from 'zustand'
import { nanoid } from 'nanoid'
import type { EditorView } from '@codemirror/view'
import { appendStreamChunk, finalizeStreaming } from '../extensions/streaming-ext'
import { appendTurnToHistory } from '../extensions/turn-boundaries'
import { getOrCreateConversationStore } from '../stores/conversation-store'
import { getMockMarkdownTurns, simulateMarkdownStream } from '@/lib/mock/markdown-chat'
import { MarkdownChatHistory } from './markdown-chat-history'
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

export function MarkdownChatView({ workspaceId, stepId }: MarkdownChatViewProps) {
  const store = getOrCreateConversationStore(workspaceId)
  const turns = useStore(store, (s) => s.turns)
  const historyViewRef = useRef<EditorView | null>(null)
  const [inputEditorView, setInputEditorView] = useState<EditorView | null>(null)
  const [isStreaming, setIsStreaming] = useState(false)
  const cancelStreamRef = useRef<(() => void) | null>(null)
  const draftWidgetsRef = useRef<WidgetData[]>([])

  // getTurns is used by widgetExt in both CM6 instances to look up widget payloads.
  // When the user has inserted widgets in the input zone before submitting, they live
  // in draftWidgetsRef (not yet in any store turn). We expose them via a synthetic
  // '__input_draft__' turn so FencedWidget.toDOM() can find them. This turn has no
  // content/timestamp/authorName — callers that need well-formed turns must filter it.
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
          widgets: [],
        })
      }
    }

    return () => { cancelStreamRef.current?.() }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [workspaceId, stepId])

  // NOTE: handleSubmit does NOT clear the input CM6. Callers are responsible:
  // - Mod-Enter keymap in MarkdownChatInput clears via its own dispatch
  // - handleSendClick clears after calling handleSubmit
  // Any future callsite must also clear the input editor itself.
  const handleSubmit = useCallback(
    (content: string) => {
      cancelStreamRef.current?.()
      const historyView = historyViewRef.current
      if (!historyView) return

      const state = store.getState()
      const userId = nanoid()
      const agentId = nanoid()

      // Collect and clear draft widgets — they belong to this user turn
      const inputWidgets = [...draftWidgetsRef.current]
      draftWidgetsRef.current = []

      // Update store
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
        widgets: [],
        streaming: true,
      })

      // Update history CM6 document imperatively
      appendTurnToHistory(historyView, userId, 'user', content)
      appendTurnToHistory(historyView, agentId, 'agent', '')

      setIsStreaming(true)
      cancelStreamRef.current = simulateMarkdownStream(
        MOCK_RESPONSE,
        (chunk) => {
          state.updateStreamingTurn(agentId, chunk)
          appendStreamChunk(historyView, chunk)
        },
        () => {
          state.finalizeStreamingTurn(agentId)
          finalizeStreaming(historyView)
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

  const handleHistoryReady = useCallback((view: EditorView) => {
    historyViewRef.current = view
  }, [])

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

  if (turns.length === 0) {
    return (
      <div className="flex h-full items-center justify-center text-muted-foreground">
        <p className="text-sm">Loading conversation…</p>
      </div>
    )
  }

  return (
    <div className="flex h-full w-full flex-col overflow-hidden">
      {/* History: flex-1 scrollable document */}
      <div className="min-h-0 flex-1 overflow-hidden">
        <MarkdownChatHistory
          turns={turns}
          getTurns={getTurns}
          onWidgetChange={handleWidgetChange}
          onReady={handleHistoryReady}
        />
      </div>

      {/* Input zone: same warm tint as user turns */}
      <div className="flex-shrink-0 border-t border-primary/10 bg-primary/5">
        <MarkdownChatInput
          getTurns={getTurns}
          onSubmit={handleSubmit}
          onWidgetChange={handleWidgetChange}
          onEditorReady={handleInputReady}
          onSlashCommand={handleSlashCommand}
        />
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
