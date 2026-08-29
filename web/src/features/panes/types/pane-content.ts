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
// see pane.ts.

export type EditorTabContentType =
  | 'editor'
  | 'terminal'
  | 'commitDiff'
  | 'markdownPreview'
  | 'htmlPreview'
  | 'csvPreview'
  | 'externalEditor'
  | 'branchReview'

/** Every content type this build can render.
 *
 *  A saved layout outlives the code that wrote it, so the restore path checks
 *  a persisted buffer's type against this set and drops what it no longer
 *  knows — see stripNewTabs in persisted-layout.ts. */
export const PANE_CONTENT_TYPES: ReadonlySet<EditorTabContentType> = new Set<EditorTabContentType>(
  [
    'editor',
    'terminal',
    'commitDiff',
    'markdownPreview',
    'htmlPreview',
    'csvPreview',
    'externalEditor',
    'branchReview',
  ],
)

// ── Base fields shared by every editor-tab content type ─────────────

interface EditorTabBase {
  id: string
  type: EditorTabContentType
  path?: string
  name: string
  isPinned?: boolean
  isPreview?: boolean
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
  remoteConnectionId?: string
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

export interface ExternalEditorContent extends EditorTabBase {
  type: 'externalEditor'
  terminalConnectionId: string
}

export interface BranchReviewContent extends EditorTabBase {
  type: 'branchReview'
  wsId: string
}

// ── Discriminated union ─────────────────────────────────────────────

export type PaneContent =
  | EditorContent
  | TerminalContent
  | CommitDiffContent
  | MarkdownPreviewContent
  | HtmlPreviewContent
  | CsvPreviewContent
  | ExternalEditorContent
  | BranchReviewContent

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

/** Content types that are virtual (not backed by a real file on disk). */
const VIRTUAL_TYPES: ReadonlySet<EditorTabContentType> = new Set([
  'terminal',
  'branchReview',
  'commitDiff',
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
    }
  | {
      type: 'terminal'
      name?: string
      command?: string
      workingDirectory?: string
      remoteConnectionId?: string
      sessionId?: string
      path?: string
    }
  | {
      type: 'commitDiff'
      wsId: string
      sha: string
      name: string
    }
  | {
      type: 'markdownPreview'
      path: string
      name: string
      content: string
      sourceFilePath: string
    }
  | {
      type: 'htmlPreview'
      path: string
      name: string
      content: string
      sourceFilePath: string
    }
  | {
      type: 'csvPreview'
      path: string
      name: string
      content: string
      sourceFilePath: string
    }
  | {
      type: 'externalEditor'
      path: string
      name: string
      terminalConnectionId: string
    }
  | {
      type: 'branchReview'
      wsId: string
      name: string
    }

// ── Buffer history / dialog state (used by workspace store) ─────────

export interface ClosedBuffer {
  path: string
  name: string
  isPinned: boolean
}

export interface PendingClose {
  bufferId: string
  type: 'single' | 'others' | 'all' | 'to-left' | 'to-right'
  anchorBufferId?: string
  keepBufferId?: string
}
