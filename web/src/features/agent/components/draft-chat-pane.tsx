import { useState, type CSSProperties, type KeyboardEvent } from 'react'
import { promoteDraftThread } from '@/components/layout/draft-promote'
import { AgentEmptyDocument } from '@/features/agent/chat/agent-empty-document'
import { isSendKey } from '@/features/agent/chat/lib/send-key'
import { AgentSelectionPicker } from '@/features/agent/controls/agent-selection-picker'
import { usePaneProviders } from '@/features/agent/hooks/use-pane-providers'
import { isPromptTextWithinLimit } from '@/features/agent/lib/prompt-queue-persistence'
import { useWorkspaceStore } from '@/features/workspace/stores/workspace-context'
import { CHROME_ROW_HEIGHT_PX } from '@/lib/chrome-metrics'

interface DraftChatPaneProps {
  paneId: string
  wsId: string
  hostId: string
  belowOverlayHeader: boolean
}

/**
 * A thread nobody has written to yet: the blank chat, with no chat behind it.
 * Sending creates the chat and hands it this prompt; closing the pane leaves
 * nothing on the daemon or in the sidebar.
 */
export function DraftChatPane({ paneId, wsId, hostId, belowOverlayHeader }: DraftChatPaneProps) {
  const store = useWorkspaceStore()
  const providers = usePaneProviders(store, wsId, hostId)
  const [text, setText] = useState('')
  const [pick, setPick] = useState({ provider: '', model: '', effort: '' })
  const [sending, setSending] = useState(false)
  const [error, setError] = useState('')
  const provider = providers.find((p) => p.id === pick.provider) ?? providers.find((p) => p.enabled)
  const headerClearancePx = belowOverlayHeader ? CHROME_ROW_HEIGHT_PX + 8 : 8

  const send = (raw: string) => {
    const prompt = raw.trim()
    if (!prompt || sending) return
    if (!isPromptTextWithinLimit(prompt)) {
      setError('This prompt is too large to submit.')
      return
    }
    setError('')
    setSending(true)
    void promoteDraftThread(paneId, { ...pick, text: prompt }).finally(() => setSending(false))
  }

  const handleKeyDown = (event: KeyboardEvent<HTMLDivElement>, readMarkdown: () => string) => {
    if (!isSendKey(event)) return
    event.preventDefault()
    send(readMarkdown())
  }

  return (
    <section
      className="agent-chat chat"
      aria-label="Agent chat"
      style={{ '--agent-header-clearance': `${headerClearancePx}px` } as CSSProperties}
    >
      <AgentEmptyDocument
        draft={text}
        draftSeed={0}
        hasText={text.trim().length > 0}
        onDraftChange={setText}
        onSubmit={() => send(text)}
        onKeyDown={handleKeyDown}
        controls={
          <span className="selpos">
            <AgentSelectionPicker
              provider={provider}
              providers={providers}
              model={pick.model}
              effort={pick.effort}
              disabled={sending}
              onSelectionChange={(next, model, effort) =>
                setPick({ provider: next, model, effort })
              }
              unsetLabel="Default"
            />
          </span>
        }
        sending={sending}
        headerClearancePx={headerClearancePx}
      />
      {error && (
        <p className="meta composer-note" role="alert">
          {error}
        </p>
      )}
    </section>
  )
}
