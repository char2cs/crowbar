import { useRef, useState, type CSSProperties, type KeyboardEvent } from 'react'
import { promoteDraftThread } from '@/components/layout/draft-promote'
import { AgentEmptyDocument } from '@/features/agent/chat/agent-empty-document'
import { isSendKey } from '@/features/agent/chat/lib/send-key'
import {
  ChatMarkdownEditor,
  type ChatMarkdownEditorHandle,
} from '@/features/agent/composer/plate/chat-markdown-editor'
import { AgentSelectionPicker } from '@/features/agent/controls/agent-selection-picker'
import { usePaneProviders } from '@/features/agent/hooks/use-pane-providers'
import { refusal } from '@/features/agent/stores/lib/outbox'
import { CHROME_ROW_HEIGHT_PX } from '@/lib/chrome-metrics'

interface DraftChatPaneProps {
  paneId: string
  wsId: string
  hostId: string
  belowOverlayHeader: boolean
}

/** A thread nobody has written to yet: the blank chat, with no chat behind it.
 *  Sending creates the chat and hands it this prompt; closing the pane leaves
 *  nothing on the daemon or in the sidebar. */
export function DraftChatPane({ paneId, wsId, hostId, belowOverlayHeader }: DraftChatPaneProps) {
  const providers = usePaneProviders(wsId, hostId)
  const editorRef = useRef<ChatMarkdownEditorHandle>(null)
  const docRef = useRef<HTMLDivElement>(null)
  const [hasText, setHasText] = useState(false)
  // Re-renders the document so its handle rides the last line as the box grows.
  const [, setFieldHeight] = useState(0)
  const [pick, setPick] = useState({ provider: '', model: '', effort: '' })
  const [sending, setSending] = useState(false)
  const [error, setError] = useState('')
  const provider = providers.find((p) => p.id === pick.provider) ?? providers.find((p) => p.enabled)
  const headerClearancePx = belowOverlayHeader ? CHROME_ROW_HEIGHT_PX + 8 : 8

  const send = (raw: string) => {
    const prompt = raw.trim()
    if (!prompt || sending) return
    const refused = refusal(prompt)
    if (refused) {
      setError(refused)
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
      // `agent-chat-pane` is the scope the picker's chip and cluster are styled under.
      className="agent-chat agent-chat-pane chat"
      aria-label="Agent chat"
      style={{ '--agent-header-clearance': `${headerClearancePx}px` } as CSSProperties}
    >
      {/* No chat exists yet, so the box takes no attachments: it has no chat to upload to. */}
      <div className="docwrap" data-testid="agent-empty-document">
        <div ref={docRef} className="doc">
          <ChatMarkdownEditor
            ref={editorRef}
            placeholder="Describe the change…"
            ariaLabel="Describe the change"
            autoFocus
            onShapeChange={(shape) => setHasText(!shape.empty)}
            onKeyDown={handleKeyDown}
            onHeightChange={setFieldHeight}
            className="blk"
          />
        </div>
        <AgentEmptyDocument
          docRef={docRef}
          hasText={hasText}
          onSubmit={() => send(editorRef.current?.getMarkdown() ?? '')}
          attach={null}
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
      </div>
      {error && (
        <p className="meta composer-note" role="alert">
          {error}
        </p>
      )}
    </section>
  )
}
