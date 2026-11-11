import { useEffect, useRef } from 'react'
import { handleCreateHomeThread } from '../home-actions'

// A cold launch lands on a blank draft in the active project's home, not the last chat; the shell
// mounts once per launch, so a resume keeps the screen. No chat exists until the first send.
export function useLaunchDraft(
  projectId: string | null | undefined,
  homeWorkspaceId: string | null,
  navigate: Parameters<typeof handleCreateHomeThread>[2],
): void {
  const opened = useRef(false)
  useEffect(() => {
    if (opened.current || !projectId || !homeWorkspaceId) return
    opened.current = true
    void handleCreateHomeThread(projectId, homeWorkspaceId, navigate)
  }, [projectId, homeWorkspaceId, navigate])
}
