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
  | 'commitDiff'
  | 'markdownPreview'
  | 'htmlPreview'
  | 'csvPreview'
  | 'externalEditor'
  | 'branchReview'
  | 'agentChat'

/** Every content type this build can render.
 *
 *  A saved layout outlives the code that wrote it, so the restore path checks
 *  a persisted buffer's type against this set and drops what it no longer
 *  knows — see stripNewTabs in persisted-layout.ts. */
export const PANE_CONTENT_TYPES: ReadonlySet<PaneContentType> = new Set<PaneContentType>([
  'editor',
  'terminal',
  'newTab',
  'commitDiff',
  'markdownPreview',
  'htmlPreview',
  'csvPreview',
  'externalEditor',
  'branchReview',
  'agentChat',
])

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

/** One commit's diff, rendered on the same windowed surface as the branch
 *  review. It carries the SHA, not the diff: the payload is fetched per file as
 *  the viewport reaches it, so a tab costs the same whether the commit changed
 *  three lines or a million. */
export interface CommitDiffContent extends PaneContentBase {
  type: 'commitDiff'
  wsId: string
  sha: string
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

// An agent-chat tab is a VIEWPORT on a moving target, so BOTH of its identity
// fields are mutable — see repointAgentChatBuffer (buffer-slice).
//
//  - the RUNNER (the vendor-CLI process) moves to another chat when the user types
//    /clear or /resume inside the CLI: chatId is re-pointed and the tab relabels,
//    while the terminal — keyed by the runner's unchanged PTY — never remounts.
//    That is the whole feature: the conversation changes without the terminal
//    changing.
//  - a DORMANT chat that is Resumed gets a brand-new runner: runnerId is
//    re-pointed and the tab re-attaches to that runner's PTY.
//
// Neither id is stable for the life of the tab, and pinning either one is exactly
// the bug this shape replaces (a pane pinned to a chatId showed "this agent has
// exited" while its CLI was alive and well in another chat).
export interface AgentChatContent extends PaneContentBase {
  type: 'agentChat'
  /** The chat this tab is SHOWING right now. Follows the runner when it moves. */
  chatId: string
  /** The runner this tab is FOLLOWING, or '' when the chat is dormant (no CLI). */
  runnerId: string
  wsId: string
}

// ── Discriminated union ─────────────────────────────────────────────

export type PaneContent =
  | EditorContent
  | TerminalContent
  | NewTabContent
  | CommitDiffContent
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

export function isBranchReviewContent(c: PaneContent): c is BranchReviewContent {
  return c.type === 'branchReview'
}

export function isCommitDiffContent(c: PaneContent): c is CommitDiffContent {
  return c.type === 'commitDiff'
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
  'commitDiff',
  'agentChat',
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
  | {
      type: 'agentChat'
      chatId: string
      wsId: string
      name: string
      /** The runner on the chat at open time, when the caller happens to know it.
       *  Optional because it is never load-bearing: the pane adopts whatever runner
       *  its chat actually has, so a tab opened with '' (or with an id that has
       *  since died) converges on the truth by itself. */
      runnerId?: string
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
