import type { MultiFileDiff } from '@/features/git/types/git-diff-types'
import type { GitDiff } from '@/features/git/types/git-types'

// ── Token entry for syntax highlighting cache ───────────────────────

export interface TokenEntry {
  start: number
  end: number
  token_type: string
  class_name: string
}

// ── Content type discriminant ───────────────────────────────────────

export type PaneContentType =
  | 'editor'
  | 'terminal'
  | 'newTab'
  | 'diff'
  | 'markdownPreview'
  | 'htmlPreview'
  | 'csvPreview'
  | 'externalEditor'
  | 'branchReview'
  | 'agentChat'

// ── Base fields shared by every content type ────────────────────────

interface PaneContentBase {
  id: string
  type: PaneContentType
  path: string
  name: string
  isPinned: boolean
  isPreview: boolean
  isActive: boolean
  isUncloseable?: boolean
}

// ── Per-type content definitions ────────────────────────────────────

export interface EditorContent extends PaneContentBase {
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

export interface TerminalContent extends PaneContentBase {
  type: 'terminal'
  sessionId: string
  initialCommand?: string
  workingDirectory?: string
  remoteConnectionId?: string
}

export interface NewTabContent extends PaneContentBase {
  type: 'newTab'
}

export interface DiffContent extends PaneContentBase {
  type: 'diff'
  content: string
  savedContent: string
  diffData?: GitDiff | MultiFileDiff
}

export interface MarkdownPreviewContent extends PaneContentBase {
  type: 'markdownPreview'
  content: string
  sourceFilePath: string
}

export interface HtmlPreviewContent extends PaneContentBase {
  type: 'htmlPreview'
  content: string
  sourceFilePath: string
}

export interface CsvPreviewContent extends PaneContentBase {
  type: 'csvPreview'
  content: string
  sourceFilePath: string
}

export interface ExternalEditorContent extends PaneContentBase {
  type: 'externalEditor'
  terminalConnectionId: string
}

export interface BranchReviewContent extends PaneContentBase {
  type: 'branchReview'
  wsId: string
}

export interface AgentChatContent extends PaneContentBase {
  type: 'agentChat'
  /** Stable chat id — the pane tab is keyed by it (survives provider switches). */
  chatId: string
  wsId: string
}

// ── Discriminated union ─────────────────────────────────────────────

export type PaneContent =
  | EditorContent
  | TerminalContent
  | NewTabContent
  | DiffContent
  | MarkdownPreviewContent
  | HtmlPreviewContent
  | CsvPreviewContent
  | ExternalEditorContent
  | BranchReviewContent
  | AgentChatContent

// ── Type guards ─────────────────────────────────────────────────────

export function isEditorContent(c: PaneContent): c is EditorContent {
  return c.type === 'editor'
}

export function isTerminalContent(c: PaneContent): c is TerminalContent {
  return c.type === 'terminal'
}

export function isNewTabContent(c: PaneContent): c is NewTabContent {
  return c.type === 'newTab'
}

export function isDiffContent(c: PaneContent): c is DiffContent {
  return c.type === 'diff'
}

export function isExternalEditorContent(c: PaneContent): c is ExternalEditorContent {
  return c.type === 'externalEditor'
}

export function isBranchReviewContent(c: PaneContent): c is BranchReviewContent {
  return c.type === 'branchReview'
}

export function isAgentChatContent(c: PaneContent): c is AgentChatContent {
  return c.type === 'agentChat'
}

// ── Helpers ─────────────────────────────────────────────────────────

/** Content types that represent real files on disk and should be persisted to session. */
export function isPersistableContent(c: PaneContent): c is EditorContent {
  return c.type === 'editor' && !c.isVirtual
}

/** Content types that are virtual (not backed by a real file on disk). */
const VIRTUAL_TYPES: ReadonlySet<PaneContentType> = new Set([
  'terminal',
  'newTab',
  'branchReview',
  'agentChat',
])

export function isVirtualContent(c: PaneContent): boolean {
  if (VIRTUAL_TYPES.has(c.type)) return true
  if (c.type === 'editor') return c.isVirtual
  return false
}

/** Whether this content type has editable text content with dirty tracking. */
export function isEditableContent(c: PaneContent): c is EditorContent | DiffContent {
  return c.type === 'editor' || c.type === 'diff'
}

/** Whether this content has text content (for search, etc.) */
export function hasTextContent(
  c: PaneContent,
): c is
  | EditorContent
  | DiffContent
  | MarkdownPreviewContent
  | HtmlPreviewContent
  | CsvPreviewContent {
  return (
    c.type === 'editor' ||
    c.type === 'diff' ||
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

export type OpenContentSpec =
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
  | { type: 'newTab' }
  | {
      type: 'diff'
      path: string
      name: string
      content: string
      diffData?: GitDiff | MultiFileDiff
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
  | {
      type: 'agentChat'
      chatId: string
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
