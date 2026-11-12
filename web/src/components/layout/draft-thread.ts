import type { PermissionLevel } from '@/features/agent/api/agent-providers-api'
import { setChatPermissionLevel } from '@/features/agent/api/agent-chat-commands'
import { report, UNREACHED } from '@/lib/errors/report'
import { saveDraft } from '@/features/agent/composer/lib/draft-persistence'
import { sendPrompt } from '@/features/agent/stores/chat-outbox'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import type { LandingChatPresentation } from '@/features/settings/lib/chat-presentation'

/** Where a draft thread lives, and where its chat will be created. */
export interface DraftSpec {
  paneId: string
  draftId: string
  projectId: string
  workspaceId: string
  /** The row the chat is placed under; '' is the panel root. */
  parentId: string
}

/** The first prompt, with the selection it was written under. */
export interface DraftPrompt {
  text: string
  provider: string
  model: string
  effort: string
  /** The level the chat starts at; empty keeps the host's default. */
  permissionLevel?: PermissionLevel | ''
  /** Where the chat opens; on Terminal nothing is sent and the box stays the chat's draft. Defaults to Chat. */
  presentation?: LandingChatPresentation
}

/** The draft a pane holds, or null when it holds none (or has been closed). */
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

/** Where a draft's chat starts: on Chat, which sends the prompt (Terminal holds it unsent),
 *  and on the agent the prompt was written for. */
export function draftLanding(prompt: DraftPrompt): {
  presentation: LandingChatPresentation
  providerId: string
} {
  return { presentation: prompt.presentation ?? 'chat', providerId: prompt.provider }
}

/** The draft became a chat: its first prompt goes to that chat's outbox, which
 *  sends it once the chat can take it, and the pane re-points at it in the same step.
 *  What was typed after the send leaves with the draft's box, so it becomes the
 *  chat's unsent draft, which the chat's own box restores. */
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
    // The level lands before the first message can start the agent.
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
  // On the terminal nothing is sent: the whole box is the chat's draft, which Chat restores.
  saveDraft(chatId, typedSince())
  paneActions.retargetPane(draft.paneId, chatId, null)
}
