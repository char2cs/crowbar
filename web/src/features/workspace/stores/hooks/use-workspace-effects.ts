import { useEffect } from 'react'
import { getMockFileTree, getMockFileContent } from '@/lib/mock/files'
import { useFileSystemStore } from '@/features/file-system/controllers/store'
import { useBufferActions } from './use-buffer-store'
import { useWorkflowActions } from './use-workflow'
import { useWorkspaceStore } from '../workspace-context'
import type { BranchReviewContent } from '@/features/panes/types/pane-content'
import type { AppFile } from '@/features/file-system/types/app'
import type { FlowDefinition } from '@/features/workflow/types/workflow'

export function useWorkspaceEffects(wsId: string, label?: string) {
  const bufferActions = useBufferActions()
  const { setFlowDefinition, setCurrentStep } = useWorkflowActions()
  const store = useWorkspaceStore()
  const repoPath = `/repos/${wsId}`

  // Seed file system
  useEffect(() => {
    const files = getMockFileTree(repoPath) as AppFile[]
    useFileSystemStore.setState({
      rootFolderPath: repoPath,
      files,
      handleFileOpen: async (path: string, revealOrIsDir?: boolean) => {
        if (revealOrIsDir === true) return
        const name = path.split('/').pop() ?? path
        const content = getMockFileContent(path)
        bufferActions.openContent({ type: 'editor', path, name, content })
      },
      handleFileSelect: (path: string, isDir?: boolean) => {
        if (isDir) return
        const name = path.split('/').pop() ?? path
        const content = getMockFileContent(path)
        bufferActions.openContent({ type: 'editor', path, name, content, isPreview: true })
      },
    })
  }, [repoPath]) // eslint-disable-line react-hooks/exhaustive-deps

  // Open the branchReview buffer — the sole default pane for a workspace.
  // (Conversations are surfaced inside the review's About tab; individual
  // chats open on demand as their own crowbarChat buffers.)
  //
  // Only open it when one isn't already present. If the layout was restored
  // with the review already in a pane, re-opening would add the same buffer to
  // the (possibly different) active pane — making it appear in two panes.
  useEffect(() => {
    const alreadyOpen = store.getState().buffers.some(
      b => b.type === 'branchReview' && (b as BranchReviewContent).wsId === wsId,
    )
    if (alreadyOpen) return
    const branchName = label ?? wsId
    bufferActions.openContent({ type: 'branchReview', wsId, branchName, name: branchName })
  }, [wsId]) // eslint-disable-line react-hooks/exhaustive-deps

  // Seed mock flow definition
  useEffect(() => {
    const mockFlow: FlowDefinition = {
      flowId: `flow-${wsId}`,
      flowType: 'crowbar-default',
      steps: [
        { id: 'brainstorm', label: 'Brainstorm', contentType: 'chat', isCompleted: false, isActive: true },
        { id: 'spec', label: 'Spec', contentType: 'diff', isCompleted: false, isActive: false },
        { id: 'build', label: 'Build', contentType: 'split', isCompleted: false, isActive: false },
        { id: 'ai_review', label: 'AI Review', contentType: 'diff', isCompleted: false, isActive: false },
        { id: 'human_review', label: 'Review', contentType: 'chat', isCompleted: false, isActive: false },
      ],
    }
    setFlowDefinition(mockFlow)
    setCurrentStep('brainstorm')
  }, [wsId]) // eslint-disable-line react-hooks/exhaustive-deps
}
