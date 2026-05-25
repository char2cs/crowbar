import type { StateCreator } from 'zustand'
import type { WorkspaceState } from '../workspace-store.types'
import type { PaneContent, EditorContent, CrowbarChatContent, DiffContent, TerminalContent, WebViewerContent } from '@/features/panes/types/pane-content'
import { nanoid } from 'nanoid'

// ── Open spec union ──────────────────────────────────────────────────

export type OurOpenContentSpec =
  | {
      type: 'editor'
      path: string
      name: string
      content: string
      isPreview?: boolean
      language?: string
      isVirtual?: boolean
    }
  | {
      type: 'crowbarChat'
      wsId: string
      name: string
    }
  | {
      type: 'diff'
      path: string
      name: string
      content: string
    }
  | {
      type: 'terminal'
      name?: string
      command?: string
      workingDirectory?: string
      remoteConnectionId?: string
      sessionId?: string
    }
  | {
      type: 'webViewer'
      url: string
      name?: string
    }

// ── Actions ──────────────────────────────────────────────────────────

export interface BufferActions {
  openContent(spec: OurOpenContentSpec): string
  /**
   * Register a buffer that already exists in the legacy global store (e.g. so
   * CodeEditor can still read it) into the workspace pane system using the
   * same buffer id.  This avoids an id-mismatch between the two stores.
   */
  registerExternalBuffer(id: string, path: string, name: string, isPreview?: boolean): string
  closeBuffer(id: string): void
  setPinned(id: string, pinned: boolean): void
  setPreview(id: string, preview: boolean): void
  promotePreview(id: string): void
  getBufferById(id: string): PaneContent | undefined
}

// ── Slice ────────────────────────────────────────────────────────────

export interface BufferSlice {
  buffers: PaneContent[]
  bufferActions: BufferActions
}

export const createBufferSlice: StateCreator<
  WorkspaceState,
  [['zustand/immer', never]],
  [],
  BufferSlice
> = (set, get) => ({
  buffers: [],

  bufferActions: {
    openContent(spec) {
      const existing = (() => {
        if (spec.type === 'editor') {
          return get().buffers.find(b => b.type === 'editor' && b.path === spec.path)
        }
        if (spec.type === 'crowbarChat') {
          return get().buffers.find(
            b => b.type === 'crowbarChat' && (b as CrowbarChatContent).wsId === spec.wsId,
          )
        }
        if (spec.type === 'diff') {
          return get().buffers.find(b => b.type === 'diff' && b.path === spec.path)
        }
        // Terminal: only deduplicate by sessionId when one is explicitly provided
        if (spec.type === 'terminal' && spec.sessionId) {
          return get().buffers.find(
            b => b.type === 'terminal' && (b as TerminalContent).sessionId === spec.sessionId,
          )
        }
        // WebViewer: deduplicate by URL
        if (spec.type === 'webViewer') {
          return get().buffers.find(
            b => b.type === 'webViewer' && (b as WebViewerContent).url === spec.url,
          )
        }
        return undefined
      })()

      if (existing) {
        // Bring existing buffer into the active pane and activate it
        get().paneActions.addBufferToPane(get().activePaneId, existing.id, true)
        return existing.id
      }

      const id = nanoid()

      if (spec.type === 'editor') {
        const isPreview = spec.isPreview ?? false
        const buf: EditorContent = {
          id,
          type: 'editor',
          path: spec.path,
          name: spec.name,
          content: spec.content,
          savedContent: spec.content,
          isDirty: false,
          isVirtual: spec.isVirtual ?? false,
          language: spec.language,
          tokens: [],
          isPinned: false,
          isPreview,
          isActive: false,
        }
        set(state => { state.buffers.push(buf as any) })
        get().paneActions.addBufferToPane(get().activePaneId, id, true)
        if (isPreview) {
          get().paneActions.setPanePreviewBuffer(get().activePaneId, id)
        }
      } else if (spec.type === 'crowbarChat') {
        const buf: CrowbarChatContent = {
          id,
          type: 'crowbarChat',
          wsId: spec.wsId,
          path: '',
          name: spec.name,
          isPinned: false,
          isPreview: false,
          isActive: false,
        }
        set(state => { state.buffers.push(buf as any) })
        get().paneActions.addBufferToPane(get().activePaneId, id, true)
      } else if (spec.type === 'diff') {
        const buf: DiffContent = {
          id,
          type: 'diff',
          path: spec.path,
          name: spec.name,
          content: spec.content,
          savedContent: spec.content,
          isPinned: false,
          isPreview: false,
          isActive: false,
        }
        set(state => { state.buffers.push(buf as any) })
        get().paneActions.addBufferToPane(get().activePaneId, id, true)
      } else if (spec.type === 'terminal') {
        const terminalCount = get().buffers.filter(b => b.type === 'terminal').length
        const sessionId = spec.sessionId ?? `terminal-tab-${Date.now()}`
        const buf: TerminalContent = {
          id,
          type: 'terminal',
          sessionId,
          path: `terminal://${sessionId}`,
          name: spec.name ?? `Terminal ${terminalCount + 1}`,
          initialCommand: spec.command,
          workingDirectory: spec.workingDirectory,
          remoteConnectionId: spec.remoteConnectionId,
          isPinned: false,
          isPreview: false,
          isActive: false,
        }
        set(state => { state.buffers.push(buf as any) })
        get().paneActions.addBufferToPane(get().activePaneId, id, true)
      } else if (spec.type === 'webViewer') {
        let displayName = spec.name ?? 'Web Viewer'
        if (!spec.name && spec.url && spec.url !== 'about:blank') {
          try {
            const urlObj = new URL(spec.url)
            if (urlObj.hostname) displayName = urlObj.hostname
          } catch {
            // Invalid URL, keep default
          }
        }
        const buf: WebViewerContent = {
          id,
          type: 'webViewer',
          url: spec.url,
          path: `web-viewer://${spec.url}`,
          name: displayName,
          isPinned: false,
          isPreview: false,
          isActive: false,
        }
        set(state => { state.buffers.push(buf as any) })
        get().paneActions.addBufferToPane(get().activePaneId, id, true)
      }

      return id
    },

    closeBuffer(id) {
      set(state => {
        state.buffers = state.buffers.filter(b => b.id !== id)
      })
    },

    setPinned(id, pinned) {
      set(state => {
        const buf = state.buffers.find(b => b.id === id)
        if (buf) buf.isPinned = pinned
      })
    },

    setPreview(id, preview) {
      set(state => {
        const buf = state.buffers.find(b => b.id === id)
        if (buf) buf.isPreview = preview
      })
    },

    promotePreview(id) {
      set(state => {
        const buf = state.buffers.find(b => b.id === id)
        if (buf) buf.isPreview = false
      })
    },

    getBufferById(id) {
      return get().buffers.find(b => b.id === id)
    },

    registerExternalBuffer(id, path, name, isPreview = false) {
      // If already tracked, just bring it into focus in the active pane.
      if (get().buffers.some(b => b.id === id)) {
        get().paneActions.addBufferToPane(get().activePaneId, id, true)
        if (isPreview) get().paneActions.setPanePreviewBuffer(get().activePaneId, id)
        return id
      }

      // Create a minimal shell — CodeEditor reads the real content from the
      // old global store using this same id.
      const buf: EditorContent = {
        id,
        type: 'editor',
        path,
        name,
        content: '',
        savedContent: '',
        isDirty: false,
        isVirtual: false,
        tokens: [],
        isPinned: false,
        isPreview,
        isActive: false,
      }
      set(state => { state.buffers.push(buf as any) })
      get().paneActions.addBufferToPane(get().activePaneId, id, true)
      if (isPreview) get().paneActions.setPanePreviewBuffer(get().activePaneId, id)
      return id
    },
  },
})
