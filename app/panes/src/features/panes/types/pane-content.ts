import type { DocumentRef } from '@/lib/api/documents'

export type EditorTabContentType =
  | 'editor'
  | 'terminal'
  | 'markdownPreview'
  | 'htmlPreview'
  | 'branchReview'
  | 'browser'
  | 'newTabPage'

export const PANE_CONTENT_TYPES: ReadonlySet<EditorTabContentType> = new Set<EditorTabContentType>([
  'editor',
  'terminal',
  'markdownPreview',
  'htmlPreview',
  'branchReview',
  'browser',
  'newTabPage',
])

export interface EditorTabBase {
  id: string
  type: EditorTabContentType
  path?: string
  name: string
  isPinned?: boolean
  isPreview?: boolean
  isUncloseable?: boolean
  workspaceId: string
}

export interface DocumentUnavailable {
  title: string
  message: string
}

export interface EditorContent extends EditorTabBase {
  type: 'editor'
  authority: DocumentRef['authority']
  isDirty: boolean
  hasExternalChange?: boolean
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
  view?: BranchReviewView
}

export interface BrowserContent extends EditorTabBase {
  type: 'browser'
  url: string
  title?: string
  loading?: boolean
}

export interface NewTabPageContent extends EditorTabBase {
  type: 'newTabPage'
}

export type PaneContent =
  | EditorContent
  | TerminalContent
  | MarkdownPreviewContent
  | HtmlPreviewContent
  | BranchReviewContent
  | BrowserContent
  | NewTabPageContent

export function isEditorContent(c: PaneContent): c is EditorContent {
  return c.type === 'editor'
}

export function isPersistableContent(c: PaneContent): c is EditorContent {
  return c.type === 'editor' && !c.isVirtual
}

export function hasUnsavedEdits(c: PaneContent): boolean {
  return c.type === 'editor' && c.isDirty
}

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
      url?: string
      workspaceId: string
    }
  | { type: 'newTabPage'; workspaceId: string }

export interface ClosedBuffer {
  authority: DocumentRef['authority']
  path: string
  name: string
  isPinned: boolean
  workspaceId: string
}

export interface PendingClose {
  bufferId: string
  paneId: string
}
