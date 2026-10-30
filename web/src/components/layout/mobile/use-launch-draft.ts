import { useEffect, useRef } from 'react'
import { openHomeDraft } from '../home-actions'

/**
 * A cold launch lands on a blank draft in the active project's home — the target the list's
 * "New thread" uses — rather than the chat last open. The shell mounts once per launch, so a
 * resume keeps what the user is looking at; no chat exists until the draft's first send.
 */
export function useLaunchDraft(
  projectId: string | null | undefined,
  homeWorkspaceId: string | null,
  navigate: Parameters<typeof openHomeDraft>[2],
): void {
  const opened = useRef(false)
  useEffect(() => {
    if (opened.current || !projectId || !homeWorkspaceId) return
    opened.current = true
    void openHomeDraft(projectId, homeWorkspaceId, navigate)
  }, [projectId, homeWorkspaceId, navigate])
}
