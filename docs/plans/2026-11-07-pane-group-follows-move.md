# A pane follows its chat when the chat moves

## Problem

`PaneGroup.workspaceId` records where a pane's chat ran when the pane opened it. It
exists so a layout restored at boot can place a chat before the chats list loads
(`lib/persistence/hydrate.ts`). Since a thread can move to another workspace, that
record goes stale: the chat entity's `workspaceId` changes and the pane's does not.

The render path already follows the chat. `usePaneWorkspace` and
`usePaneChatWorkspaceId` (`features/panes/hooks/use-pane-workspace.ts`) read the chat
entity first and fall back to the pane's record only until the chat is known. The
chat view, its editor tabs, and the phone's IDE screen use them.

These readers still take the pane's record as the truth:

| Reader                                                                                                                                                          | What goes wrong after a move                                                                                                       |
| --------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------- |
| `useActivePaneWorkspaceId`, `useViewWorkspaceIds` (`features/panes/hooks/use-chat-workspace-id.ts`), used by `use-ide-shell-workspace-retention.ts`             | The old workspace's store stays retained and the new one is not. That costs memory, and the new store may be rebuilt on first use. |
| `resolveOnscreenPaneForWorkspace`, `resolveChatWorkspaceId` (`features/panes/lib/pane-chat-workspace.ts`), used by the Files panel and the phone's native files | A file click in the new workspace does not land in the moved chat's pane. A click in the old one does.                             |
| `viewMembers`, `chatWorkspaceIn` (`features/panes/lib/view-state.ts`)                                                                                           | `release` and `adopt` match on the old workspace.                                                                                  |
| `release-closed-chat.ts`                                                                                                                                        | Closing the moved chat compares against the old workspace.                                                                         |
| `PaneHandle` (`features/panes/components/pane-handle.tsx`)                                                                                                      | The drag subject is built from the old workspace's row.                                                                            |
| `draftOfPane` (`components/layout/draft-thread.ts`)                                                                                                             | Only drafts read it, and a draft never moves, so this one is unaffected.                                                           |
| `recents-actions.ts`                                                                                                                                            | Opens the route under the old workspace. The route then resolves the chat, so the effect is one extra navigation.                  |

## Why not rewrite the pane on move

Copying the chat's new workspace into every pane that shows it would make the field a
second copy kept in sync by an effect. The conventions rule that out.

## Plan

1. Add one non-reactive resolver next to the hook: `paneChatWorkspaceId(pane)`. It
   returns the chat entity's `workspaceId` (`entityStore.getState()`), else
   `pane.workspaceId`. Use it in `pane-chat-workspace.ts`, `view-state.ts`,
   `release-closed-chat.ts` and `recents-actions.ts`. These all run in event handlers
   or store actions, so `getState()` is allowed there.
2. Make `useActivePaneWorkspaceId` and `useViewWorkspaceIds` read through
   `usePaneChatWorkspaceId`, and the view set through a `useSelect` key over the
   chats in view. That way a move re-renders the retention hook and a streaming frame
   does not.
3. Give `PaneHandle` `usePaneChatWorkspaceId(pane)`.
4. Narrow `PaneGroup.workspaceId` to what it is for: the boot-time placement hint. Rename
   it `placedIn` and keep it out of every reader except `hydrate.ts` and the resolver.

Each step starts with a failing test in the reader's existing test file. The case:
a chat entity written with `ws-2` while its pane recorded `ws-1`.

## Done when

`rg "pane\.workspaceId|\.panes\[.*\]\?\.workspaceId" web/src` matches only
`hydrate.ts`, the resolver and its hook.
