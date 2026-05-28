import type { StateCreator } from 'zustand'
import type { WorkspaceState } from '../workspace-store.types'
import type {
  PaneContent,
  OpenContentSpec,
  EditorContent,
  CrowbarChatContent,
  DiffContent,
  TerminalContent,
  WebViewerContent,
  AgentContent,
  NewTabContent,
  ImageContent,
  PdfContent,
  BinaryContent,
  DatabaseContent,
  PullRequestContent,
  GitHubIssueContent,
  GitHubActionContent,
  MarkdownPreviewContent,
  HtmlPreviewContent,
  CsvPreviewContent,
  ExternalEditorContent,
  GlobalSearchContent,
  DiagnosticsContent,
  ReferencesContent,
  OnboardingContent,
  ClosedBuffer,
  PendingClose,
} from '@/features/panes/types/pane-content'
import { shouldStartLsp } from '@/features/panes/types/pane-content'
import { getAllPaneGroups } from '@/features/panes/utils/pane-tree'
import { EDITOR_CONSTANTS } from '@/features/editor/config/constants'
import { nanoid } from 'nanoid'

// ── Constants ────────────────────────────────────────────────────────

const AUTO_EVICTION_PROTECTED = new Set<PaneContent['type']>([
  'agent', 'externalEditor', 'terminal', 'webViewer',
])

// ── Actions ──────────────────────────────────────────────────────────

export interface BufferActions {
  openContent(spec: OpenContentSpec): string
  closeBuffer(id: string): void
  setPinned(id: string, pinned: boolean): void
  setPreview(id: string, preview: boolean): void
  promotePreview(id: string): void
  getBufferById(id: string): PaneContent | undefined
  reopenLastClosedBuffer(): void
  setPendingClose(pc: PendingClose | null): void
  confirmPendingClose(): void
}

// ── Slice ────────────────────────────────────────────────────────────

export interface BufferSlice {
  buffers: PaneContent[]
  closedBuffersHistory: ClosedBuffer[]
  pendingClose: PendingClose | null
  maxOpenTabs: number
  bufferActions: BufferActions
}

export const createBufferSlice: StateCreator<
  WorkspaceState,
  [['zustand/immer', never]],
  [],
  BufferSlice
> = (set, get) => ({
  buffers: [],
  closedBuffersHistory: [],
  pendingClose: null,
  maxOpenTabs: EDITOR_CONSTANTS.MAX_OPEN_TABS,

  bufferActions: {
    openContent(spec) {
      // Deduplicate: return existing buffer id if already open
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
        if (spec.type === 'terminal' && spec.sessionId) {
          return get().buffers.find(
            b => b.type === 'terminal' && (b as TerminalContent).sessionId === spec.sessionId,
          )
        }
        if (spec.type === 'webViewer') {
          return get().buffers.find(
            b => b.type === 'webViewer' && (b as WebViewerContent).url === spec.url,
          )
        }
        if (spec.type === 'agent' && spec.sessionId) {
          return get().buffers.find(
            b => b.type === 'agent' && (b as AgentContent).sessionId === spec.sessionId,
          )
        }
        if (spec.type === 'image') {
          return get().buffers.find(b => b.type === 'image' && b.path === spec.path)
        }
        if (spec.type === 'pdf') {
          return get().buffers.find(b => b.type === 'pdf' && b.path === spec.path)
        }
        if (spec.type === 'binary') {
          return get().buffers.find(b => b.type === 'binary' && b.path === spec.path)
        }
        if (spec.type === 'database') {
          return get().buffers.find(b => b.type === 'database' && b.path === spec.path)
        }
        if (spec.type === 'pullRequest') {
          return get().buffers.find(
            b => b.type === 'pullRequest' && (b as PullRequestContent).prNumber === spec.prNumber,
          )
        }
        if (spec.type === 'githubIssue') {
          return get().buffers.find(
            b => b.type === 'githubIssue' &&
              (b as GitHubIssueContent).issueNumber === spec.issueNumber,
          )
        }
        if (spec.type === 'githubAction') {
          return get().buffers.find(
            b => b.type === 'githubAction' && (b as GitHubActionContent).runId === spec.runId,
          )
        }
        if (spec.type === 'markdownPreview') {
          return get().buffers.find(b => b.type === 'markdownPreview' && b.path === spec.path)
        }
        if (spec.type === 'htmlPreview') {
          return get().buffers.find(b => b.type === 'htmlPreview' && b.path === spec.path)
        }
        if (spec.type === 'csvPreview') {
          return get().buffers.find(b => b.type === 'csvPreview' && b.path === spec.path)
        }
        if (spec.type === 'externalEditor') {
          return get().buffers.find(b => b.type === 'externalEditor' && b.path === spec.path)
        }
        if (spec.type === 'globalSearch') {
          return get().buffers.find(b => b.type === 'globalSearch')
        }
        if (spec.type === 'diagnostics') {
          return get().buffers.find(b => b.type === 'diagnostics')
        }
        if (spec.type === 'references') {
          return get().buffers.find(b => b.type === 'references')
        }
        if (spec.type === 'onboarding') {
          return get().buffers.find(b => b.type === 'onboarding')
        }
        return undefined
      })()

      if (existing) {
        get().paneActions.addBufferToPane(get().activePaneId, existing.id, true)
        return existing.id
      }

      // Auto-evict when at max tabs (before creating a new buffer)
      if (get().buffers.length >= get().maxOpenTabs) {
        const evictee = get().buffers.find(b => !b.isPinned && !AUTO_EVICTION_PROTECTED.has(b.type))
        if (evictee) {
          const allPanes = [
            ...getAllPaneGroups(get().paneRoot),
            ...getAllPaneGroups(get().bottomRoot),
          ]
          for (const pane of allPanes) {
            if (pane.bufferIds.includes(evictee.id)) {
              get().paneActions.removeBufferFromPane(pane.id, evictee.id, true)
            }
          }
          set(state => { state.buffers = state.buffers.filter(b => b.id !== evictee.id) })
        }
      }

      const id = nanoid()

      // Build the new buffer object
      let buf: PaneContent

      if (spec.type === 'editor') {
        const isPreview = spec.isPreview ?? false
        buf = {
          id, type: 'editor',
          path: spec.path, name: spec.name,
          content: spec.content, savedContent: spec.content,
          isDirty: false,
          isVirtual: spec.isVirtual ?? false,
          language: spec.language,
          tokens: [],
          isPinned: false, isPreview, isActive: false,
        } satisfies EditorContent
      } else if (spec.type === 'crowbarChat') {
        buf = {
          id, type: 'crowbarChat',
          wsId: spec.wsId, name: spec.name,
          path: '', isPinned: false, isPreview: false, isActive: false,
        } satisfies CrowbarChatContent
      } else if (spec.type === 'diff') {
        buf = {
          id, type: 'diff',
          path: spec.path, name: spec.name,
          content: spec.content, savedContent: spec.content,
          diffData: spec.diffData,
          isPinned: false, isPreview: false, isActive: false,
        } satisfies DiffContent
      } else if (spec.type === 'terminal') {
        const terminalCount = get().buffers.filter(b => b.type === 'terminal').length
        const sessionId = spec.sessionId ?? `terminal-tab-${Date.now()}`
        buf = {
          id, type: 'terminal',
          sessionId,
          path: spec.path ?? `terminal://${sessionId}`,
          name: spec.name ?? `Terminal ${terminalCount + 1}`,
          initialCommand: spec.command,
          workingDirectory: spec.workingDirectory,
          remoteConnectionId: spec.remoteConnectionId,
          isPinned: false, isPreview: false, isActive: false,
        } satisfies TerminalContent
      } else if (spec.type === 'webViewer') {
        let displayName = 'Web Viewer'
        if (spec.url && spec.url !== 'about:blank') {
          try { displayName = new URL(spec.url).hostname || displayName } catch { /* invalid url */ }
        }
        buf = {
          id, type: 'webViewer',
          url: spec.url,
          path: `web-viewer://${spec.url}`,
          name: displayName,
          zoomLevel: spec.zoomLevel,
          profileKey: spec.profileKey,
          history: spec.history,
          historyIndex: spec.historyIndex,
          isPinned: false, isPreview: false, isActive: false,
        } satisfies WebViewerContent
      } else if (spec.type === 'agent') {
        const agentCount = get().buffers.filter(b => b.type === 'agent').length
        const sessionId = spec.sessionId ?? nanoid()
        buf = {
          id, type: 'agent',
          sessionId,
          path: `agent://${sessionId}`,
          name: `Agent ${agentCount + 1}`,
          isPinned: false, isPreview: false, isActive: false,
        } satisfies AgentContent
      } else if (spec.type === 'newTab') {
        buf = {
          id, type: 'newTab',
          path: '', name: 'New Tab',
          isPinned: false, isPreview: false, isActive: false,
        } satisfies NewTabContent
      } else if (spec.type === 'image') {
        buf = {
          id, type: 'image',
          path: spec.path, name: spec.name,
          isPinned: false, isPreview: false, isActive: false,
        } satisfies ImageContent
      } else if (spec.type === 'pdf') {
        buf = {
          id, type: 'pdf',
          path: spec.path, name: spec.name,
          isPinned: false, isPreview: false, isActive: false,
        } satisfies PdfContent
      } else if (spec.type === 'binary') {
        buf = {
          id, type: 'binary',
          path: spec.path, name: spec.name,
          isPinned: false, isPreview: false, isActive: false,
        } satisfies BinaryContent
      } else if (spec.type === 'database') {
        buf = {
          id, type: 'database',
          path: spec.path, name: spec.name,
          databaseType: spec.databaseType,
          connectionId: spec.connectionId,
          isPinned: false, isPreview: false, isActive: false,
        } satisfies DatabaseContent
      } else if (spec.type === 'pullRequest') {
        buf = {
          id, type: 'pullRequest',
          path: `pr://${spec.prNumber}`,
          name: spec.name ?? `PR #${spec.prNumber}`,
          prNumber: spec.prNumber,
          authorAvatarUrl: spec.authorAvatarUrl,
          isPinned: false, isPreview: false, isActive: false,
        } satisfies PullRequestContent
      } else if (spec.type === 'githubIssue') {
        buf = {
          id, type: 'githubIssue',
          path: `github-issue://${spec.issueNumber}`,
          name: spec.name ?? `Issue #${spec.issueNumber}`,
          issueNumber: spec.issueNumber,
          repoPath: spec.repoPath,
          authorAvatarUrl: spec.authorAvatarUrl,
          url: spec.url,
          isPinned: false, isPreview: false, isActive: false,
        } satisfies GitHubIssueContent
      } else if (spec.type === 'githubAction') {
        buf = {
          id, type: 'githubAction',
          path: `github-action://${spec.runId}`,
          name: spec.name ?? `Action #${spec.runId}`,
          runId: spec.runId,
          repoPath: spec.repoPath,
          url: spec.url,
          isPinned: false, isPreview: false, isActive: false,
        } satisfies GitHubActionContent
      } else if (spec.type === 'markdownPreview') {
        buf = {
          id, type: 'markdownPreview',
          path: spec.path, name: spec.name,
          content: spec.content,
          sourceFilePath: spec.sourceFilePath,
          isPinned: false, isPreview: false, isActive: false,
        } satisfies MarkdownPreviewContent
      } else if (spec.type === 'htmlPreview') {
        buf = {
          id, type: 'htmlPreview',
          path: spec.path, name: spec.name,
          content: spec.content,
          sourceFilePath: spec.sourceFilePath,
          isPinned: false, isPreview: false, isActive: false,
        } satisfies HtmlPreviewContent
      } else if (spec.type === 'csvPreview') {
        buf = {
          id, type: 'csvPreview',
          path: spec.path, name: spec.name,
          content: spec.content,
          sourceFilePath: spec.sourceFilePath,
          isPinned: false, isPreview: false, isActive: false,
        } satisfies CsvPreviewContent
      } else if (spec.type === 'externalEditor') {
        buf = {
          id, type: 'externalEditor',
          path: spec.path, name: spec.name,
          terminalConnectionId: spec.terminalConnectionId,
          isPinned: false, isPreview: false, isActive: false,
        } satisfies ExternalEditorContent
      } else if (spec.type === 'globalSearch') {
        buf = {
          id, type: 'globalSearch',
          path: 'global-search://', name: 'Search',
          isPinned: false, isPreview: false, isActive: false,
        } satisfies GlobalSearchContent
      } else if (spec.type === 'diagnostics') {
        buf = {
          id, type: 'diagnostics',
          path: 'diagnostics://', name: 'Problems',
          isPinned: false, isPreview: false, isActive: false,
        } satisfies DiagnosticsContent
      } else if (spec.type === 'references') {
        buf = {
          id, type: 'references',
          path: 'references://', name: 'References',
          isPinned: false, isPreview: false, isActive: false,
        } satisfies ReferencesContent
      } else {
        // spec.type === 'onboarding'
        buf = {
          id, type: 'onboarding',
          path: 'onboarding://', name: 'Welcome',
          mode: spec.context.mode,
          currentVersion: spec.context.currentVersion ?? '',
          previousVersion: spec.context.previousVersion,
          isPinned: false, isPreview: false, isActive: false,
        } satisfies OnboardingContent
      }

      set(state => { state.buffers.push(buf) })
      get().paneActions.addBufferToPane(get().activePaneId, id, true)
      if (spec.type === 'editor' && spec.isPreview) {
        get().paneActions.setPanePreviewBuffer(get().activePaneId, id)
      }

      return id
    },

    closeBuffer(id) {
      const buf = get().buffers.find(b => b.id === id)
      if (buf && shouldStartLsp(buf)) {
        set(state => {
          const entry: ClosedBuffer = { path: buf.path, name: buf.name, isPinned: buf.isPinned }
          state.closedBuffersHistory.unshift(entry)
          if (state.closedBuffersHistory.length > EDITOR_CONSTANTS.MAX_CLOSED_BUFFERS_HISTORY) {
            state.closedBuffersHistory.pop()
          }
        })
      }
      set(state => { state.buffers = state.buffers.filter(b => b.id !== id) })
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
        if (buf) { buf.isPreview = false; found = true }
      })
      if (found) get().paneActions.clearPreviewBufferEverywhere(id)
    },

    getBufferById(id) {
      return get().buffers.find(b => b.id === id)
    },

    reopenLastClosedBuffer() {
      const entry = get().closedBuffersHistory[0]
      if (!entry) return
      set(state => { state.closedBuffersHistory.shift() })
      get().bufferActions.openContent({
        type: 'editor',
        path: entry.path,
        name: entry.name,
        content: '',
      })
    },

    setPendingClose(pc) {
      set(state => { state.pendingClose = pc })
    },

    confirmPendingClose() {
      const pc = get().pendingClose
      if (!pc) return
      set(state => { state.pendingClose = null })
      if (pc.type === 'single') {
        get().bufferActions.closeBuffer(pc.bufferId)
      }
      // Other close types (others, all, to-left, to-right) are handled by the
      // callers that set pendingClose — they call closeBuffer for each target
      // after confirmation. This resets the gate.
    },
  },
})
