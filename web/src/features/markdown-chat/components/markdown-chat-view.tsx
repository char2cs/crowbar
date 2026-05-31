import { useEffect, useRef, useCallback } from 'react'
import { useStore } from 'zustand'
import { nanoid } from 'nanoid'
import type { EditorView } from '@codemirror/view'
import { appendStreamChunk, finalizeStreaming } from '../extensions/streaming-ext'
import { getOrCreateConversationStore } from '../stores/conversation-store'
import { getMockMarkdownTurns, simulateMarkdownStream } from '@/lib/mock/markdown-chat'
import { MarkdownChatEditor } from './markdown-chat-editor'
import { MarkdownChatToolbar } from './markdown-chat-toolbar'
import type { SlashCommand } from './slash-command-palette'

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
  const editorViewRef = useRef<EditorView | null>(null)
  const toolbarEditorRef = useRef<EditorView | null>(null)
  const cancelStreamRef = useRef<(() => void) | null>(null)

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

  const handleSubmit = useCallback(
    (content: string) => {
      cancelStreamRef.current?.()
      const state = store.getState()

      state.appendTurn({
        id: nanoid(),
        role: 'user',
        content,
        timestamp: new Date().toISOString(),
        authorName: 'You',
        widgets: [],
      })

      const agentId = nanoid()
      state.appendTurn({
        id: agentId,
        role: 'agent',
        content: '',
        timestamp: new Date().toISOString(),
        authorName: 'Claude',
        widgets: [],
        streaming: true,
      })

      cancelStreamRef.current = simulateMarkdownStream(
        MOCK_RESPONSE,
        (chunk) => {
          state.updateStreamingTurn(agentId, chunk)
          if (editorViewRef.current) appendStreamChunk(editorViewRef.current, chunk)
        },
        () => {
          state.finalizeStreamingTurn(agentId)
          if (editorViewRef.current) finalizeStreaming(editorViewRef.current)
          cancelStreamRef.current = null
        },
      )
    },
    [store],
  )

  const handleWidgetChange = useCallback(
    (widgetId: string, payload: unknown) => {
      const { turns: currentTurns } = store.getState()
      const turn = currentTurns.find((t) => t.widgets.some((w) => w.id === widgetId))
      if (turn) store.getState().updateWidgetPayload(turn.id, widgetId, payload)
    },
    [store],
  )

  const handleInsertWidget = useCallback(
    (_widgetType: string, widgetId: string) => {
      const { turns: currentTurns } = store.getState()
      const lastTurn = currentTurns.at(-1)
      if (lastTurn?.role === 'user') {
        store.getState().updateWidgetPayload(lastTurn.id, widgetId, null)
      }
    },
    [store],
  )

  const handleSlashCommand = useCallback(
    (cmd: SlashCommand) => {
      handleSubmit(cmd.id)
    },
    [handleSubmit],
  )

  const handleEditorReady = useCallback((view: EditorView) => {
    editorViewRef.current = view
    toolbarEditorRef.current = view
  }, [])

  const streamingTurnId = turns.find((t) => t.streaming)?.id ?? null

  if (turns.length === 0) {
    return (
      <div className="flex h-full items-center justify-center text-muted-foreground">
        <p className="text-sm">Loading conversation…</p>
      </div>
    )
  }

  return (
    <div className="flex h-full flex-col overflow-hidden">
      <MarkdownChatEditor
        turns={turns}
        streamingTurnId={streamingTurnId}
        onSubmit={handleSubmit}
        onWidgetChange={handleWidgetChange}
        onSlashCommand={handleSlashCommand}
        onEditorReady={handleEditorReady}
      />
      <MarkdownChatToolbar
        editorView={toolbarEditorRef.current}
        onInsertWidget={handleInsertWidget}
      />
    </div>
  )
}
