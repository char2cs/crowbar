import { ROOT_PANE_ID } from '@/features/panes/constants/pane'
import { openBranchReviewForWorkspace } from '@/features/panes/utils/pane-command-actions'
import type { OpenBuffer } from './open-buffer'

/** Opens the branch review of the workspace; a second call finds the open tab. */
export const openReviewBuffer: OpenBuffer = (wsId) =>
  Promise.resolve(openBranchReviewForWorkspace(wsId, ROOT_PANE_ID))
