import { useEffect, useRef, useCallback, useState } from 'react'
import { useStore } from 'zustand'
import { nanoid } from 'nanoid'
import type { EditorView } from '@codemirror/view'
import { getOrCreateConversationStore } from '../stores/conversation-store'
import { wsManager } from '@/lib/ws/manager'
import { useChatStore } from '@/features/markdown-chat/stores/chat-store'
import { dataOf } from '@/lib/loadable'
import { MarkdownHistory } from './markdown/markdown-history'
import { MarkdownChatInput } from './markdown-chat-input'
import { MarkdownChatToolbar } from './markdown-chat-toolbar'
import type { SlashCommand } from './slash-command-palette'
import type { MarkdownTurn, WidgetData } from '../types'

interface MarkdownChatViewProps {
  workspaceId: string
  stepId: string
}

export function MarkdownChatView({ workspaceId, stepId }: MarkdownChatViewProps) {
  const store = getOrCreateConversationStore(workspaceId)
  const turns = useStore(store, (s) => s.turns)
  const [inputEditorView, setInputEditorView] = useState<EditorView | null>(null)
  const [isStreaming, setIsStreaming] = useState(false)
  const cancelStreamRef = useRef<(() => void) | null>(null)
  const draftWidgetsRef = useRef<WidgetData[]>([])
  const historyScrollRef = useRef<HTMLDivElement>(null)
  const prevTurnCountRef = useRef(0)

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

  // Seed turns on mount from API — only when the store is empty (cold start)
  const chatLoadable = useChatStore(s => s.data)
  const initialTurns = dataOf(chatLoadable)
  useEffect(() => { void useChatStore.getState().fetch(workspaceId, stepId) }, [workspaceId, stepId])
  useEffect(() => {
    const state = store.getState()
    if (!initialTurns || state.turns.length > 0) return
    initialTurns.forEach(t => state.appendTurn(t))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [initialTurns])

  // Cleanup streaming on unmount/workspace change
  useEffect(() => {
    return () => { cancelStreamRef.current?.() }
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

      // Per-chat content stream (content/done frames). stepId identifies the
      // chat. Real route: /v0/ws/chats/:chatId/stream.
      const endpoint = `/v0/ws/chats/${stepId}/stream`
      setIsStreaming(true)
      const unsubscribe = wsManager.subscribe(endpoint, (msg: unknown) => {
        const m = msg as { content: string; done: boolean }
        if (!m.done) {
          state.updateStreamingTurn(agentId, m.content)
        } else {
          state.finalizeStreamingTurn(agentId)
          cancelStreamRef.current = null
          setIsStreaming(false)
          unsubscribe()
        }
      })
      wsManager.send(endpoint, { turnId: agentId, message: content })
      cancelStreamRef.current = unsubscribe
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

  // Auto-follow the conversation: jump to the bottom when a new turn is added
  // (you just sent), and follow streaming output — unless you've scrolled up.
  useEffect(() => {
    const el = historyScrollRef.current
    if (!el) return
    const grewByTurn = turns.length > prevTurnCountRef.current
    prevTurnCountRef.current = turns.length
    const nearBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 160
    if (grewByTurn || (isStreaming && nearBottom)) {
      el.scrollTop = el.scrollHeight
    }
  }, [turns, isStreaming])

  // No empty-state placeholder: when there are no turns, the editable tail simply
  // fills the whole canvas (like opening a blank document).
  return (
    <div className="@container flex h-full w-full flex-col overflow-hidden">
      {/* Rendered turns — only when there are any (an empty conversation shows
          just the input canvas, no blank gap at the top). Scrolls internally. */}
      {turns.length > 0 && (
        <div
          ref={historyScrollRef}
          className="min-h-0 shrink grow-0 overflow-y-auto"
          style={{
            // both-edges reserves the scrollbar gutter symmetrically, keeping the
            // centered column (and its rails) aligned with the input's column
            // whether or not a scrollbar is present.
            scrollbarGutter: 'stable both-edges',
            scrollbarWidth: 'thin',
            scrollbarColor: 'var(--app-scrollbar-thumb) var(--app-scrollbar-track)',
          }}
        >
          <MarkdownHistory turns={turns} onWidgetChange={handleWidgetChange} />
        </div>
      )}

      {/* Editable tail — tinted canvas that fills the viewport when short and
          docks at the bottom otherwise. The input fills the region (scrolls
          internally when long); the toolbar is pinned at the bottom. */}
      <div
        className="relative flex min-h-[120px] shrink grow flex-col overflow-hidden"
        style={{ background: 'color-mix(in srgb, var(--primary) 10%, transparent)' }}
      >
        {/* Column rails — primary, since the input is the user's turn. */}
        <div
          aria-hidden
          className="pointer-events-none absolute inset-y-0 w-px"
          style={{ left: 'max(48px, calc((100% - 680px) / 2))', background: 'color-mix(in srgb, var(--primary) 35%, transparent)' }}
        />
        <div
          aria-hidden
          className="pointer-events-none absolute inset-y-0 w-px"
          style={{ right: 'max(48px, calc((100% - 680px) / 2))', background: 'color-mix(in srgb, var(--primary) 35%, transparent)' }}
        />
        <div className="min-h-0 flex-1">
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
