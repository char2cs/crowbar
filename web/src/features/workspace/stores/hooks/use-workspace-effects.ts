import { useEffect } from 'react'
import { getMockFileTree, getMockFileContent } from '@/lib/mock/files'
import { useFileSystemStore } from '@/features/file-system/controllers/store'
import { useBufferStore } from '@/features/editor/stores/buffer-store'
import { useBufferActions } from './use-buffer-store'
import { useWorkflowActions } from './use-workflow'
import type { AppFile } from '@/features/file-system/types/app'
import type { FlowDefinition } from '@/features/workflow/types/workflow'

export function useWorkspaceEffects(wsId: string, label?: string) {
  const bufferActions = useBufferActions()
  const { setFlowDefinition, setCurrentStep } = useWorkflowActions()
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
        // Open in the legacy global store so CodeEditor can read the content,
        // then mirror the same buffer id into the workspace pane system.
        const bufferId = useBufferStore.getState().actions.openContent({
          type: 'editor', path, name, content,
        })
        bufferActions.registerExternalBuffer(bufferId, path, name, false)
      },
      handleFileSelect: (path: string, isDir?: boolean) => {
        if (isDir) return
        const name = path.split('/').pop() ?? path
        const content = getMockFileContent(path)
        const bufferId = useBufferStore.getState().actions.openContent({
          type: 'editor', path, name, content,
        })
        bufferActions.registerExternalBuffer(bufferId, path, name, true)
      },
    })
  }, [repoPath]) // eslint-disable-line react-hooks/exhaustive-deps

  // Open crowbarChat buffer
  useEffect(() => {
    const name = label ?? 'Workspace'
    bufferActions.openContent({ type: 'crowbarChat', wsId, name })
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
