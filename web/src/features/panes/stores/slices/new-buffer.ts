import type {
  BranchReviewContent,
  BrowserContent,
  CommitDiffContent,
  EditorContent,
  HtmlPreviewContent,
  MarkdownPreviewContent,
  NewTabPageContent,
  OpenEditorTabSpec,
  PaneContent,
  TerminalContent,
} from '@/features/panes/types/pane-content'
import { browserTabName } from '@/features/browser/lib/browser-tab-name'

const base = (id: string, workspaceId: string) => ({
  id,
  isPinned: false,
  isPreview: false,
  workspaceId,
})

/** The buffer `openContent` creates for `spec`; `terminalCount` names an unnamed terminal. */
export function newBuffer(
  spec: OpenEditorTabSpec,
  id: string,
  workspaceId: string,
  terminalCount: number,
): PaneContent {
  switch (spec.type) {
    case 'editor':
      return {
        ...base(id, workspaceId),
        type: 'editor',
        authority: spec.doc.authority,
        path: spec.doc.path,
        name: spec.name,
        isDirty: false,
        isVirtual: spec.isVirtual ?? false,
        language: spec.language,
        isPreview: spec.isPreview ?? false,
      } satisfies EditorContent
    case 'branchReview':
      return {
        ...base(id, workspaceId),
        type: 'branchReview',
        wsId: spec.wsId,
        name: spec.name,
        path: `branch-review://${spec.wsId}`,
      } satisfies BranchReviewContent
    case 'commitDiff':
      return {
        ...base(id, workspaceId),
        type: 'commitDiff',
        wsId: spec.wsId,
        sha: spec.sha,
        name: spec.name,
        // One tab per commit per workspace: reopening the same commit
        // focuses the tab that is already there rather than stacking another.
        path: `commit-diff://${spec.wsId}/${spec.sha}`,
      } satisfies CommitDiffContent
    case 'terminal': {
      const sessionId = spec.sessionId ?? `terminal-tab-${Date.now()}`
      return {
        ...base(id, workspaceId),
        type: 'terminal',
        sessionId,
        path: spec.path ?? `terminal://${sessionId}`,
        name: spec.name ?? `Terminal ${terminalCount + 1}`,
        initialCommand: spec.command,
        workingDirectory: spec.workingDirectory,
      } satisfies TerminalContent
    }
    case 'browser':
      return {
        ...base(id, workspaceId),
        type: 'browser',
        url: spec.url ?? '',
        name: browserTabName(undefined, spec.url ?? ''),
        path: `browser://${id}`,
      } satisfies BrowserContent
    case 'newTabPage':
      return {
        ...base(id, workspaceId),
        type: 'newTabPage',
        name: 'New Tab',
        path: `new-tab://${id}`,
      } satisfies NewTabPageContent
    default:
      return {
        ...base(id, workspaceId),
        type: spec.type,
        path: `${spec.source.path}:preview`,
        name: spec.name,
        content: spec.content,
        source: spec.source,
      } satisfies MarkdownPreviewContent | HtmlPreviewContent
  }
}
