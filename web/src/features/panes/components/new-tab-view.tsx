import { lazy, Suspense, useCallback, useMemo } from 'react'
import { File as FileIcon, TerminalWindow, ChatCircle } from '@phosphor-icons/react'
import { CrowbarWordmark } from '@/components/ui/crowbar-wordmark'
import { RepoIconMark, type RepoIconSource } from '@/components/layout/repo-icon-mark'
import { ProjectIconMark, type ProjectIconSource } from '@/components/layout/project-icon-mark'
import { AgentChatGlyph } from '@/features/agent/shared/agent-chat-glyph'
import { createChat } from '@/features/agent/api/agent-api'
import { toastSpawnFailure } from '@/features/agent/lib/spawn-error'
import { openAgentChat } from '@/features/agent/lib/open-agent-chat'
import { UNTITLED_CHAT_LABEL } from '@/features/agent/lib/chat-label'
import { useEffectiveChordMap } from '@/features/keymaps/hooks/use-effective-keymap'
import { formatChord } from '@/features/keymaps/utils/chord'
import {
  AGENT_NEW_CHAT,
  SIDEBAR_TAB_CHATS,
  TAB_NEW_FILE,
  TAB_NEW_TERMINAL,
} from '@/features/keymaps/registry'
import {
  useWorkspaceStore,
  useWorkspaceStoreContext,
} from '@/features/workspace/stores/workspace-context'
import { orderedChats } from '@/features/workspace/stores/slices/agent-chats-slice'
import { useSidebarStore } from '@/lib/store/sidebar'
import { useProjectStore, useProjectDataStore, EMPTY_PROJECTS } from '@/lib/store/projects'
import { deriveContextPillModel } from '@/components/layout/context-pill-model'
import { parseWorkspaceScopeFromPath } from '@/lib/workspace-scope'
import { dataOf } from '@/lib/loadable'
import { useRouterState } from '@tanstack/react-router'

// Lazy so the BAKED POINT CLOUD stays off the startup path. ascii-crowbar pulls
// in ascii-crowbar-cloud.ts, a single 180,000-character base64 literal, and a
// static import from here landed it in the entry shell chunk: 107,835 B gzip —
// 39% of that chunk — paid by every cold start for a decoration only ever drawn
// on a New Tab pane. Same treatment editor-pane.tsx gives Plate.
//
// The fallback is `null` and that is correct, not lazy shorthand: this is a
// pointer-events-none, aria-hidden backdrop positioned `absolute inset-0`, so it
// occupies no layout and its late arrival shifts nothing on the surface.
const AsciiCrowbar = lazy(() => import('@/features/panes/components/ascii-crowbar'))

/**
 * The surface is not a chat browser — the sidebar is (⌘2). Past this many, the
 * remainder collapses into one hand-off row, so the cluster keeps a fixed height
 * however many chats a worktree has accumulated.
 */
const HISTORY_CAP = 3

/**
 * "Where am I" — repo + branch for a repo workspace, project + "home" for
 * Project Home. Same derivation (and now the same two-line shape) as the context
 * pill (components/layout/context-pill.tsx), so the two can never disagree about
 * what workspace you are looking at.
 *
 * `avatar` is the owning REPO's mark, resolved here rather than off the pill
 * model: the model only carries `repoAvatar` for the repo-home workspace (the
 * pill swaps it in for the branch glyph there), whereas this surface wants the
 * repo's identity beside every branch. Reading it from the same `repos` array
 * the model is derived from keeps them naming the same repo without changing
 * what the pill draws.
 */
function useContextHeading(): {
  eyebrow: string
  title: string
  avatar?: RepoIconSource
  /** Set on Project Home: the project's own mark, drawn by the same component
   *  the sidebar row and the context pill use. */
  project?: ProjectIconSource
  isHome: boolean
} {
  const pathname = useRouterState({ select: (s) => s.location.pathname })
  const repos = useSidebarStore((s) => s.repos)
  const projects = useProjectDataStore((s) => dataOf(s.data) ?? EMPTY_PROJECTS)
  const activeProjectId = useProjectStore((s) => s.activeProjectId)
  const activeWorkspaceId = parseWorkspaceScopeFromPath(pathname)?.wsId
  const model = deriveContextPillModel({
    activeWorkspaceId,
    isHomeRoute: !activeWorkspaceId && /\/ide\/[^/]+\/home$/.test(pathname),
    repos,
    projects,
    activeProjectId,
  })
  // The eyebrow is the repo (or the project, for Project Home); the title is the
  // branch (or "home"). Mirrors the pill's repo-over-branch stack.
  if (model.kind === 'workspace') {
    const repo = (repos ?? []).find(
      (r) =>
        r.defaultWorkspaceId === activeWorkspaceId ||
        r.workspaces.some((ws) => ws.id === activeWorkspaceId),
    )
    return {
      eyebrow: model.repoName,
      title: model.branchName,
      // No repo row (a workspace the sidebar hasn't loaded yet) → no mark, and
      // the heading falls back to the bare two-line stack rather than an empty box.
      avatar: repo && {
        name: repo.name,
        avatarURL: repo.avatarURL,
        avatarLabel: repo.avatarLabel,
        avatarColor: repo.avatarColor,
      },
      isHome: false,
    }
  }
  if (model.kind === 'home') {
    return {
      eyebrow: model.projectName,
      title: 'home',
      // Project Home marks itself with the PROJECT's icon, off the same model
      // the pill reads. It used to hardcode the Library glyph here, so a project
      // that had set an icon showed it in the sidebar and the pill and the
      // default mark on this surface.
      project: {
        name: model.projectName,
        avatarUrl: model.projectAvatarUrl,
        avatarEmoji: model.projectAvatarEmoji,
      },
      isHome: true,
    }
  }
  if (model.kind === 'project') return { eyebrow: '', title: model.projectName, isHome: false }
  return { eyebrow: '', title: '', isHome: false }
}

function Chord({ commandId }: { commandId: string }) {
  const chordMap = useEffectiveChordMap()
  const chord = chordMap[commandId]
  if (!chord) return null
  return (
    <kbd className="shrink-0 rounded border border-border bg-muted px-1.5 py-0.5 font-mono text-[10px] leading-none text-muted-foreground">
      {formatChord(chord)}
    </kbd>
  )
}

/**
 * The surface a pane shows when it holds nothing yet — and, since it is a real
 * `newTab` buffer, the surface ⌘T opens.
 *
 * A cluster centred in the pane reads top-down: worktree name (the heading) →
 * create actions → rule → recent chats. The cluster centres as a block but its
 * contents stay left-aligned in a single 300px measure — centring the rows would
 * ragged-edge them and their chord badges, which read as a list. The isologo sits
 * separately on the pane's bottom edge, centred, as an ambient brand mark.
 *
 * Deliberately ONE column at every pane size. It used to split side-by-side on a
 * wide pane, which reserved a fixed second column for a history that usually
 * holds a single item — a third of the composition sitting empty. A single
 * column cannot have that failure.
 */
export function NewTabView({ paneId }: { paneId: string }) {
  const workspaceStore = useWorkspaceStore()
  // The per-workspace store holds only `workspaceId`; the human-readable repo
  // and branch live in the sidebar store. `deriveContextPillModel` already
  // resolves both — and already handles Project Home, which has a project name
  // and no branch — so reuse it rather than re-deriving and drifting from the
  // context pill.
  const heading = useContextHeading()
  const wsId = useWorkspaceStoreContext((s) => s.workspaceId)
  const chats = useWorkspaceStoreContext((s) => s.agentChats.chats)
  const chatOrder = useWorkspaceStoreContext((s) => s.agentChats.order)
  const working = useWorkspaceStoreContext((s) => s.agentChats.working)
  // AgentChat carries activeProviderId, not a resolved icon — the icon SVG lives
  // on the workspace's provider list (agent-chats-panel.tsx / agent-chat-tab-icon.tsx
  // resolve it the same way). `?? []` is memoized (rather than inlined at every
  // read) so it stays referentially stable across renders where the store's own
  // list hasn't changed — otherwise the fallback array's fresh identity on every
  // nullish render would defeat every hook below that depends on `providers`.
  const providersRaw = useWorkspaceStoreContext((s) => s.agentChats.providers)
  const providers = useMemo(() => providersRaw ?? [], [providersRaw])
  const providerIcons = useMemo(() => new Map(providers.map((p) => [p.id, p.icon])), [providers])

  // "Recent" has no backing `updatedAt` to sort by (AgentChat carries only
  // createdAt), so this reuses the Chats SIDEBAR's own ordering (orderedChats:
  // pinned first in the user's chosen order, then createdAt NEWEST first) rather
  // than inventing a second, independent notion of recency. That is deliberate:
  // the two surfaces showing DIFFERENT top-N chats (I5) was the actual defect,
  // and the only ordering that can never disagree with "the top of the sidebar"
  // is the sidebar's own — which is also why the newest-first fix belongs in
  // orderedChats rather than here, where it would have split them again.
  const recent = useMemo(
    () => orderedChats(chats, chatOrder).slice(0, HISTORY_CAP),
    [chats, chatOrder],
  )
  const overflow = chats.length - recent.length

  // Every action here draws on THIS pane's surface, not necessarily the
  // active one (a background pane in a split renders its own New Tab too).
  // pane-container's mousedown-capture handler deliberately skips focusing the
  // pane when the mousedown target is a <button> — so without this, clicking
  // an action here would land its buffer in whichever OTHER pane was already
  // active (I7). Focusing first is also the natural read of "acting inside a
  // New Tab": the click focuses this pane, same as clicking anywhere else in
  // it (pane-container's handlePaneMouseDownCapture/onClick).
  const activateThisPane = useCallback(() => {
    workspaceStore.getState().paneActions.setActivePane(paneId)
  }, [workspaceStore, paneId])

  const openTerminal = useCallback(() => {
    activateThisPane()
    workspaceStore.getState().bufferActions.openContent({ type: 'terminal' })
  }, [activateThisPane, workspaceStore])

  // A virtual buffer needs neither a target directory nor write access, so New
  // File works on a locked worktree (protected branches, Project Home) where the
  // file-explorer's New File is hidden outright.
  const openFile = useCallback(() => {
    activateThisPane()
    workspaceStore.getState().bufferActions.openContent({
      type: 'editor',
      path: 'untitled:Untitled',
      name: 'Untitled',
      content: '',
      isVirtual: true,
    })
  }, [activateThisPane, workspaceStore])

  // Only the hand-off row ("N more in this worktree") goes to the sidebar — it
  // means "show me the rest", so a browser is the right destination. The
  // unified SidebarTree renders under 'workspaces' now; there is no separate
  // Chats tab to switch to.
  const openChatsTab = useCallback(() => useSidebarStore.getState().setActiveTab('workspaces'), [])

  // A named chat row opens THAT chat. Shared with the Chats sidebar
  // (open-agent-chat.ts) so both honour the same reveal rule: a chat already
  // open in another split is focused where it lives rather than having its tab
  // yanked into this pane.
  const openChat = useCallback(
    (chatId: string) => {
      activateThisPane()
      openAgentChat(workspaceStore, wsId, chatId)
    },
    [activateThisPane, workspaceStore, wsId],
  )

  // Provider-agnostic, per the design spec: this action does not ask which
  // provider, it just starts one — the FIRST ENABLED provider in priority order,
  // the same pick the sidebar's unified "New chat" row and the ⌘N chord make
  // (selectEnabledProviders). A disabled provider is hidden entirely (spec §2.2),
  // so it is never chosen even when it leads the list. Follows the sidebar panel's
  // own createChat call pattern (agent-chats-panel.tsx's `newChat`).
  // Whether ANY provider can start a chat. Same rule the sidebar's unified
  // New-chat row follows (the first enabled provider in priority order), and the
  // gate on this surface's own "New Chat" action — a row that stays live while
  // its handler can only return is a button that lies.
  const hasEnabledProvider = useMemo(() => providers.some((p) => p.enabled), [providers])

  const createNewChat = useCallback(() => {
    const provider = providers.find((p) => p.enabled)
    if (!provider) return
    // Focus this pane NOW (so the surface reacts immediately) and again right
    // before the buffer actually lands (in case the user moved focus elsewhere
    // while the request was in flight) — mirrors openTerminal/openFile, just
    // split across the async boundary.
    activateThisPane()
    createChat(wsId, provider.id)
      .then((chatId) => {
        const st = workspaceStore.getState()
        st.setActiveAgentChatId(chatId)
        st.paneActions.setActivePane(paneId)
        // A brand-new chat has no runner yet — null until it spawns one.
        st.paneActions.setPaneChat(paneId, chatId, null)
      })
      .catch((err: unknown) => toastSpawnFailure(err, provider.displayName, 'start'))
  }, [activateThisPane, paneId, providers, workspaceStore, wsId])

  return (
    // `pane-cq` lives here (not on the pane's shared content wrapper) so size
    // containment — which makes this element the containing block for its
    // `position: fixed` descendants — is scoped to the New Tab subtree only.
    // The shared wrapper hosts every buffer type (editor, terminal, agent
    // chat, …), including LSP's HoverTooltip, which is `fixed` and positions
    // itself against the viewport; putting containment there would make it
    // resolve against the pane box instead, for every buffer in every pane.
    // This div still fills the pane's content area exactly as the shared
    // wrapper used to, so the `@container pane` queries in theme.css keep
    // measuring the same box.
    // Column, not a stack of absolutes: the isologo is a real row at the
    // bottom and the cluster centres in what is left above it, so the two can
    // never overlap in a short pane the way a bottom-anchored absolute would.
    <div className="pane-cq relative flex h-full w-full flex-col overflow-hidden">
      {/* Decorative tumbling-crowbar ASCII backdrop. Sits BEHIND the cluster
          (earlier in DOM = painted first) and is pointer-events-none, so it
          never intercepts a click meant for an action row. Seeded by workspace
          so a cold workspace doesn't open on the same pose as the others.
          Code-split — see the lazy() above for why, and why null is the right
          fallback. */}
      <Suspense fallback={null}>
        <AsciiCrowbar seed={wsId} />
      </Suspense>
      {/* The cluster is centred in the pane, but its CONTENTS stay left-aligned:
          centring the text too would ragged-edge the action rows and their chord
          badges, which read as a list. So the outer box centres, the inner box
          is shrink-wrapped and start-aligned. */}
      <div className="relative grid min-h-0 flex-1 place-items-center overflow-hidden p-6">
        <div className="flex flex-col items-start gap-4">
          {/* Echoes the sidebar context pill: the repo (or project) as a muted
              eyebrow above the branch (or "home"), both in the mono face — with
              the repo's own mark leading the stack, which is what tells two
              same-named branches in different repos apart at a glance. */}
          <div className="flex items-center gap-2.5 px-2.5">
            {heading.avatar ? (
              <RepoIconMark repo={heading.avatar} size="xl" />
            ) : heading.project ? (
              <ProjectIconMark project={heading.project} size="xl" />
            ) : null}
            <div className="flex min-w-0 flex-col items-start gap-0.5">
              {heading.eyebrow && (
                <span className="ui-text-xs font-mono text-foreground/70">{heading.eyebrow}</span>
              )}
              <h2 className="ui-text-xl font-mono font-semibold text-foreground">
                {heading.title}
              </h2>
            </div>
          </div>

          {/* w-full (not a fixed w-[300px]) so the action + history rows follow
              the cluster's width, which the branch heading drives when it is long.
              The cluster is shrink-wrapped by its centring grid parent, so its
              width = the widest child; min-w-[300px] keeps this block's own
              intrinsic contribution at the old 300px floor, so a short branch
              still yields the same measure while a long one widens the rows to
              match the heading instead of leaving them pinned narrow. */}
          <div className="nt-cols flex w-full min-w-[300px] flex-col items-start">
            {/* Ordered by how often the action is actually wanted: starting a
                conversation is what this surface is opened to do (and it owns the
                unshifted ⌘N), a terminal next, and a new file last. */}
            <div className="flex w-full flex-col gap-0.5">
              <ActionRow
                icon={<ChatCircle />}
                label="New Chat"
                commandId={AGENT_NEW_CHAT}
                onClick={createNewChat}
                // Same sentence the Chats sidebar shows in place of its New-chat
                // row, so the two dead surfaces give the user one answer.
                disabledReason={
                  hasEnabledProvider
                    ? undefined
                    : 'No agents are enabled. Turn one on in Settings → Agents.'
                }
              />
              <ActionRow
                icon={<TerminalWindow />}
                label="New Terminal"
                commandId={TAB_NEW_TERMINAL}
                onClick={openTerminal}
              />
              <ActionRow
                icon={<FileIcon />}
                label="New File"
                commandId={TAB_NEW_FILE}
                onClick={openFile}
              />
            </div>

            {chats.length > 0 && (
              <>
                {/* A rule, not a label. The two groups need separating or they
                    read as one six-item list, but a heading here would sit in
                    the gap above the first action row with nothing to balance it. */}
                <div className="my-2.5 h-px w-[calc(100%-1.25rem)] self-center bg-border" />
                <div className="nt-history flex w-full flex-col gap-0.5">
                  {recent.map((chat) => (
                    <button
                      key={chat.id}
                      type="button"
                      data-testid="nt-chat-row"
                      onClick={() => openChat(chat.id)}
                      className="flex min-h-[30px] w-full items-center gap-2.5 rounded-md px-2.5 py-1.5 text-left leading-[18px] hover:bg-muted"
                    >
                      <AgentChatGlyph
                        providerIcon={providerIcons.get(chat.activeProviderId) ?? ''}
                        working={Boolean(working[chat.id])}
                        className="size-3.5"
                      />
                      <span className="ui-text-xs min-w-0 flex-1 truncate text-foreground">
                        {chat.title || UNTITLED_CHAT_LABEL}
                      </span>
                    </button>
                  ))}
                  {overflow > 0 && (
                    <button
                      type="button"
                      onClick={openChatsTab}
                      className="flex min-h-[30px] w-full items-center gap-2.5 rounded-md px-2.5 py-1.5 text-left leading-[18px] text-muted-foreground hover:bg-muted"
                    >
                      <span className="ui-text-xs min-w-0 flex-1 truncate pl-[22px]">
                        {overflow} more in this worktree
                      </span>
                      <Chord commandId={SIDEBAR_TAB_CHATS} />
                    </button>
                  )}
                </div>
              </>
            )}
          </div>
        </div>
      </div>

      {/* Isologo, centred on the pane's bottom edge. It is brand, not a control,
          so it is aria-hidden and pointer-events-none — a click near it falls
          through to the pane, same as clicking any empty area. Drawn in muted
          ink (not full foreground, not the brand green) so it settles into the
          empty stage rather than competing with the action rows for attention.
          Sized off the pane's shorter side so it neither towers in a tall pane
          nor vanishes in a split. */}
      <div className="relative flex shrink-0 justify-center pb-5">
        <CrowbarWordmark
          aria-hidden="true"
          className="pointer-events-none h-auto w-[clamp(96px,14cqmin,148px)] text-muted-foreground"
        />
      </div>
    </div>
  )
}

function ActionRow({
  icon,
  label,
  commandId,
  onClick,
  disabledReason,
}: {
  icon: React.ReactNode
  label: string
  commandId: string
  onClick: () => void
  /**
   * Why this action cannot run right now. Present ⇒ the row is a real disabled
   * <button> carrying the reason as its tooltip, rather than a live-looking row
   * (chord badge and all) whose handler silently returns — which is what "New
   * Chat" was with every provider turned off.
   */
  disabledReason?: string
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      disabled={disabledReason !== undefined}
      title={disabledReason}
      // min-h + leading pinned so these rows and the chat rows are the same
      // height: one is a <button> and one a <div>, and their default line boxes
      // differ by a pixel, which reads as two staggered columns.
      className="flex min-h-[30px] w-full items-center gap-2.5 rounded-md px-2.5 py-1.5 text-left leading-[18px] hover:bg-muted disabled:pointer-events-none disabled:opacity-50"
    >
      <span className="shrink-0 text-muted-foreground [&_svg]:size-3.5">{icon}</span>
      <span className="ui-text-xs min-w-0 flex-1 text-foreground">{label}</span>
      <Chord commandId={commandId} />
    </button>
  )
}
