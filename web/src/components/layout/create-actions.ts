import { useSidebarStore, type Repo } from '@/lib/store/sidebar'
import { createChatWithOwnWorktree } from '@/features/agent/api/agent-api'
import type { LandingChatPresentation } from '@/features/settings/lib/chat-presentation'
import { useFolderSignalStore } from '@/lib/store/folder-signal'
import { usePendingCreatesStore } from '@/lib/store/pending-creates'
import { toast } from '@/features/window/stores/toast-store'
import { resolveHomeRowScope } from '@/lib/store/home-tree'
import { rowsFromRepo, resolveHomeOwnerId } from '@/components/sidebar/lib/rows-from-repo'
import {
  navigateThenOpenChat,
  openChatInOwnView,
  resolveChatRow,
  resolveRow,
  type NavigateFn,
} from './open-actions'
import { startHomeRowThread } from './home-actions'
import {
  createInFlight,
  enabledProvider,
  failCreate,
  panelRowsAtClick,
  startThread,
  untilLanded,
} from './thread-create'

// The sidebar's "+" on a row: a fork (named first, then minted) or a thread.

function waitForRow(predicate: (repos: readonly Repo[]) => boolean): Promise<void> {
  return untilLanded(
    (onChange) => useSidebarStore.subscribe(onChange),
    () => predicate(useSidebarStore.getState().repos),
  )
}

/** Whether some repo's chat list now carries `chatId`, ALREADY placed under
 *  `parentId` — true only once the create that minted it has both reseeded
 *  AND its placement write has landed, never merely once its POST resolved
 *  or the chat merely exists.
 *
 *  `parentId`, and checking it, is load-bearing — this is `forkHasLanded`'s
 *  own shape, not the bare existence check this function used to be.
 *  Sidebar-placement-unification Task 8 moved a repo-scoped chat's placement
 *  onto a separate `Node` write (`CreateChat`'s own `MintChat` then
 *  `placeChat`, chats.go) — the SAME two-aggregate split Task 5 gave home
 *  rows — so `chat.parentId` no longer reliably reflects where the create
 *  actually landed the instant the chat itself is merely observed to exist:
 *  the chat lifecycle hub broadcasts on `MintChat`'s own commit alone, with
 *  no idea the placement write is still in flight, so a reseed can land here
 *  showing the chat already existing but still parented at root. See
 *  `waitForHomeChat`'s own doc, which pins the identical race for home. */
function chatHasLanded(chatId: string, parentId: string): (repos: readonly Repo[]) => boolean {
  return (repos) =>
    repos.some((r) => r.chats?.some((c) => c.id === chatId && c.parentId === parentId))
}

/** `chatHasLanded`'s own twin for a FORK: true only once BOTH halves have
 *  arrived, correctly placed — the chat (under `parentId`) AND the workspace
 *  it owns.
 *
 *  A workspace mints its owning chat chat-first (rows-from-repo.ts's own
 *  doc), so the two land as separate reseed frames, never atomically. Until
 *  the workspace frame catches up, `rows-from-repo.ts` has no WORKSPACE
 *  NODE to fold this chat onto — its render position falls through to the
 *  chat's OWN placement rules (parentId, then workspaceId-as-ground, which
 *  fails since that workspace isn't in the tree yet), landing it at the
 *  REPO ROOT rather than nested under the branch it was actually forked
 *  from. Clearing the pending spinner on existence alone revealed exactly
 *  that frame — caught live: a fresh fork appeared outside its parent for a
 *  beat, shoving every row below it down, before snapping into its real
 *  nested position the instant the workspace frame landed.
 *
 *  The chat's OWN `parentId` match is required on top of that, for the same
 *  reason `chatHasLanded` now checks it: a fork's placement is ALSO a
 *  separate Node write from its mint (`createOwnWorktreeChat` calls the same
 *  `MintChat`-then-`placeChat` sequence), so the workspace-owner half landing
 *  does not by itself guarantee the CHAT half's placement has too. */
function forkHasLanded(chatId: string, parentId: string): (repos: readonly Repo[]) => boolean {
  return (repos) =>
    repos.some(
      (r) =>
        r.chats?.some((c) => c.id === chatId && c.parentId === parentId) &&
        r.workspaces.some((w) => w.owningChatId === chatId),
    )
}

/** A fork create armed by `handleCreate`'s 'workspace' branch, waiting on the
 *  name the user types into the pending row's inline input before it can
 *  actually fire — keyed by that row's `tempId`. `confirmPendingCreateName`
 *  and `cancelPendingCreate` are the only two ways an entry ever leaves this
 *  map, and each releases the `createInFlight` key with it. */
const armedBranchCreates = new Map<
  string,
  {
    projectId: string
    repoId: string
    providerId: string
    placementParentId: string
    navigate: NavigateFn
    release: () => void
  }
>()

/**
 * Creates a fork ('workspace') or a thread under `parentId`, drawing an
 * optimistic row at its final slot the instant it is clicked. A fork asks for
 * its branch name first (`confirmPendingCreateName` mints it); a thread fires
 * immediately and opens the moment it exists. A project-home row can only
 * thread: home has no worktree to fork.
 */
export function handleCreate(
  parentId: string,
  kind: 'workspace' | 'thread',
  navigate: NavigateFn,
  /** The surface the new thread is created and landed on; undefined takes
   *  the user's default (`createSurfaceFor`). Never applies to a fork. */
  presentation?: LandingChatPresentation,
): void {
  const homeRow = resolveHomeRowScope(parentId)
  if (homeRow) {
    if (kind === 'thread') {
      startHomeRowThread(homeRow.projectId, homeRow.homeWorkspaceId, parentId, presentation)
    }
    return
  }

  const currentRepos = useSidebarStore.getState().repos
  // A bubble's Fork/Thread resolve against its GROUND workspace
  // (`Chat.workspaceId`); with no ground at all there is nothing to act on.
  const chatRow = resolveChatRow(currentRepos, parentId)
  if (chatRow && !chatRow.chat.workspaceId) return
  const found = resolveRow(currentRepos, chatRow?.chat.workspaceId ?? parentId)
  if (!found) return
  const { repo, subject } = found
  const { projectId } = repo
  if (!projectId) return

  if (kind === 'workspace') {
    const inFlightKey = `workspace:${parentId}`
    if (createInFlight.has(inFlightKey)) return
    createInFlight.add(inFlightKey)
    const release = (): void => {
      createInFlight.delete(inFlightKey)
    }
    // Task 8: mints the workspace AND its first chat in ONE call (POST
    // .../chats {ownWorktree: true} — backend Task 7) instead of the old
    // chat-less postWorkspace, which produced a bare branch row now and a
    // separate child chat row only once something else later started a
    // conversation in it. The parent named below is the clicked row's own
    // fork parent, same as the old mapping (a folder is the one exception the
    // old `placement` distinguished — that split has no counterpart on this
    // endpoint's single `parentId`, so a folder click also just names
    // itself here) — resolved to the chat that owns it, see below.
    const provider = enabledProvider()
    if (!provider) {
      release()
      return
    }
    // The daemon places by CHAT id, and the clicked row's own id is only that
    // id for a branch row (a locked branch, the repo home — `rows-from-repo.ts`
    // draws those AS their owning `branch` chat). A REGULAR fork's row is id'd
    // from its `Workspace`, because its owner is an ordinary conversation
    // already drawn beside it, so its owning chat has to be read off the
    // workspace record instead. Falls back to the clicked row for the ids that
    // name no workspace of this repo (the repo home, a folder) and for a frame
    // that carries no owner yet.
    const owningChatId = repo.workspaces.find((w) => w.id === subject.id)?.owningChatId
    // `parentId` is only a safe fallback for a DIRECT click on the row itself
    // (its own rendered id already equals whatever this resolves to). A
    // bubble's ground workspace can ALSO be the repo home — never in
    // `repo.workspaces` for `owningChatId` to be read off — so a bubble
    // forking the home workspace needs the SAME resolution the home row's
    // own id was rendered with, not the clicked bubble's unrelated id.
    const placementParentId =
      owningChatId ||
      (subject.id === repo.defaultWorkspaceId
        ? resolveHomeOwnerId(subject.id, repo.defaultOwningChatId, repo.chats ?? [])
        : parentId)
    // The new fork's OWN tree position, once real: nested under
    // `placementParentId`, never `subject.id`. `walkTreeIntoRows` stamps a
    // REAL child row's own `parentId` with its parent's RENDERED id —
    // `node.id`, already folded onto the owning chat for any branch row that
    // resolved one (rows-from-repo.ts) — so a sibling count (and the pending
    // row's own `parentId`) keyed on `subject.id`'s raw WORKSPACE-id-space
    // value matches no real row at all whenever an owning chat exists (the
    // normal case), landing the naming/spinner row at the sidebar ROOT
    // instead of nested under the clicked branch — caught live: forking
    // "main" drew its naming input as a top-level row after every other
    // project's, not under "main" where its real fork lands.
    // `placementParentId` is exactly `handleCreate`'s own OTHER id — already
    // the rendered/folded parent id, since it is either the resolved owning
    // chat or the clicked row's own (already-rendered) id. EVERY sibling
    // counts here, any kind — folders, branches AND chats interleave on one
    // dense order (workspace-tree-utils.ts's own doc), and the backend's own
    // placement write (owning_chat.go's placeOwningRow) appends a new fork by
    // counting ALL existing rows under the same parent chat id, not just the
    // branch/folder-kind ones — a narrower count here would produce an order
    // value real siblings already hold, landing the pending row somewhere
    // other than the tail slot the real create actually appends to.
    const siblingRows = rowsFromRepo(repo)
    const order = siblingRows.filter((r) => r.parentId === placementParentId).length
    // Only one naming input is ever open at once (matching the old tree's
    // own single `creatingChildOf`) — replacing rather than stacking a
    // second one, and releasing whatever the FIRST one held (its
    // `createInFlight` lock, its `armedBranchCreates` entry) so opening a
    // second one elsewhere can never orphan the first's.
    const otherNaming = usePendingCreatesStore.getState().entries.find((e) => e.status === 'naming')
    if (otherNaming) cancelPendingCreate(otherNaming.tempId)
    const tempId = `pending-${crypto.randomUUID()}`
    armedBranchCreates.set(tempId, {
      projectId,
      repoId: repo.id,
      providerId: provider.id,
      placementParentId,
      navigate,
      release,
    })
    usePendingCreatesStore.getState().startNaming({
      tempId,
      kind: 'branch',
      projectId,
      parentId: placementParentId,
      order,
      workspaceId: null,
      ownsWorktree: true,
    })
    return
  }

  // A thread runs in a real workspace: a `workspace` subject is one; a folder
  // takes its nearest owning workspace, stamped on its row at build time.
  const siblingRows = rowsFromRepo(repo)
  const wsId =
    subject.kind === 'workspace'
      ? subject.id
      : (siblingRows.find((r) => r.id === subject.id)?.workspaceId ?? null)
  if (!wsId) {
    toast.error('Start a thread from a workspace row — a folder has none to run it in')
    return
  }
  void startThread({
    inFlightKey: `thread:${parentId}`,
    projectId,
    workspaceId: wsId,
    parentId,
    presentation,
    landed: (chatId) => waitForRow(chatHasLanded(chatId, parentId)),
    onCreated: (chatId) => {
      announceTreeChange(repo.id)
      if (!openChatInOwnView(chatId, wsId)) {
        void navigateThenOpenChat(navigate, { projectId, repoId: repo.id, wsId }, chatId)
      }
    },
  })
}

/**
 * Confirms a fork's pending row — the inline input's Enter/blur — with the
 * typed branch name: flips the row to its spinner state and fires the
 * create `handleCreate`'s 'workspace' branch armed but did not send. Absent
 * from `armedBranchCreates` means the row already left naming (a stale
 * confirm racing a cancel elsewhere) — a no-op rather than a second request.
 */
export function confirmPendingCreateName(tempId: string, name: string): void {
  const armed = armedBranchCreates.get(tempId)
  if (!armed) return
  armedBranchCreates.delete(tempId)
  usePendingCreatesStore
    .getState()
    .confirmNaming(
      tempId,
      name,
      panelRowsAtClick(armed.projectId, armed.placementParentId).rowIdsAtClick,
    )
  // `armed.release` fires the moment the REQUEST itself settles — see the
  // identical reasoning on the thread path above; the same hang risk applies
  // here, and a stuck naming lock would leave every later "+" click on this
  // exact row permanently inert.
  createChatWithOwnWorktree(
    armed.projectId,
    armed.repoId,
    armed.providerId,
    armed.placementParentId,
    name,
  )
    .then((chatId) => {
      armed.release()
      announceTreeChange(armed.repoId)
      // Hides the real row (space-scroller.tsx's `unconfirmedRealIds`) from
      // first paint — see PendingCreateEntry.realId, and forkHasLanded's own
      // doc for the placement race this closes.
      usePendingCreatesStore.getState().attachRealId(tempId, chatId)
      return waitForRow(forkHasLanded(chatId, armed.placementParentId)).then(() => {
        usePendingCreatesStore.getState().clear(tempId)
        // Opens the new branch's own chat the moment it's real, same as the
        // thread path above — a fork only knows its OWN workspace id once
        // `forkHasLanded` confirms the owning-workspace half of the create
        // has landed (the API response carries only the chat id), so this
        // has to wait for that rather than firing right off the response.
        const wsId = useSidebarStore
          .getState()
          .repos.flatMap((r) => r.workspaces)
          .find((w) => w.owningChatId === chatId)?.id
        if (wsId && !openChatInOwnView(chatId, wsId)) {
          void navigateThenOpenChat(
            armed.navigate,
            { projectId: armed.projectId, repoId: armed.repoId, wsId },
            chatId,
          )
        }
      })
    })
    .catch((err: unknown) => {
      armed.release()
      failCreate(tempId, err, 'Failed to create workspace')
    })
}

/** Drops a pending row outright: a naming input the user cancelled (Escape,
 *  or blurred empty — never reached the network, nothing to roll back) or an
 *  error the user dismissed. Releasing `createInFlight` here, not in
 *  `handleCreate`, is what keeps a second "+" click on the same row inert
 *  for as long as its naming input is still open. */
export function cancelPendingCreate(tempId: string): void {
  const armed = armedBranchCreates.get(tempId)
  if (armed) {
    armedBranchCreates.delete(tempId)
    armed.release()
  }
  usePendingCreatesStore.getState().clear(tempId)
}

/**
 * Tell `repoId`'s sidebar tree to re-read its rows after this client created
 * one.
 *
 * See `removal-commit.ts`'s `bumpRepoTree` for the full story — same signal,
 * same reason, the create half. The daemon really does mint the chat and its
 * worktree (measured live: the repo's chat count went 9 -> 11 on two clicks),
 * but `openRepoTreeSubscription` reseeds `crowbar_chats` only on this
 * generation moving, and the only thing that normally moves it is a chat frame
 * arriving for a MOUNTED workspace of this repo. Fork from the sidebar with no
 * workspace of that repo open — on the project-home route, say — and the row
 * never appeared at all. The button had worked; nothing had drawn it.
 */
function announceTreeChange(repoId: string): void {
  if (repoId) useFolderSignalStore.getState().bump(repoId)
}
