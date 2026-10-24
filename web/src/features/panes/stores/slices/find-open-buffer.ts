import type { OpenEditorTabSpec, PaneContent } from '@/features/panes/types/pane-content'

/**
 * The open buffer `spec` would duplicate — what `openContent` shows instead of
 * creating another. Path-keyed types are scoped to `workspaceId`: worktrees of
 * one repo share relative paths. A New Tab page is unique per pane only.
 */
export function findOpenBuffer(
  buffers: readonly PaneContent[],
  spec: OpenEditorTabSpec,
  workspaceId: string,
  paneTabIds: readonly string[],
): PaneContent | undefined {
  switch (spec.type) {
    case 'editor':
    case 'markdownPreview':
    case 'htmlPreview':
    case 'csvPreview':
      return buffers.find(
        (b) => b.type === spec.type && b.path === spec.path && b.workspaceId === workspaceId,
      )
    case 'branchReview':
      return buffers.find(
        (b) => b.type === 'branchReview' && b.wsId === spec.wsId && b.workspaceId === workspaceId,
      )
    case 'commitDiff':
      return buffers.find(
        (b) =>
          b.type === 'commitDiff' &&
          b.wsId === spec.wsId &&
          b.sha === spec.sha &&
          b.workspaceId === workspaceId,
      )
    case 'terminal':
      if (!spec.sessionId) return undefined
      return buffers.find((b) => b.type === 'terminal' && b.sessionId === spec.sessionId)
    case 'newTabPage': {
      const inPane = new Set(paneTabIds)
      return buffers.find((b) => b.type === 'newTabPage' && inPane.has(b.id))
    }
    default:
      // A browser tab is a fresh webview every time.
      return undefined
  }
}
