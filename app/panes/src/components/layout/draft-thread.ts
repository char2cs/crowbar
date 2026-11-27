import type { PermissionLevel } from '@/features/agent/api/agent-providers-api'
import { setChatPermissionLevel } from '@/features/agent/api/agent-chat-commands'
import { report, UNREACHED } from '@/lib/errors/report'
import { saveDraft } from '@/features/agent/composer/lib/draft-persistence'
import { sendPrompt } from '@/features/agent/stores/chat-outbox'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import type { LandingChatPresentation } from '@/features/settings/lib/chat-presentation'

export interface DraftSpec {
  paneId: string
  draftId: string
  projectId: string
  workspaceId: string
  parentId: string
}

export interface DraftPrompt {
  text: string
  provider: string
  model: string
  effort: string
  permissionLevel?: PermissionLevel | ''
  presentation?: LandingChatPresentation
}

export function draftOfPane(paneId: string): DraftSpec | null {
  const { panes, views } = windowPaneStore.getState()
  const pane = panes[paneId]
  if (!pane?.chatId || !pane.workspaceId || !pane.viewId) return null
  const projectId = views[pane.viewId]?.projectId
  if (!projectId) return null
  return {
    paneId,
    draftId: pane.chatId,
    projectId,
    workspaceId: pane.workspaceId,
    parentId: pane.draftParentId ?? '',
  }
}

export function draftLanding(prompt: DraftPrompt): {
  presentation: LandingChatPresentation
  providerId: string
} {
  return { presentation: prompt.presentation ?? 'chat', providerId: prompt.provider }
}

export function finishDraftThread(
  draft: DraftSpec,
  chatId: string,
  prompt: DraftPrompt,
  typedSince: () => string,
): void {
  const { permissionLevel, presentation, ...message } = prompt
  const send = () => {
    if (presentation !== 'terminal')
      sendPrompt(draft.workspaceId, chatId, { ...message, steer: false })
  }
  if (permissionLevel) {
    void setChatPermissionLevel(draft.workspaceId, chatId, permissionLevel)
      .catch((err: unknown) =>
        report(err, { action: 'Could not set permission level', fallback: UNREACHED }),
      )
      .finally(send)
  } else {
    send()
  }
  const { panes, paneActions } = windowPaneStore.getState()
  if (panes[draft.paneId]?.chatId !== draft.draftId) return
  saveDraft(chatId, typedSince())
  paneActions.retargetPane(draft.paneId, chatId, null)
}
