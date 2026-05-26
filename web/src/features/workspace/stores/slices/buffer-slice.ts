import type { StateCreator } from 'zustand'
import type { WorkspaceState } from '../workspace-store.types'
import type { PaneContent, EditorContent, CrowbarChatContent, DiffContent, TerminalContent, WebViewerContent, AgentContent } from '@/features/panes/types/pane-content'
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
  | {
      type: 'agent'
      sessionId?: string
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
        // Agent: deduplicate by sessionId when provided
        if (spec.type === 'agent' && spec.sessionId) {
          return get().buffers.find(
            b => b.type === 'agent' && (b as AgentContent).sessionId === spec.sessionId,
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
        set(state => { state.buffers.push(buf as PaneContent) })
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
        set(state => { state.buffers.push(buf as PaneContent) })
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
        set(state => { state.buffers.push(buf as PaneContent) })
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
        set(state => { state.buffers.push(buf as PaneContent) })
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
        set(state => { state.buffers.push(buf as PaneContent) })
        get().paneActions.addBufferToPane(get().activePaneId, id, true)
      } else if (spec.type === 'agent') {
        const agentCount = get().buffers.filter(b => b.type === 'agent').length
        const sessionId = spec.sessionId ?? nanoid()
        const buf: AgentContent = {
          id,
          type: 'agent',
          sessionId,
          path: `agent://${sessionId}`,
          name: `Agent ${agentCount + 1}`,
          isPinned: false,
          isPreview: false,
          isActive: false,
        }
        set(state => { state.buffers.push(buf as PaneContent) })
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
      let found = false
      set(state => {
        const buf = state.buffers.find(b => b.id === id)
        if (buf) {
          buf.isPreview = false
          found = true
        }
      })
      if (found) get().paneActions.clearPreviewBufferEverywhere(id)
    },

    getBufferById(id) {
      return get().buffers.find(b => b.id === id)
    },

    registerExternalBuffer(id, path, name, isPreview = false) {
      const existing = get().buffers.find(b => b.id === id)

      if (existing) {
        // Don't downgrade a permanent tab to a preview on single-click.
        // Only update isPreview if:
        //   (a) current buffer is already a preview and we're keeping/clearing it, or
        //   (b) we're promoting it to permanent (isPreview = false)
        const willBecomePreview = isPreview && !existing.isPreview
        if (!willBecomePreview) {
          set(state => {
            const buf = state.buffers.find(b => b.id === id)
            if (buf) buf.isPreview = isPreview
          })
          get().paneActions.addBufferToPane(get().activePaneId, id, true)
          if (isPreview) {
            get().paneActions.setPanePreviewBuffer(get().activePaneId, id)
          } else {
            get().paneActions.clearPreviewBufferEverywhere(id)
          }
        } else {
          // Buffer is already permanent — just activate it without changing preview status
          get().paneActions.addBufferToPane(get().activePaneId, id, true)
        }
        return id
      }

      // New buffer — close any existing preview in the active pane first
      if (isPreview) {
        const activePaneId = get().activePaneId
        const pane = get().paneActions.getPaneById(activePaneId)
        if (pane?.previewBufferId && pane.previewBufferId !== id) {
          const oldId = pane.previewBufferId
          set(state => { state.buffers = state.buffers.filter(b => b.id !== oldId) })
          get().paneActions.removeBufferFromPane(activePaneId, oldId)
        }
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
      set(state => { state.buffers.push(buf as PaneContent) })
      get().paneActions.addBufferToPane(get().activePaneId, id, true)
      if (isPreview) get().paneActions.setPanePreviewBuffer(get().activePaneId, id)
      return id
    },
  },
})
