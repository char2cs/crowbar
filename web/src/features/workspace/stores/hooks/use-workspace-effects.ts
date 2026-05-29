import { useEffect } from 'react'
import { getMockFileTree, getMockFileContent } from '@/lib/mock/files'
import { useFileSystemStore } from '@/features/file-system/controllers/store'
import { useUIState } from '@/features/window/stores/ui-state-store'
import { getActiveWorkspaceStoreRef } from '@/features/workspace/stores/workspace-store-ref'
import { BOTTOM_PANE_ID } from '@/features/panes/constants/pane'
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

  // Open crowbarChat buffer
  useEffect(() => {
    const name = label ?? 'Workspace'
    bufferActions.openContent({ type: 'crowbarChat', wsId, name })
  }, [wsId]) // eslint-disable-line react-hooks/exhaustive-deps

  // Listen for the terminal-ensure-session event dispatched by the command palette.
  // When triggered, ensure the bottom pane has a terminal buffer and request focus.
  useEffect(() => {
    const handler = () => {
      const store = getActiveWorkspaceStoreRef()
      if (!store) return

      const state = store.getState()

      // Check if there's already a terminal buffer in the bottom pane
      const bottomPane = state.paneActions.getPane(BOTTOM_PANE_ID)
      const hasTerminalInBottomPane = bottomPane?.bufferIds.some(id => {
        const buf = state.buffers.find(b => b.id === id)
        return buf?.type === 'terminal'
      })

      if (!hasTerminalInBottomPane) {
        // Open a new terminal buffer and place it in the bottom pane
        const bufferId = state.bufferActions.openContent({ type: 'terminal' })
        if (bufferId) {
          state.paneActions.addBufferToPane(BOTTOM_PANE_ID, bufferId, true)
        }
      } else if (bottomPane?.activeBufferId) {
        // Activate the existing terminal buffer
        state.paneActions.addBufferToPane(BOTTOM_PANE_ID, bottomPane.activeBufferId, true)
      }

      // Make bottom pane visible and request terminal focus after a brief delay
      // to let the terminal mount and register its focus function.
      useUIState.getState().setIsBottomPaneVisible(true)
      const timer = setTimeout(() => {
        useUIState.getState().requestTerminalFocus()
      }, 50)

      return () => clearTimeout(timer)
    }

    window.addEventListener('terminal-ensure-session', handler)
    return () => window.removeEventListener('terminal-ensure-session', handler)
  }, []) // eslint-disable-line react-hooks/exhaustive-deps

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
