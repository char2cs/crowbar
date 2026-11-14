import type { DocumentRef } from '@/lib/api/documents'

// ── Content type discriminant ───────────────────────────────────────
// A pane's chat is PaneGroup.chatId, never a tab type. Saved layouts may still carry a
// 'newTab' placeholder (not 'newTabPage'); the restore path must keep dropping it.

export type EditorTabContentType =
  | 'editor'
  | 'terminal'
  | 'markdownPreview'
  | 'htmlPreview'
  | 'branchReview'
  | 'browser'
  | 'newTabPage'

/** Every content type this build can render. A saved layout outlives the code that wrote it, so
 *  validateLoadedBuffers (persisted-layout.ts) drops any persisted type not in this set. */
export const PANE_CONTENT_TYPES: ReadonlySet<EditorTabContentType> = new Set<EditorTabContentType>([
  'editor',
  'terminal',
  'markdownPreview',
  'htmlPreview',
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
  /** The workspace this content belongs to. `path` is workspace-relative and every workspace's
   *  buffers share one window-wide list, so dedup, disk reconciliation and per-workspace
   *  teardown tell `src/foo.ts` in A from B only by this. */
  workspaceId: string
}

// ── Per-type content definitions ────────────────────────────────────

export interface DocumentUnavailable {
  title: string
  message: string
}

/** A tab on a document; its text lives in the document's Monaco model. */
export interface EditorContent extends EditorTabBase {
  type: 'editor'
  /** Whose file `path` names: a workspace's (relative), or the host's (absolute, `workspaceId` ''). */
  authority: DocumentRef['authority']
  /** Mirrors the document's dirty flag, written only when it flips. */
  isDirty: boolean
  /** The file changed on disk while the buffer had unsaved edits; the edits are kept and a save
   *  overwrites disk. Cleared by the next save or external reload. */
  hasExternalChange?: boolean
  /** Why the document could not be loaded: the pane says so instead of showing an editor, and a
   *  reconcile or reopen reads it again. */
  unavailable?: DocumentUnavailable
  isVirtual: boolean
  language?: string
  languageOverride?: string
}

export interface TerminalContent extends EditorTabBase {
  type: 'terminal'
  sessionId: string
  initialCommand?: string
  workingDirectory?: string
}

export interface MarkdownPreviewContent extends EditorTabBase {
  type: 'markdownPreview'
  content: string
  source: DocumentRef
}

export interface HtmlPreviewContent extends EditorTabBase {
  type: 'htmlPreview'
  content: string
  source: DocumentRef
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
  | MarkdownPreviewContent
  | HtmlPreviewContent
  | BranchReviewContent
  | BrowserContent
  | NewTabPageContent

// ── Type guards ─────────────────────────────────────────────────────

export function isEditorContent(c: PaneContent): c is EditorContent {
  return c.type === 'editor'
}

// ── Helpers ─────────────────────────────────────────────────────────

/** Content types that represent real files on disk and should be persisted to session. */
export function isPersistableContent(c: PaneContent): c is EditorContent {
  return c.type === 'editor' && !c.isVirtual
}

/** An editor buffer holding edits disk does not have. */
export function hasUnsavedEdits(c: PaneContent): boolean {
  return c.type === 'editor' && c.isDirty
}

/** Content types that are virtual (not backed by a real file on disk). */
const VIRTUAL_TYPES: ReadonlySet<EditorTabContentType> = new Set([
  'terminal',
  'branchReview',
  'browser',
  'newTabPage',
])

export function isVirtualContent(c: PaneContent): boolean {
  if (VIRTUAL_TYPES.has(c.type)) return true
  if (c.type === 'editor') return c.isVirtual
  return false
}

// ── Open spec (input to openContent) ────────────────────────────────

// Every spec names its own workspace: an open never falls back to the active one.
export type OpenEditorTabSpec =
  | {
      type: 'editor'
      doc: DocumentRef
      name: string
      isVirtual?: boolean
      isPreview?: boolean
      language?: string
    }
  | {
      type: 'terminal'
      name?: string
      command?: string
      workingDirectory?: string
      sessionId?: string
      path?: string
      workspaceId: string
    }
  | { type: 'markdownPreview' | 'htmlPreview'; source: DocumentRef; name: string; content: string }
  | { type: 'branchReview'; wsId: string; name: string }
  | {
      type: 'browser'
      /** Opens on this page; omitted for the blank address bar. */
      url?: string
      workspaceId: string
    }
  | { type: 'newTabPage'; workspaceId: string }

// ── Buffer history / dialog state (used by workspace store) ─────────

export interface ClosedBuffer {
  authority: DocumentRef['authority']
  path: string
  name: string
  isPinned: boolean
  workspaceId: string
}

/** A dirty tab's close, awaiting the user's answer; the pane it closes from answers it. */
export interface PendingClose {
  bufferId: string
  paneId: string
}
