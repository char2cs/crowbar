import { useEffect } from 'react'
import { useBufferActions } from './use-buffer-store'

export function useWorkspaceEffects(wsId: string, label?: string) {
  const bufferActions = useBufferActions()

  // Open crowbarChat buffer
  useEffect(() => {
    const name = label ?? 'Workspace'
    bufferActions.openContent({ type: 'crowbarChat', wsId, name })
  }, [wsId]) // eslint-disable-line react-hooks/exhaustive-deps

  // Open branchReview buffer
  useEffect(() => {
    const branchName = label ?? wsId
    bufferActions.openContent({ type: 'branchReview', wsId, branchName, name: branchName })
  }, [wsId]) // eslint-disable-line react-hooks/exhaustive-deps
}
