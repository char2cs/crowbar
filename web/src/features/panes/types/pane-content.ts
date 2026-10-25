// ── Token entry for syntax highlighting cache ───────────────────────

export interface TokenEntry {
  start: number
  end: number
  token_type: string
  class_name: string
}

// ── Content type discriminant ───────────────────────────────────────
//
// A pane's chat is PaneGroup.chatId, never a member of this union: 'agentChat'
// and 'newTab' left it entirely when the pane gained a dedicated chat slot —
// see pane.ts. 'newTabPage' is a different, real tab; old layouts still carry
// the retired 'newTab' placeholder, which must keep being dropped.

export type EditorTabContentType =
  | 'editor'
  | 'terminal'
  | 'commitDiff'
  | 'markdownPreview'
  | 'htmlPreview'
  | 'csvPreview'
  | 'branchReview'
  | 'browser'
  | 'newTabPage'

/** Every content type this build can render.
 *
 *  A saved layout outlives the code that wrote it, so the restore path checks
 *  a persisted buffer's type against this set and drops what it no longer
 *  knows — see validateLoadedBuffers in persisted-layout.ts.
 */
export const PANE_CONTENT_TYPES: ReadonlySet<EditorTabContentType> = new Set<EditorTabContentType>([
  'editor',
  'terminal',
  'commitDiff',
  'markdownPreview',
  'htmlPreview',
  'csvPreview',
  'branchReview',
  'browser',
  'newTabPage',
])

// ── Base fields shared by every editor-tab content type ─────────────

export interface EditorTabBase {
  id: string
  type: EditorTabContentType
  path?: string
  name: string
  isPinned?: boolean
  isPreview?: boolean
  isUncloseable?: boolean
  /**
   * The workspace this tab's content belongs to. Load-bearing since Task 26
   * hoisted `buffers`/`panes` out of the per-workspace store registry into
   * one window-level store: `path` is workspace-RELATIVE (a sibling worktree
   * can hold a different file at the same path), so once every workspace's
   * buffers share one flat list, this field is the only thing that keeps
   * "src/foo.ts in workspace A" from being confused with "src/foo.ts in
   * workspace B" — in dedup (`openContent`), disk reconciliation
   * (`external-buffer-sync.ts`, `hydrate.ts`), and per-workspace teardown
   * (`destroyWorkspaceStore`'s terminal-detach/blame-clear/history-cleanup).
   */
  workspaceId: string
}

// ── Per-type content definitions ────────────────────────────────────

export interface EditorContent extends EditorTabBase {
  type: 'editor'
  content: string
  savedContent: string
  isDirty: boolean
  /**
   * The file changed on disk (externally) while the buffer had unsaved edits.
   * The user's edits are kept; saving will overwrite the on-disk version.
   * Cleared on the next successful save or external reload.
   */
  hasExternalChange?: boolean
  /**
   * The file backing this buffer no longer exists on disk (the content load
   * 404'd at session restore). Terminal until the file reappears — the pane
   * renders a "file not found" placeholder and no further loads are retried.
   */
  fileMissing?: boolean
  isVirtual: boolean
  language?: string
  languageOverride?: string
  tokens: TokenEntry[]
}

export interface TerminalContent extends EditorTabBase {
  type: 'terminal'
  sessionId: string
  initialCommand?: string
  workingDirectory?: string
}

/** One commit's diff, rendered on the same windowed surface as the branch
 *  review. It carries the SHA, not the diff: the payload is fetched per file as
 *  the viewport reaches it, so a tab costs the same whether the commit changed
 *  three lines or a million. */
export interface CommitDiffContent extends EditorTabBase {
  type: 'commitDiff'
  wsId: string
  sha: string
}

export interface MarkdownPreviewContent extends EditorTabBase {
  type: 'markdownPreview'
  content: string
  sourceFilePath: string
}

export interface HtmlPreviewContent extends EditorTabBase {
  type: 'htmlPreview'
  content: string
  sourceFilePath: string
}

export interface CsvPreviewContent extends EditorTabBase {
  type: 'csvPreview'
  content: string
  sourceFilePath: string
}

export type BranchReviewView = 'conversation' | 'files'

export interface BranchReviewContent extends EditorTabBase {
  type: 'branchReview'
  wsId: string
  /** Which tab of the review is showing; absent until the user picks one. */
  view?: BranchReviewView
}

/** A native webview tab. The buffer id is the Rust webview id; `url` is ''
 *  until the first navigation, and the webview is not created before then. */
export interface BrowserContent extends EditorTabBase {
  type: 'browser'
  url: string
  title?: string
  /** A page load is in flight, as the webview reports it. */
  loading?: boolean
}

/** A browser-style New Tab: the empty editor area's actions as a tab of its
 *  own, which the content chosen on it replaces. At most one per pane. */
export interface NewTabPageContent extends EditorTabBase {
  type: 'newTabPage'
}

// ── Discriminated union ─────────────────────────────────────────────

export type PaneContent =
  | EditorContent
  | TerminalContent
  | CommitDiffContent
  | MarkdownPreviewContent
  | HtmlPreviewContent
  | CsvPreviewContent
  | BranchReviewContent
  | BrowserContent
  | NewTabPageContent

// ── Type guards ─────────────────────────────────────────────────────

export function isEditorContent(c: PaneContent): c is EditorContent {
  return c.type === 'editor'
}

export function isBranchReviewContent(c: PaneContent): c is BranchReviewContent {
  return c.type === 'branchReview'
}

export function isCommitDiffContent(c: PaneContent): c is CommitDiffContent {
  return c.type === 'commitDiff'
}

// ── Helpers ─────────────────────────────────────────────────────────

/** Content types that represent real files on disk and should be persisted to session. */
export function isPersistableContent(c: PaneContent): c is EditorContent {
  return c.type === 'editor' && !c.isVirtual
}

/** An editor buffer holding edits disk does not have — the only copy of them. */
export function hasUnsavedEdits(c: PaneContent): boolean {
  return c.type === 'editor' && (c.isDirty || c.content !== c.savedContent)
}

/** Content types that are virtual (not backed by a real file on disk). */
const VIRTUAL_TYPES: ReadonlySet<EditorTabContentType> = new Set([
  'terminal',
  'branchReview',
  'commitDiff',
  'browser',
  'newTabPage',
])

export function isVirtualContent(c: PaneContent): boolean {
  if (VIRTUAL_TYPES.has(c.type)) return true
  if (c.type === 'editor') return c.isVirtual
  return false
}

/** Whether this content has text content (for search, etc.) */
export function hasTextContent(
  c: PaneContent,
): c is EditorContent | MarkdownPreviewContent | HtmlPreviewContent | CsvPreviewContent {
  return (
    c.type === 'editor' ||
    c.type === 'markdownPreview' ||
    c.type === 'htmlPreview' ||
    c.type === 'csvPreview'
  )
}

/** Whether the content type should trigger LSP operations. */
export function shouldStartLsp(c: PaneContent): c is EditorContent {
  return c.type === 'editor' && !c.isVirtual
}

// ── Open spec (input to openContent) ────────────────────────────────

export type OpenEditorTabSpec =
  | {
      type: 'editor'
      path: string
      name: string
      content: string
      isVirtual?: boolean
      isPreview?: boolean
      language?: string
      /** Defaults to the active workspace when omitted — see EditorTabBase.workspaceId. */
      workspaceId?: string
    }
  | {
      type: 'terminal'
      name?: string
      command?: string
      workingDirectory?: string
      sessionId?: string
      path?: string
      workspaceId?: string
    }
  | {
      type: 'commitDiff'
      wsId: string
      sha: string
      name: string
      workspaceId?: string
    }
  | {
      type: 'markdownPreview'
      path: string
      name: string
      content: string
      sourceFilePath: string
      workspaceId?: string
    }
  | {
      type: 'htmlPreview'
      path: string
      name: string
      content: string
      sourceFilePath: string
      workspaceId?: string
    }
  | {
      type: 'csvPreview'
      path: string
      name: string
      content: string
      sourceFilePath: string
      workspaceId?: string
    }
  | {
      type: 'branchReview'
      wsId: string
      name: string
      workspaceId?: string
    }
  | {
      type: 'browser'
      /** Opens on this page; omitted for the blank address bar. */
      url?: string
      workspaceId?: string
    }
  | {
      type: 'newTabPage'
      workspaceId?: string
    }

// ── Buffer history / dialog state (used by workspace store) ─────────

export interface ClosedBuffer {
  path: string
  name: string
  isPinned: boolean
  workspaceId: string
}

export interface PendingClose {
  bufferId: string
  type: 'single' | 'others' | 'all' | 'to-left' | 'to-right'
  anchorBufferId?: string
  keepBufferId?: string
}
