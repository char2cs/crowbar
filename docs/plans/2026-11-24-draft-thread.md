# A new thread is a draft until its first send

## Why
Develop's "+ New thread" (the home footer, and a plain new thread on a workspace row) opens a
draft: a chat pane on a client-minted id `draft-<uuid>` with no chat on the daemon. Its tab reads
"New thread", it has no identity header and no attach button, and no sidebar row appears. The first
send creates the chat and re-points the pane at it (`web/src/components/layout/draft-promote.ts`,
`draft-thread.ts finishDraftThread`). Closing the draft leaves nothing behind.

The shell posts a chat at once (`store/plan/create.rs home_thread`, `thread`), so it shows an
"Untitled chat" tab with a header and a +, and a sidebar row, before anything is typed.

## Owner of the state
The draft lives in exactly one place: the chat view's params. The shell's layout already owns them
and persists them; nothing else holds the draft.

## Steps
1. **Store** — `Command::NewThread(project)` and `Command::Thread(row)` on a workspace subject that
   lands on Chat stop planning a POST. They answer `Outcome::OpenDraft { workspace, project, parent }`.
   Terminal landings and threads under a chat keep the POST, as develop does.
2. **Shell** — opens the chat type with params `threadId=draft-<uuid>`, `workspaceId`, `projectId`,
   `parentId`. The tab strip labels a draft id "New thread", provisional (italic), as develop's tab.
3. **Panes (`app/panes/src/entries`)** — `ChatPaneMount` fills the root pane with the draft's
   workspace, a view carrying its project and `draftParentId`, so `draftOfPane` reads it as it does
   in develop. `ChatPaneHost` draws a draft without the identity header (`DraftChatPane` through the
   existing `AgentChatPaneGate`).
4. **Re-pointing** — `finishDraftThread`'s `retargetPane` also tells the shell. New SDK frame
   `pane.setParams { threadId }` -> `HostEvent::ParamsChanged` -> a layout command that re-keys the
   pane instance on the new params (same pane id, no re-mount). The page already shows the chat.
5. **Restore** — a draft view restored from a saved layout is dropped, as develop's hydrate clears a
   draft pane's workspace.

## Tests
- store: NewThread answers OpenDraft and sends nothing; a terminal landing still posts.
- layout: re-keying keeps the pane, its tab and focus; a clash with an open view of that chat
  focuses that view and closes the draft.
- panehost: `pane.setParams` decodes to ParamsChanged; an unknown key is refused.
- panes: `chat-pane-mount` mounts a draft with its workspace; the host renders no header for it.
- Rig: "+ New thread" pair (tab, no header, no row), then send "hi" pair (row appears, tab titles).
