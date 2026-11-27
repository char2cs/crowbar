import { ROOT_PANE_ID } from '@/features/panes/constants/pane'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { openBranchReviewForWorkspace } from '@/features/panes/utils/pane-command-actions'
import {
  getActiveWorkspaceId,
  getOrCreateWorkspaceStore,
} from '@/features/workspace/stores/workspace-store-registry'
import type { OpenBuffer } from './open-buffer'

function revealOf(saved: unknown): string | null {
  if (!(saved instanceof Object)) return null
  const reveal: unknown = Reflect.get(saved, 'reveal')
  return typeof reveal === 'string' ? reveal : null
}

function revealReviewFile(wsId: string, path: string): void {
  const id = openBranchReviewForWorkspace(wsId, ROOT_PANE_ID)
  if (id) windowPaneStore.getState().bufferActions.setBranchReviewView(id, 'files')
  getOrCreateWorkspaceStore(wsId).getState().revealBranchReviewFile(path)
}

export function revealInActiveWorkspace(path: string): void {
  const wsId = getActiveWorkspaceId()
  if (wsId) revealReviewFile(wsId, path)
}

export const openReviewBuffer: OpenBuffer = (wsId, saved) => {
  const id = openBranchReviewForWorkspace(wsId, ROOT_PANE_ID)
  const reveal = revealOf(saved)
  if (reveal) revealReviewFile(wsId, reveal)
  return Promise.resolve(id)
}
