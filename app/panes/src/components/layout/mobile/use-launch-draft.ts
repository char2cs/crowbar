import { useEffect, useRef } from 'react'
import { handleCreateHomeThread } from '../home-actions'

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
