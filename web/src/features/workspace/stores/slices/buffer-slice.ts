import type { StateCreator } from 'zustand'
import type { WorkspaceState } from '../workspace-store.types'
import type {
  PaneContent,
  OpenContentSpec,
  EditorContent,
  BranchReviewContent,
  AgentChatContent,
  CommitDiffContent,
  TerminalContent,
  NewTabContent,
  MarkdownPreviewContent,
  HtmlPreviewContent,
  CsvPreviewContent,
  ExternalEditorContent,
  ClosedBuffer,
  PendingClose,
} from '@/features/panes/types/pane-content'
import { shouldStartLsp, isEditorContent } from '@/features/panes/types/pane-content'
import { EDITOR_CONSTANTS } from '@/features/editor/config/constants'
import { fileUri } from '@/features/editor/lib/editor-uri'
import { useHistoryStore } from '@/features/editor/stores/history-store'
import { cleanupBufferHistoryTracking } from '@/features/editor/stores/buffer-history-tracking'
// Leaf module (zustand only, no Plate) — a static import here keeps the rich
// editor's chunk out of the base bundle while still giving closeBuffer a
// synchronous way to release the buffer's rich/source preference.
import { useMarkdownViewStore } from '@/features/editor/markdown/plate/markdown-view-store'
import { useSettingsStore } from '@/features/settings/store'
import type { WorkspaceStore } from '../workspace-store'
import { nanoid } from 'nanoid'

// ── Constants ────────────────────────────────────────────────────────

// 'newTab' is included alongside the always-live externalEditor/terminal:
// a pane with no tabs falls back to showing its (evicted) New Tab, so letting
// one be auto-evicted out from under a pane that is only holding it because
// it has nothing else would leave that pane with NO tabs at all — the exact
// state closing a pane's last real tab is supposed to degrade into, not skip
// past. See openNewTab's own doc comment for the invariant this protects.
const AUTO_EVICTION_PROTECTED = new Set<PaneContent['type']>([
  'externalEditor',
  'terminal',
  'newTab',
])

// ── Actions ──────────────────────────────────────────────────────────

export interface BufferActions {
  openContent(spec: OpenContentSpec): string
  /**
   * Open (or re-focus) the pane's New Tab. A pane holds at most ONE — a second
   * identical blank is clutter, not a feature — so this is idempotent per pane
   * and returns the id either way.
   *
   * Returns `null`, minting nothing, when `paneId` names no registered pane:
   * `addBufferToPane` would silently no-op for it, so pushing a buffer anyway
   * would attach it to no pane (inflating `buffers` for nothing to render) and
   * returning a fabricated id would tell the caller a New Tab now exists
   * somewhere it does not. `string | null` is the signature that can't lie.
   *
   * Never changes which pane is globally active — it only ensures the target
   * pane has a New Tab and makes it that PANE's active buffer. Moving global
   * focus (setActivePane / activatePaneBuffer, which both move it) is left to
   * the caller, because a caller may deliberately target a background pane
   * (e.g. re-seeding a pane whose last real tab just closed) and must not have
   * focus stolen out from under whatever the user is doing elsewhere.
   */
  openNewTab(paneId?: string): string | null
  closeBuffer(id: string): void
  renameBuffer(id: string, name: string): void
  repointAgentChatBuffer(id: string, to: { chatId: string; runnerId: string }): void
  setPinned(id: string, pinned: boolean): void
  setPreview(id: string, preview: boolean): void
  promotePreview(id: string): void
  getBufferById(id: string): PaneContent | undefined
  reopenLastClosedBuffer(): void
  setPendingClose(pc: PendingClose | null): void
  confirmPendingClose(): void
}

/** The one shape of a New Tab placeholder buffer. Both mint sites — `openContent`'s
 *  `newTab` spec and `openNewTab` — build the exact same literal, so it lives here
 *  once instead of twice (drift between the two was Finding 3). */
function makeNewTabBuffer(id: string): NewTabContent {
  return {
    id,
    type: 'newTab',
    path: '',
    name: 'New Tab',
    isPinned: false,
    isPreview: false,
    isActive: false,
  }
}

/** The New Tab a pane is holding, if any. A pane holds at most one. */
function findNewTabInPane(
  buffers: PaneContent[],
  pane: { bufferIds: string[] } | null | undefined,
): PaneContent | undefined {
  if (!pane) return undefined
  const byId = new Map(buffers.map((b) => [b.id, b]))
  for (const id of pane.bufferIds) {
    const buf = byId.get(id)
    if (buf?.type === 'newTab') return buf
  }
  return undefined
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
> = (set, get, api) => {
  // See pane-slice for why reaching `editorManager` through `api` is safe: it is
  // the non-reactive handle Object.assign'd onto the store after slice creation.
  const editorManagerOf = () => (api as unknown as WorkspaceStore).editorManager

  /** A New Tab is a placeholder for "something will go here". The moment
   *  something actually lands in `paneId` — a brand-new buffer, or an existing
   *  one deduped into it — that placeholder has served its purpose: replace it
   *  in place rather than leaving a blank tab sitting next to the thing it
   *  produced. Two call sites in `openContent` share this (the brand-new-buffer
   *  path and the dedup path that attaches an existing buffer to the active
   *  pane) — NOT the dedup path that jumps focus to a pane elsewhere instead,
   *  which lands nothing here and must leave `paneId`'s New Tab alone. */
  const consumeNewTabInPane = (paneId: string): void => {
    const staleNewTab = findNewTabInPane(get().buffers, get().paneActions.getPaneById(paneId))
    if (!staleNewTab) return
    get().paneActions.removeBufferFromPane(paneId, staleNewTab.id, true)
    set((state) => {
      state.buffers = state.buffers.filter((b) => b.id !== staleNewTab.id)
    })
  }

  return {
    buffers: [],
    closedBuffersHistory: [],
    pendingClose: null,
    maxOpenTabs: EDITOR_CONSTANTS.MAX_OPEN_TABS,

    bufferActions: {
      openContent(spec) {
        // Deduplicate: return existing buffer id if already open
        const existing = (() => {
          if (spec.type === 'editor') {
            return get().buffers.find((b) => b.type === 'editor' && b.path === spec.path)
          }
          if (spec.type === 'branchReview') {
            return get().buffers.find(
              (b) => b.type === 'branchReview' && (b as BranchReviewContent).wsId === spec.wsId,
            )
          }
          if (spec.type === 'agentChat') {
            return get().buffers.find(
              (b) => b.type === 'agentChat' && (b as AgentChatContent).chatId === spec.chatId,
            )
          }
          if (spec.type === 'commitDiff') {
            return get().buffers.find(
              (b) => b.type === 'commitDiff' && b.wsId === spec.wsId && b.sha === spec.sha,
            )
          }
          if (spec.type === 'terminal' && spec.sessionId) {
            return get().buffers.find(
              (b) => b.type === 'terminal' && (b as TerminalContent).sessionId === spec.sessionId,
            )
          }
          if (spec.type === 'markdownPreview') {
            return get().buffers.find((b) => b.type === 'markdownPreview' && b.path === spec.path)
          }
          if (spec.type === 'htmlPreview') {
            return get().buffers.find((b) => b.type === 'htmlPreview' && b.path === spec.path)
          }
          if (spec.type === 'csvPreview') {
            return get().buffers.find((b) => b.type === 'csvPreview' && b.path === spec.path)
          }
          if (spec.type === 'externalEditor') {
            return get().buffers.find((b) => b.type === 'externalEditor' && b.path === spec.path)
          }
          return undefined
        })()

        if (existing) {
          // A terminal or an agent chat buffer IS a live view onto ONE PTY. Opening
          // it again must REVEAL the view that already exists — jump to the pane
          // that holds it — never drop a SECOND copy into the active pane. Two views
          // of the same PTY race each other over the shared transport (the daemon
          // serializes its screen model to a client only at attach), and one of the
          // two ends up blank: the agent-chat duplication-blank bug, and the same
          // latent hazard for a terminal. Editors and everything else are safe to
          // surface in the active pane, so they keep the addBufferToPane path.
          if (existing.type === 'agentChat' || existing.type === 'terminal') {
            const pane = get().paneActions.getPaneByBufferId(existing.id)
            if (pane) {
              get().paneActions.setActivePane(pane.id)
              get().paneActions.activatePaneBuffer(pane.id, existing.id)
              return existing.id
            }
          }
          // Unlike the jump-path above, something lands in the active pane here —
          // see consumeNewTabInPane's doc comment for why that consumes it first.
          consumeNewTabInPane(get().activePaneId)
          get().paneActions.addBufferToPane(get().activePaneId, existing.id, true)
          return existing.id
        }

        // Auto-evict when at max tabs (before creating a new buffer).
        //
        // ONE cap, enforced here. `maxOpenTabs` on this slice is the engine's
        // own budget; the user's `settings.maxOpenTabs` narrows it when they ask
        // for fewer tabs than that. Until now the setting was enforced a second
        // time, in the tab bar, by an effect that watched the workspace-wide
        // buffer list and closed tabs through the pane's own close handler — so
        // it only bit below the engine budget, and with two panes open BOTH tab
        // bars trimmed the same shared list.
        const settingCap = useSettingsStore.getState().settings.maxOpenTabs
        const cap = settingCap > 0 ? Math.min(get().maxOpenTabs, settingCap) : get().maxOpenTabs
        if (get().buffers.length >= cap) {
          const evictee = get().buffers.find(
            (b) => !b.isPinned && !AUTO_EVICTION_PROTECTED.has(b.type),
          )
          if (evictee) {
            const allPanes = Object.values(get().panes)
            for (const pane of allPanes) {
              if (pane.bufferIds.includes(evictee.id)) {
                get().paneActions.removeBufferFromPane(pane.id, evictee.id, true)
              }
            }
            set((state) => {
              state.buffers = state.buffers.filter((b) => b.id !== evictee.id)
            })
          }
        }

        const id = nanoid()

        // Build the new buffer object
        let buf: PaneContent

        if (spec.type === 'editor') {
          const isPreview = spec.isPreview ?? false
          buf = {
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
          } satisfies EditorContent
        } else if (spec.type === 'branchReview') {
          buf = {
            id,
            type: 'branchReview',
            wsId: spec.wsId,
            name: spec.name,
            path: `branch-review://${spec.wsId}`,
            isPinned: false,
            isPreview: false,
            isActive: false,
          } satisfies BranchReviewContent
        } else if (spec.type === 'agentChat') {
          buf = {
            id,
            type: 'agentChat',
            chatId: spec.chatId,
            // '' is the honest default: the caller may not know (or may not have)
            // a runner. The pane adopts whatever runner its chat really has.
            runnerId: spec.runnerId ?? '',
            wsId: spec.wsId,
            name: spec.name,
            path: `agent-chat://${spec.chatId}`,
            isPinned: false,
            isPreview: false,
            isActive: false,
          } satisfies AgentChatContent
        } else if (spec.type === 'commitDiff') {
          buf = {
            id,
            type: 'commitDiff',
            wsId: spec.wsId,
            sha: spec.sha,
            name: spec.name,
            // One tab per commit per workspace: reopening the same commit
            // focuses the tab that is already there rather than stacking another.
            path: `commit-diff://${spec.wsId}/${spec.sha}`,
            isPinned: false,
            isPreview: false,
            isActive: false,
          } satisfies CommitDiffContent
        } else if (spec.type === 'terminal') {
          const terminalCount = get().buffers.filter((b) => b.type === 'terminal').length
          const sessionId = spec.sessionId ?? `terminal-tab-${Date.now()}`
          buf = {
            id,
            type: 'terminal',
            sessionId,
            path: spec.path ?? `terminal://${sessionId}`,
            name: spec.name ?? `Terminal ${terminalCount + 1}`,
            initialCommand: spec.command,
            workingDirectory: spec.workingDirectory,
            remoteConnectionId: spec.remoteConnectionId,
            isPinned: false,
            isPreview: false,
            isActive: false,
          } satisfies TerminalContent
        } else if (spec.type === 'newTab') {
          buf = makeNewTabBuffer(id)
        } else if (spec.type === 'markdownPreview') {
          buf = {
            id,
            type: 'markdownPreview',
            path: spec.path,
            name: spec.name,
            content: spec.content,
            sourceFilePath: spec.sourceFilePath,
            isPinned: false,
            isPreview: false,
            isActive: false,
          } satisfies MarkdownPreviewContent
        } else if (spec.type === 'htmlPreview') {
          buf = {
            id,
            type: 'htmlPreview',
            path: spec.path,
            name: spec.name,
            content: spec.content,
            sourceFilePath: spec.sourceFilePath,
            isPinned: false,
            isPreview: false,
            isActive: false,
          } satisfies HtmlPreviewContent
        } else if (spec.type === 'csvPreview') {
          buf = {
            id,
            type: 'csvPreview',
            path: spec.path,
            name: spec.name,
            content: spec.content,
            sourceFilePath: spec.sourceFilePath,
            isPinned: false,
            isPreview: false,
            isActive: false,
          } satisfies CsvPreviewContent
        } else {
          // spec.type === 'externalEditor'
          buf = {
            id,
            type: 'externalEditor',
            path: spec.path,
            name: spec.name,
            terminalConnectionId: spec.terminalConnectionId,
            isPinned: false,
            isPreview: false,
            isActive: false,
          } satisfies ExternalEditorContent
        }

        set((state) => {
          state.buffers.push(buf)
        })
        // See consumeNewTabInPane's doc comment: a brand-new buffer landing here
        // consumes the active pane's own New Tab.
        consumeNewTabInPane(get().activePaneId)
        get().paneActions.addBufferToPane(get().activePaneId, id, true)
        if (spec.type === 'editor' && spec.isPreview) {
          get().paneActions.setPanePreviewBuffer(get().activePaneId, id)
        }

        return id
      },

      openNewTab(paneId) {
        const targetPane = paneId ?? get().activePaneId
        const pane = get().paneActions.getPaneById(targetPane)
        // Finding 1: an unregistered pane can hold nothing — see this action's
        // doc comment for why minting a buffer anyway (or lying about its id)
        // would be worse than doing nothing.
        if (!pane) return null

        const existing = findNewTabInPane(get().buffers, pane)
        // Both branches activate the New Tab as the TARGET PANE's own active
        // buffer via `addBufferToPane`'s `setActive` — the same mechanism
        // `openContent` already uses to re-surface a deduped buffer — and
        // never via setActivePane/activatePaneBuffer, both of which also move
        // GLOBAL focus (Finding 2). Which pane the user is looking at is the
        // caller's decision, not this action's.
        if (existing) {
          get().paneActions.addBufferToPane(targetPane, existing.id, true)
          return existing.id
        }
        const id = nanoid()
        set((state) => {
          state.buffers.push(makeNewTabBuffer(id))
        })
        get().paneActions.addBufferToPane(targetPane, id, true)
        return id
      },

      closeBuffer(id) {
        const buf = get().buffers.find((b) => b.id === id)
        // Closing a terminal tab is final (terminals never enter the undo-close
        // history) — terminate the backend PTY so shell processes don't leak.
        // Dynamic import avoids a workspace-slice → terminal-feature cycle.
        if (buf && buf.type === 'terminal') {
          const { sessionId } = buf as TerminalContent
          const workspaceId = get().workspaceId
          void import('@/features/terminal/lib/kill-terminal-session').then(
            async ({ killTerminalSession }) => {
              await killTerminalSession(sessionId).catch(() => {})
              // Clear the reconnect map entry so a stale connectionId can't be
              // picked up if the same tab sessionId is reused in a later session.
              const { clearReconnect } =
                await import('@/features/terminal/lib/terminal-reconnect-map')
              clearReconnect(workspaceId, sessionId)
            },
          )
        }
        // Closing an agent-chat tab STOPS its vendor CLI process but KEEPS the chat
        // entry, dormant and resumable: reopening the tab revives the real
        // conversation through the existing resume path (agent-chat-pane's auto-revive
        // on mount). This is deliberately NOT deleteChat — unlike a terminal (whose PTY
        // is destroyed), the chat and its bound conversation survive; only the live
        // process is gracefully terminated. The backend no-ops when the chat is already
        // dormant. Dynamic import avoids a workspace-slice → agent-feature cycle.
        if (buf && buf.type === 'agentChat') {
          const { chatId, wsId } = buf as AgentChatContent
          void import('@/features/agent/api/agent-api').then(async ({ stopChat }) => {
            await stopChat(wsId, chatId).catch(() => {})
          })
        }
        if (buf && shouldStartLsp(buf)) {
          set((state) => {
            const entry: ClosedBuffer = { path: buf.path, name: buf.name, isPinned: buf.isPinned }
            state.closedBuffersHistory.unshift(entry)
            if (state.closedBuffersHistory.length > EDITOR_CONSTANTS.MAX_CLOSED_BUFFERS_HISTORY) {
              state.closedBuffersHistory.pop()
            }
          })
        }
        // Release the held Monaco model for any pane that STILL holds this buffer.
        // The canonical close paths call `removeBufferFromPane` first (which already
        // released for that pane and stripped the id), so this only fires for panes
        // that were skipped (direct closeBuffer callers) or other panes holding the
        // same file — release exactly once per holding pane. Disposes the model when
        // the last holder releases, so a reopen reads fresh content (no stale model).
        if (buf && isEditorContent(buf)) {
          const uri = fileUri(buf.path)
          const manager = editorManagerOf()
          for (const pane of Object.values(get().panes ?? {})) {
            if (pane.bufferIds.includes(id)) manager?.closeBuffer(pane.id, uri)
          }
        }
        // Free git-blame data accumulated for this file so per-file Maps don't
        // grow unbounded across a long session. Dynamic import mirrors the pattern
        // used above for terminal/chat to avoid circular slice → git-feature deps.
        if (buf && isEditorContent(buf)) {
          const filePath = buf.path
          void import('@/features/git/stores/git-blame-store').then(({ useGitBlameStore }) => {
            useGitBlameStore.getState().clearBlameForFile(filePath)
          })
        }
        // Release this buffer's markdown rich/source preference. The view store
        // is keyed by bufferId and nothing else ever removes an entry, so
        // without this it grows for the life of the session (no-ops when the
        // buffer never had one).
        useMarkdownViewStore.getState().clearView(id)
        // Free full-content history snapshots so closed buffers don't leak memory.
        // clearHistory drops up to 100 HistoryEntry objects each holding a full copy
        // of the file text — the dominant source of memory growth in long sessions.
        cleanupBufferHistoryTracking(id)
        useHistoryStore.getState().actions.clearHistory(id)
        set((state) => {
          state.buffers = state.buffers.filter((b) => b.id !== id)
        })
      },

      // Rename an open buffer's tab label in place. `openContent` snapshots the
      // name at open time, so any content whose title can change AFTER the tab
      // exists (an agent chat auto-titled by the agent, or renamed by the user)
      // needs this to keep the tab in sync with its source of truth.
      renameBuffer(id, name) {
        set((state) => {
          const buf = state.buffers.find((b) => b.id === id)
          if (buf) buf.name = name
        })
      },

      // Re-point an agent-chat tab at the chat/runner it is showing NOW. Both ids
      // move, and they move for different reasons:
      //
      //   the RUNNER moved  → a new chatId, the SAME runnerId. The user typed
      //                       /clear or /resume inside the CLI and it switched
      //                       conversation; the tab follows the process. The PTY is
      //                       unchanged, so the terminal (keyed by it) never
      //                       remounts — the conversation changes without the
      //                       terminal changing.
      //   the chat was RESUMED, or its provider switched
      //                     → the same chatId, a new runnerId. A different process
      //                       is on the conversation now, with a different PTY, so
      //                       the terminal re-attaches.
      //
      // `path` tracks chatId so the buffer's identity never contradicts it. The tab
      // LABEL is not touched here: it follows the chat's title through renameBuffer,
      // which fires on the same store update.
      //
      // A no-op when nothing actually changed — an idempotent write would otherwise
      // mint a new buffers array on every render pass that re-asserts the same pair.
      repointAgentChatBuffer(id, to) {
        set((state) => {
          const buf = state.buffers.find((b) => b.id === id)
          if (!buf || buf.type !== 'agentChat') return
          if (buf.chatId === to.chatId && buf.runnerId === to.runnerId) return
          buf.chatId = to.chatId
          buf.runnerId = to.runnerId
          buf.path = `agent-chat://${to.chatId}`
        })
      },

      setPinned(id, pinned) {
        set((state) => {
          const buf = state.buffers.find((b) => b.id === id)
          if (buf) buf.isPinned = pinned
        })
      },

      setPreview(id, preview) {
        set((state) => {
          const buf = state.buffers.find((b) => b.id === id)
          if (buf) buf.isPreview = preview
        })
      },

      promotePreview(id) {
        let found = false
        set((state) => {
          const buf = state.buffers.find((b) => b.id === id)
          if (buf) {
            buf.isPreview = false
            found = true
          }
        })
        if (found) get().paneActions.clearPreviewBufferEverywhere(id)
      },

      getBufferById(id) {
        return get().buffers.find((b) => b.id === id)
      },

      reopenLastClosedBuffer() {
        const entry = get().closedBuffersHistory[0]
        if (!entry) return
        set((state) => {
          state.closedBuffersHistory.shift()
        })
        const id = get().bufferActions.openContent({
          type: 'editor',
          path: entry.path,
          name: entry.name,
          content: '',
        })
        // The history entry carries no content; load it from disk and fill the
        // buffer in place. Dynamic import avoids a slice → platform-controller
        // cycle. Skip the fill if the user already typed into the empty buffer.
        // Read from this store's own workspace: the active workspace can change
        // while the read is in flight, and the same relative path in a sibling
        // worktree holds different content.
        void import('@/features/file-system/controllers/platform').then(
          async ({ readWorkspaceFile }) => {
            try {
              const content = await readWorkspaceFile(get().workspaceId, entry.path)
              set((state) => {
                const buf = state.buffers.find((b) => b.id === id)
                if (buf && buf.type === 'editor' && buf.content === '') {
                  buf.content = content
                  buf.savedContent = content
                  buf.isDirty = false
                }
              })
            } catch {
              // File no longer exists — leave the empty buffer; saving will recreate it.
            }
          },
        )
      },

      setPendingClose(pc) {
        set((state) => {
          state.pendingClose = pc
        })
      },

      confirmPendingClose() {
        const pc = get().pendingClose
        if (!pc) return
        set((state) => {
          state.pendingClose = null
        })
        if (pc.type === 'single') {
          get().bufferActions.closeBuffer(pc.bufferId)
        }
        // Other close types (others, all, to-left, to-right) are handled by the
        // callers that set pendingClose — they call closeBuffer for each target
        // after confirmation. This resets the gate.
      },
    },
  }
}
