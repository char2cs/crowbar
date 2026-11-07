# Persisted composer drafts

## Problem

A chat's unsent draft lives only in its editor. A chat switch keeps it (the view
is parked), but a reload, a closed tab or a daemon-driven remount loses it.

## Owner and copy

The editor (ChatMarkdownEditor, `persistAs`) stays the one owner of the draft
while it is mounted; nothing reads the stored copy then. Persistence lives in the
editor itself because it mounts after the chat view does. The stored copy is the editor's content at the moments the
editor stops owning it: the page hiding (reload, close) and the view unmounting.
It is read once, when a composer for that chat mounts on an empty editor.

## Contract

- Key: `crowbar:agent-draft:v1:<chatId>` in localStorage (the `lib/local-storage`
  seam). Keyed by chat only, so a chat moved to another workspace keeps its draft.
- Written on `pagehide` and on unmount; an empty draft removes the key.
- Removed when the composer is cleared (send, stash), so a sent prompt never
  comes back as a draft.
- Restored without focus, so a parked tab mounting never takes the caret.
- Per viewer, best effort: storage that throws reads as no draft.
