import { useRef, useState, type CSSProperties, type KeyboardEvent } from 'react'
import type { PermissionLevel } from '@/features/agent/api/agent-providers-api'
import { promoteDraftThread } from '@/components/layout/draft-promote'
import { draftOfPane } from '@/components/layout/draft-thread'
import { AgentEmptyDocument } from '@/features/agent/chat/agent-empty-document'
import { isSendKey } from '@/features/agent/chat/lib/send-key'
import {
  ChatMarkdownEditor,
  type ChatMarkdownEditorHandle,
} from '@/features/agent/composer/plate/chat-markdown-editor'
import { ViewSwitch } from '@/features/agent/controls/view-switch'
import { providerCanStartOnTerminal } from '@/features/agent/api/agent-providers-api'
import { IS_MOBILE } from '@/utils/platform'
import { SelectionCluster } from '@/features/agent/controls/selection-cluster'
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
  const [pick, setPick] = useState<{
    provider: string
    model: string
    effort: string
    permissionLevel: PermissionLevel | ''
  }>({ provider: '', model: '', effort: '', permissionLevel: '' })
  const [sending, setSending] = useState(false)
  const [error, setError] = useState('')
  const provider = providers.find((p) => p.id === pick.provider) ?? providers.find((p) => p.enabled)
  const needsModel = provider?.modelRequired === true && !pick.model
  const headerClearancePx = belowOverlayHeader ? CHROME_ROW_HEIGHT_PX + 8 : 8

  const send = (raw: string) => {
    const prompt = raw.trim()
    if (!prompt || sending || needsModel) return
    const refused = refusal(prompt)
    if (refused) {
      setError(refused)
      return
    }
    setError('')
    setSending(true)
    // The box empties as the prompt leaves; what is typed while the chat is created is
    // the chat's next draft. A refused create gives the prompt back, ahead of that text.
    const editor = editorRef.current
    const draftId = draftOfPane(paneId)?.draftId
    editor?.clear()
    const typedSince = () => editor?.getMarkdown() ?? ''
    void promoteDraftThread(paneId, { ...pick, text: prompt }, typedSince).finally(() => {
      setSending(false)
      if (draftOfPane(paneId)?.draftId !== draftId) return
      const rest = typedSince().trim()
      editor?.setMarkdown(rest ? `${prompt}\n\n${rest}` : prompt)
    })
  }

  // A draft is always on Chat; Terminal makes the chat in its terminal, where the box's text
  // stays as the draft. The segment then belongs to that chat, never to the draft.
  const openTerminal = () => {
    if (sending) return
    setSending(true)
    const editor = editorRef.current
    void promoteDraftThread(
      paneId,
      { ...pick, presentation: 'terminal', text: '' },
      () => editor?.getMarkdown() ?? '',
    ).finally(() => setSending(false))
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
            document
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
            <>
              <SelectionCluster
                provider={provider}
                providers={providers}
                model={pick.model}
                effort={pick.effort}
                reportedModel={provider?.defaultModel}
                switchDisabled={sending}
                permission={{
                  wsId,
                  chatId: '',
                  levels: provider?.permissionLevels,
                  draft: {
                    hostId,
                    level: pick.permissionLevel,
                    onPick: (permissionLevel) => setPick((was) => ({ ...was, permissionLevel })),
                  },
                }}
                onSelectionChange={(next, model, effort) =>
                  setPick((was) => ({ ...was, provider: next, model, effort }))
                }
              />
              {IS_MOBILE && providerCanStartOnTerminal(provider) && provider?.hasTerminal && (
                <ViewSwitch
                  presentation="chat"
                  handoverBlocked={sending}
                  onSelect={(next) => next === 'terminal' && openTerminal()}
                />
              )}
            </>
          }
          sending={sending}
          headerClearancePx={headerClearancePx}
        />
      </div>
      {(needsModel || error) && (
        <p className="meta composer-note" role="alert">
          {needsModel ? 'Choose a model to start' : error}
        </p>
      )}
    </section>
  )
}
